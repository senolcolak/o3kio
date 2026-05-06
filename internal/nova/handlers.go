package nova

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog/log"
	"github.com/cobaltcore-dev/o3k/internal/common"
	"github.com/cobaltcore-dev/o3k/internal/database"
	"github.com/cobaltcore-dev/o3k/internal/middleware"
	"github.com/cobaltcore-dev/o3k/internal/neutron"
	"github.com/cobaltcore-dev/o3k/internal/tunnel"
	"github.com/cobaltcore-dev/o3k/pkg/cache"
	"github.com/cobaltcore-dev/o3k/pkg/hypervisor"
)

// Service handles Nova API endpoints
type Service struct {
	db            database.DBIF
	libvirtURI    string
	libvirtMode   string
	vmManager     *hypervisor.VMManager
	cache         *cache.Cache
	neutronSvc    NeutronService // For port allocation
	dispatcher    *tunnel.Dispatcher
	wg            sync.WaitGroup
	ctx           context.Context
	cancel        context.CancelFunc
	// quotaMu serialises quota check + INSERT per project to prevent TOCTOU races.
	quotaMu sync.Map // map[projectID]*sync.Mutex
}

const (
	novaMinVersion     = "2.1"
	novaCurrentVersion = "2.90"
)

// NeutronService defines the interface for Neutron operations Nova needs
type NeutronService interface {
	AllocatePortForInstance(ctx context.Context, networkID, projectID, instanceID string) (interface{}, error)
	BindPort(portID, mac, ip, networkID, hostname string) error
	UnbindPort(portID, mac, networkID string) error
}

// NewService creates a new Nova service
func NewService(libvirtURI, libvirtMode string, cacheInstance *cache.Cache) *Service {
	ctx, cancel := context.WithCancel(context.Background())
	return &Service{
		libvirtURI:  libvirtURI,
		libvirtMode: libvirtMode,
		cache:       cacheInstance,
		neutronSvc:  nil, // Set via SetNeutronService after initialization
		ctx:         ctx,
		cancel:      cancel,
	}
}

// NewServiceWithDB creates a Nova service with an injected DB for testing.
func NewServiceWithDB(db database.DBIF, libvirtMode string) *Service {
	ctx, cancel := context.WithCancel(context.Background())
	return &Service{
		db:          db,
		libvirtMode: libvirtMode,
		ctx:         ctx,
		cancel:      cancel,
	}
}

// activeDB returns the injected DB or falls back to the global.
func (svc *Service) activeDB() database.DBIF {
	if svc.db != nil {
		return svc.db
	}
	return database.DB
}

// projectQuotaMu returns (or lazily creates) the per-project mutex used to
// serialise quota check + resource INSERT and prevent TOCTOU races.
func (svc *Service) projectQuotaMu(projectID string) *sync.Mutex {
	v, _ := svc.quotaMu.LoadOrStore(projectID, &sync.Mutex{})
	return v.(*sync.Mutex)
}

// Shutdown signals all background goroutines to stop and waits for them.
func (svc *Service) Shutdown() {
	svc.cancel()
	svc.wg.Wait()
}

// SetNeutronService sets the Neutron service reference (called after both services are created)
func (svc *Service) SetNeutronService(neutron NeutronService) {
	svc.neutronSvc = neutron
}

// SetDispatcher wires the tunnel Dispatcher for async task dispatch.
func (svc *Service) SetDispatcher(d *tunnel.Dispatcher) {
	svc.dispatcher = d
}

// InitHypervisor initializes the hypervisor connection
func (svc *Service) InitHypervisor() error {
	vmManager, err := hypervisor.NewVMManager(svc.libvirtURI, svc.libvirtMode)
	if err != nil {
		return fmt.Errorf("failed to initialize hypervisor: %w", err)
	}
	svc.vmManager = vmManager
	return nil
}

// RegisterRoutes registers Nova routes
func (svc *Service) RegisterRoutes(r *gin.RouterGroup) {
	// Version discovery
	r.GET("/", svc.ListVersions)
	r.GET("/v2.1", svc.GetVersion)

	v21 := r.Group("/v2.1")
	{
		// Servers (instances)
		v21.GET("/servers", svc.ListServers)
		v21.GET("/servers/detail", svc.ListServersDetail)
		v21.POST("/servers", svc.CreateServer)
		v21.GET("/servers/:id", svc.GetServer)
		v21.PATCH("/servers/:id", svc.UpdateServer)
		v21.PUT("/servers/:id", svc.UpdateServer) // OpenStack also supports PUT for updates
		v21.DELETE("/servers/:id", svc.DeleteServer)
		v21.POST("/servers/:id/action", svc.ServerAction)
		v21.GET("/servers/:id/diagnostics", svc.GetServerDiagnostics)
		v21.GET("/servers/:id/os-instance-actions", svc.ListInstanceActions)
		v21.GET("/servers/:id/os-instance-actions/:request_id", svc.GetInstanceAction)

		// Server metadata
		v21.GET("/servers/:id/metadata", svc.GetServerMetadata)
		v21.POST("/servers/:id/metadata", svc.UpdateServerMetadata)
		v21.PUT("/servers/:id/metadata", svc.ResetServerMetadata)

		// Server tags
		v21.GET("/servers/:id/tags", svc.ListServerTags)
		v21.PUT("/servers/:id/tags", svc.ReplaceServerTags)
		v21.DELETE("/servers/:id/tags", svc.DeleteAllServerTags)
		v21.PUT("/servers/:id/tags/:tag", svc.AddServerTag)
		v21.DELETE("/servers/:id/tags/:tag", svc.DeleteServerTag)

		// Flavors
		v21.GET("/flavors", svc.ListFlavors)
		v21.GET("/flavors/detail", svc.ListFlavorsDetail)
		v21.POST("/flavors", svc.CreateFlavor)
		v21.GET("/flavors/:id", svc.GetFlavor)
		v21.DELETE("/flavors/:id", svc.DeleteFlavor)

		// Flavor extra specs
		v21.GET("/flavors/:id/os-extra_specs", svc.GetFlavorExtraSpecs)
		v21.POST("/flavors/:id/os-extra_specs", svc.CreateFlavorExtraSpecs)
		v21.GET("/flavors/:id/os-extra_specs/:key", svc.GetFlavorExtraSpecKey)
		v21.PUT("/flavors/:id/os-extra_specs/:key", svc.UpdateFlavorExtraSpecKey)
		v21.DELETE("/flavors/:id/os-extra_specs/:key", svc.DeleteFlavorExtraSpecKey)

		// Flavor actions and access
		v21.POST("/flavors/:id/action", svc.FlavorAction)
		v21.GET("/flavors/:id/os-flavor-access", svc.GetFlavorAccess)

		// Images (proxy to Glance)
		v21.GET("/images", svc.ListImages)
		v21.GET("/images/detail", svc.ListImagesDetail)

		// Keypairs
		v21.GET("/os-keypairs", svc.ListKeypairs)
		v21.POST("/os-keypairs", svc.CreateKeypair)
		v21.GET("/os-keypairs/:id", svc.GetKeypair)
		v21.DELETE("/os-keypairs/:id", svc.DeleteKeypair)

		// Hypervisors (for Horizon compatibility)
		v21.GET("/os-hypervisors", svc.ListHypervisors)
		v21.GET("/os-hypervisors/detail", svc.ListHypervisorsDetail)
		v21.GET("/os-hypervisors/statistics", svc.GetHypervisorStatistics)

		// Availability zones
		v21.GET("/os-availability-zone", svc.ListAvailabilityZones)
		v21.GET("/os-availability-zone/detail", svc.ListAvailabilityZonesDetail)

		// Limits (quotas and usage)
		v21.GET("/limits", svc.GetLimits)

		// Services (compute service status)
		v21.GET("/os-services", svc.ListServices)

		// Tenant usage (usage statistics)
		v21.GET("/os-simple-tenant-usage", svc.ListTenantUsage)
		v21.GET("/os-simple-tenant-usage/:id", svc.GetTenantUsage)

		// Server groups
		v21.GET("/os-server-groups", svc.ListServerGroups)
		v21.POST("/os-server-groups", svc.CreateServerGroup)
		v21.GET("/os-server-groups/:id", svc.GetServerGroup)
		v21.DELETE("/os-server-groups/:id", svc.DeleteServerGroup)

		// Server migrations
		v21.GET("/os-migrations", svc.ListMigrations)
		v21.GET("/servers/:id/migrations", svc.ListServerMigrations)
		v21.GET("/servers/:id/migrations/:migration_id", svc.GetServerMigration)
		v21.DELETE("/servers/:id/migrations/:migration_id", svc.DeleteServerMigration)
		v21.POST("/servers/:id/migrations/:migration_id/action", svc.ServerMigrationAction)

		// Volume attachments
		v21.GET("/servers/:id/os-volume_attachments", svc.ListVolumeAttachments)
		v21.POST("/servers/:id/os-volume_attachments", svc.AttachVolume)
		v21.DELETE("/servers/:id/os-volume_attachments/:volume_id", svc.DetachVolume)

		// Interface attachments (network hot-plug)
		v21.GET("/servers/:id/os-interface", svc.ListInterfaceAttachments)
		v21.POST("/servers/:id/os-interface", svc.AttachInterface)
		v21.DELETE("/servers/:id/os-interface/:port_id", svc.DetachInterface)

		// Quotas
		v21.GET("/os-quota-sets/:id", svc.GetQuotaSet)
		v21.PUT("/os-quota-sets/:id", svc.UpdateQuotaSet)
		v21.GET("/os-quota-sets/:id/defaults", svc.GetQuotaSetDefaults)

		// Console access
		v21.POST("/servers/:id/remote-consoles", svc.GetRemoteConsole)

		// Host aggregates
		v21.GET("/os-aggregates", svc.ListAggregates)
		v21.POST("/os-aggregates", svc.CreateAggregate)
		v21.GET("/os-aggregates/:id", svc.GetAggregate)
		v21.PUT("/os-aggregates/:id", svc.UpdateAggregate)
		v21.DELETE("/os-aggregates/:id", svc.DeleteAggregate)
		v21.POST("/os-aggregates/:id/action", svc.AggregateAction)
	}
}

// ListVersions returns available API versions
func (svc *Service) ListVersions(c *gin.Context) {
	c.JSON(200, gin.H{
		"versions": []gin.H{
			{
				"id":          "v2.1",
				"status":      "CURRENT",
				"version":     novaCurrentVersion,
				"min_version": novaMinVersion,
				"links": []gin.H{
					{"rel": "self", "href": fmt.Sprintf("%s/v2.1", common.BaseURL(c, 8774))},
				},
			},
		},
	})
}

// GetVersion returns version details
func (svc *Service) GetVersion(c *gin.Context) {
	c.Header("OpenStack-API-Version", "compute "+novaCurrentVersion)
	c.Header("OpenStack-API-Minimum-Version", "compute "+novaMinVersion)
	c.JSON(200, gin.H{
		"version": gin.H{
			"id":          "v2.1",
			"status":      "CURRENT",
			"version":     novaCurrentVersion,
			"min_version": novaMinVersion,
		},
	})
}

// CreateServerRequest represents a server creation request
type CreateServerRequest struct {
	Server struct {
		Name      string `json:"name" binding:"required"`
		FlavorRef string `json:"flavorRef" binding:"required"`
		ImageRef  string `json:"imageRef"`
		Networks  []struct {
			UUID string `json:"uuid"`
		} `json:"networks"`
		KeyName string `json:"key_name"`
	} `json:"server"`
}

// CreateServer creates a new server instance
func (svc *Service) CreateServer(c *gin.Context) {
	logger := middleware.GetLogger(c)
	start := time.Now()

	var req CreateServerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Warn().Err(err).Msg("Invalid request body for server creation")
		common.SendError(c, common.NewBadRequestError("invalid request body"))
		return
	}

	projectID := c.GetString("project_id")
	userID := c.GetString("user_id")

	middleware.LogOperationStart(c, "create", "server", req.Server.Name)

	// Fetch flavor (support lookup by UUID or name)
	var flavor database.Flavor
	queryStart := time.Now()
	err := svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT id, name, vcpus, ram_mb, disk_gb FROM flavors WHERE id::text = $1 OR name = $1 LIMIT 1",
		req.Server.FlavorRef,
	).Scan(&flavor.ID, &flavor.Name, &flavor.VCPUs, &flavor.RAMMB, &flavor.DiskGB)
	middleware.LogDatabaseQuery(c, "SELECT flavor", time.Since(queryStart), err)

	if err == pgx.ErrNoRows {
		logger.Warn().Str("flavor_ref", req.Server.FlavorRef).Msg("Flavor not found")
		middleware.LogOperationEnd(c, "create", "server", req.Server.Name, time.Since(start), err)
		common.SendError(c, common.NewNotFoundError("flavor"))
		return
	}
	if err != nil {
		logger.Error().Err(err).Msg("Failed to query flavor")
		middleware.LogOperationEnd(c, "create", "server", req.Server.Name, time.Since(start), err)
		log.Error().Err(err).Str("operation", "query_flavor").Msg("database error")
		common.SendError(c, common.NewInternalServerError("failed to query flavor"))
		return
	}

	// Serialise quota check + INSERT per project to prevent TOCTOU races.
	// The mutex is held until the instance row is committed, so concurrent
	// requests for the same project cannot both pass the quota check.
	mu := svc.projectQuotaMu(projectID)
	mu.Lock()

	// Check quotas before creating instance
	if err := svc.CheckQuota(c, "instances", 1); err != nil {
		mu.Unlock()
		if _, ok := err.(*QuotaExceededError); ok {
			common.SendError(c, common.NewQuotaExceededError("instances"))
			return
		}
		log.Error().Err(err).Str("operation", "check_quota_instances").Msg("quota check error")
		common.SendError(c, common.NewInternalServerError("failed to check quota"))
		return
	}

	// Check cores quota
	if err := svc.CheckQuota(c, "cores", flavor.VCPUs); err != nil {
		mu.Unlock()
		if _, ok := err.(*QuotaExceededError); ok {
			common.SendError(c, common.NewQuotaExceededError("cores"))
			return
		}
		log.Error().Err(err).Str("operation", "check_quota_cores").Msg("quota check error")
		common.SendError(c, common.NewInternalServerError("failed to check quota"))
		return
	}

	// Check RAM quota
	if err := svc.CheckQuota(c, "ram", flavor.RAMMB); err != nil {
		mu.Unlock()
		if _, ok := err.(*QuotaExceededError); ok {
			common.SendError(c, common.NewQuotaExceededError("ram"))
			return
		}
		log.Error().Err(err).Str("operation", "check_quota_ram").Msg("quota check error")
		common.SendError(c, common.NewInternalServerError("failed to check quota"))
		return
	}

	// Generate instance ID
	instanceID := uuid.New().String()

	// Create instance record in database
	now := time.Now()

	// Handle NULL image_id (for volume-backed instances)
	var imageID interface{}
	if req.Server.ImageRef != "" {
		imageID = req.Server.ImageRef
	} else {
		imageID = nil
	}

	queryStart = time.Now()
	_, err = svc.activeDB().Exec(c.Request.Context(), `
		INSERT INTO instances (id, name, project_id, user_id, flavor_id, image_id, status, power_state, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`, instanceID, req.Server.Name, projectID, userID, flavor.ID, imageID, "BUILD", 0, now, now)
	// Release the quota lock as soon as the row is committed (or failed).
	mu.Unlock()
	middleware.LogDatabaseQuery(c, "INSERT instance", time.Since(queryStart), err)

	if err != nil {
		logger.Error().Err(err).Str("instance_id", instanceID).Msg("Failed to create instance in database")
		middleware.LogOperationEnd(c, "create", "server", instanceID, time.Since(start), err)
		log.Error().Err(err).Str("operation", "insert_instance").Msg("database error")
		common.SendError(c, common.NewInternalServerError("failed to create instance"))
		return
	}

	logger.Info().Str("instance_id", instanceID).Str("flavor", flavor.Name).Msg("Instance record created")

	// Log instance action
	requestID := uuid.New().String()
	if _, err := svc.activeDB().Exec(c.Request.Context(), `
		INSERT INTO instance_actions (instance_id, action, request_id, user_id, project_id, start_time, message)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, instanceID, "create", requestID, userID, projectID, now, "Instance created"); err != nil {
		log.Debug().Err(err).Str("instance_id", instanceID).Msg("failed to log instance action (non-critical)")
	}

	// Async mode: insert task row for the worker to pick up
	if svc.dispatcher != nil {
		taskPayload, _ := json.Marshal(map[string]interface{}{
			"instance_id":      instanceID,
			"flavor_id":        req.Server.FlavorRef,
			"image_local_path": fmt.Sprintf("/var/lib/o3k/images/%s.qcow2", req.Server.ImageRef),
			"vcpu":             flavor.VCPUs,
			"ram_mb":           flavor.RAMMB,
			"disk_gb":          flavor.DiskGB,
		})

		taskID := uuid.New().String()
		_, err := svc.activeDB().Exec(c.Request.Context(), `
			INSERT INTO tasks (id, type, resource_id, project_id, payload, timeout_sec, req_vcpu, req_ram_mb, req_disk_gb)
			VALUES ($1, 'VM_CREATE', $2, $3, $4, 120, $5, $6, $7)`,
			taskID, instanceID, projectID, taskPayload, flavor.VCPUs, flavor.RAMMB, flavor.DiskGB)

		if err != nil {
			log.Error().Err(err).Str("task_id", taskID).Msg("Failed to insert task")
		} else {
			svc.activeDB().Exec(c.Request.Context(), "SELECT pg_notify('new_task', $1)", taskID)
		}
		// Skip the sync VM creation goroutine — worker handles it
	} else if svc.vmManager != nil {
		logger.Info().Str("instance_id", instanceID).Msg("Starting VM creation via libvirt")
		svc.wg.Add(1)
		go func() {
			defer svc.wg.Done()
			// Recover from panics in goroutine
			defer func() {
				if r := recover(); r != nil {
					log.Error().Interface("panic", r).Str("instance_id", instanceID).Msg("PANIC in VM creation goroutine")
					svc.activeDB().Exec(context.Background(),
						"UPDATE instances SET status = $1, updated_at = $2 WHERE id = $3",
						"ERROR", time.Now(), instanceID)
				}
			}()

			ctx, cancel := context.WithTimeout(svc.ctx, 30*time.Second)
			defer cancel()

			log.Debug().Str("instance_id", instanceID).Msg("VM creation goroutine started")
			libvirtStart := time.Now()

			// Allocate ports from Neutron for requested networks
			log.Debug().Str("instance_id", instanceID).Int("network_count", len(req.Server.Networks)).Msg("Allocating ports")
			var networks []hypervisor.NetworkConfig
			if svc.neutronSvc != nil && len(req.Server.Networks) > 0 {
				for _, network := range req.Server.Networks {
					log.Debug().Str("network_id", network.UUID).Msg("Allocating port for network")
					portInfoRaw, err := svc.neutronSvc.AllocatePortForInstance(ctx, network.UUID, projectID, instanceID)
					if err != nil {
						log.Error().Err(err).
							Str("instance_id", instanceID).
							Str("network_id", network.UUID).
							Msg("Failed to allocate port from Neutron")
						// Continue with empty networks rather than failing
						continue
					}

					// Type assert to neutron.PortInfo
					if portInfo, ok := portInfoRaw.(*neutron.PortInfo); ok {
						networks = append(networks, hypervisor.NetworkConfig{
							PortID:     portInfo.ID,
							MACAddress: portInfo.MAC,
							BridgeName: fmt.Sprintf("br-%s", portInfo.NetworkID[:8]),
							IPAddress:  portInfo.IPAddress,
							NetworkID:  portInfo.NetworkID,
						})
					}
				}
			}

			// Bind ports to prepare DHCP leases on the host
			if svc.neutronSvc != nil {
				for _, net := range networks {
					if err := svc.neutronSvc.BindPort(net.PortID, net.MACAddress, net.IPAddress, net.NetworkID, req.Server.Name); err != nil {
						log.Warn().Err(err).Str("port_id", net.PortID).Msg("Failed to bind port")
					}
				}
			}

			// Generate cloud-init configuration if SSH key is provided
			var cloudInit *hypervisor.CloudInitConfig
			if req.Server.KeyName != "" {
				// Fetch SSH public key from database
				var publicKey string
				err := svc.activeDB().QueryRow(ctx,
					"SELECT public_key FROM keypairs WHERE user_id = $1 AND name = $2",
					userID, req.Server.KeyName,
				).Scan(&publicKey)

				if err == nil {
					// Generate cloud-init config with SSH key
					cloudInit = hypervisor.DefaultCloudInitConfig(req.Server.Name, publicKey)

					// Generate cloud-init ISO
					isoPath, err := hypervisor.GenerateCloudInitISO(instanceID, cloudInit)
					if err != nil {
						logger.Error().Err(err).
							Str("instance_id", instanceID).
							Msg("Failed to generate cloud-init ISO")
						// Continue without cloud-init rather than failing
					} else {
						logger.Info().
							Str("instance_id", instanceID).
							Str("iso_path", isoPath).
							Msg("Cloud-init ISO generated successfully")
					}
				} else {
					logger.Warn().Err(err).
						Str("key_name", req.Server.KeyName).
						Msg("Failed to fetch SSH key for cloud-init")
				}
			}

			// Generate VM XML
			log.Debug().Str("instance_id", instanceID).Msg("Generating VM XML")
			spec := hypervisor.VMSpec{
				UUID:      instanceID,
				Name:      fmt.Sprintf("instance-%s", instanceID[:8]),
				VCPUs:     flavor.VCPUs,
				MemoryMB:  flavor.RAMMB,
				DiskGB:    flavor.DiskGB,
				ImagePath: fmt.Sprintf("/var/lib/o3k/images/%s.qcow2", req.Server.ImageRef),
				Networks:  networks,
				CloudInit: cloudInit,
			}

			log.Debug().Str("image_path", spec.ImagePath).Int("network_count", len(spec.Networks)).Msg("VM spec prepared")
			xml := hypervisor.GenerateVMXML(spec)

			// Create VM
			log.Debug().Str("instance_id", instanceID).Msg("Calling CreateVM")
			libvirtUUID, err := svc.vmManager.CreateVM(ctx, xml)

			if err != nil {
				log.Error().Err(err).Str("instance_id", instanceID).Msg("Failed to create VM via libvirt")
				// Update instance status to ERROR
				dbCtx, dbCancel := context.WithTimeout(svc.ctx, 5*time.Second)
				defer dbCancel()
				svc.activeDB().Exec(dbCtx,
					"UPDATE instances SET status = $1, updated_at = $2 WHERE id = $3",
					"ERROR", time.Now(), instanceID)
				return
			}

			log.Debug().Str("libvirt_uuid", libvirtUUID).Msg("VM created successfully")

			logger.Info().
				Str("instance_id", instanceID).
				Str("libvirt_uuid", libvirtUUID).
				Dur("duration", time.Since(libvirtStart)).
				Msg("VM created successfully via libvirt")

			// Update instance with libvirt UUID
			dbCtx, dbCancel := context.WithTimeout(svc.ctx, 5*time.Second)
			defer dbCancel()
			svc.activeDB().Exec(dbCtx, `
				UPDATE instances
				SET status = $1, power_state = $2, libvirt_domain_id = $3, launched_at = $4, updated_at = $5
				WHERE id = $6
			`, "ACTIVE", 1, libvirtUUID, time.Now(), time.Now(), instanceID)
		}()
	} else {
		logger.Debug().Msg("Stub mode: skipping libvirt VM creation")
		// In pure stub mode, auto-transition to ACTIVE after a brief delay
		go func() {
			time.Sleep(100 * time.Millisecond)
			svc.activeDB().Exec(context.Background(),
				"UPDATE instances SET status = 'ACTIVE', power_state = 1, task_state = '', updated_at = $1 WHERE id = $2 AND status = 'BUILD'",
				time.Now(), instanceID,
			)
		}()
	}

	middleware.LogOperationEnd(c, "create", "server", instanceID, time.Since(start), nil)

	// Return instance details
	c.JSON(http.StatusAccepted, gin.H{
		"server": gin.H{
			"id":         instanceID,
			"name":       req.Server.Name,
			"status":     "BUILD",
			"tenant_id":  projectID,
			"user_id":    userID,
			"created":    now.Format(time.RFC3339),
			"updated":    now.Format(time.RFC3339),
			"flavor":     gin.H{"id": flavor.ID},
			"image":      gin.H{"id": req.Server.ImageRef},
			"metadata":   gin.H{},
			"adminPass":  common.GeneratePassword(16),
		},
	})
}

// ListServers lists all servers (brief)
func (svc *Service) ListServers(c *gin.Context) {
	projectID := c.GetString("project_id")

	// Parse pagination parameters
	limit := 1000 // Default limit
	offset := 0
	if limitParam := c.Query("limit"); limitParam != "" {
		if parsedLimit, err := strconv.Atoi(limitParam); err == nil && parsedLimit > 0 {
			limit = parsedLimit
		}
	}
	if offsetParam := c.Query("offset"); offsetParam != "" {
		if parsedOffset, err := strconv.Atoi(offsetParam); err == nil && parsedOffset >= 0 {
			offset = parsedOffset
		}
	}
	if markerParam := c.Query("marker"); markerParam != "" {
		// Marker-based pagination: get offset of marker UUID
		var markerOffset int
		svc.activeDB().QueryRow(c.Request.Context(),
			`SELECT ROW_NUMBER() OVER (ORDER BY created_at DESC) - 1
			 FROM instances WHERE project_id = $1 AND id = $2`,
			projectID, markerParam,
		).Scan(&markerOffset)
		if markerOffset > 0 {
			offset = markerOffset
		}
	}

	rows, err := svc.activeDB().Query(c.Request.Context(),
		"SELECT id, name FROM instances WHERE project_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3",
		projectID, limit, offset,
	)
	if err != nil {
		log.Error().Err(err).Str("operation", "list_servers").Msg("database error")
		common.SendError(c, common.NewInternalServerError("failed to list servers"))
		return
	}
	defer rows.Close()

	var servers []gin.H
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			log.Warn().Err(err).Msg("failed to scan server row")
			continue
		}
		servers = append(servers, gin.H{
			"id":   id,
			"name": name,
			"links": []gin.H{
				{"rel": "self", "href": fmt.Sprintf("%s/v2.1/servers/%s", common.BaseURL(c, 8774), id)},
			},
		})
	}

	if servers == nil {
		servers = []gin.H{}
	}

	c.JSON(http.StatusOK, gin.H{"servers": servers})
}

// ListServersDetail lists all servers (detailed)
func (svc *Service) ListServersDetail(c *gin.Context) {
	projectID := c.GetString("project_id")

	// Parse pagination parameters
	limit := 1000
	offset := 0
	if limitParam := c.Query("limit"); limitParam != "" {
		if parsedLimit, err := strconv.Atoi(limitParam); err == nil && parsedLimit > 0 {
			limit = parsedLimit
		}
	}
	if offsetParam := c.Query("offset"); offsetParam != "" {
		if parsedOffset, err := strconv.Atoi(offsetParam); err == nil && parsedOffset >= 0 {
			offset = parsedOffset
		}
	}

	// Marker-based pagination
	var markerCondition string
	var queryArgs []interface{}
	queryArgs = append(queryArgs, projectID)
	argIdx := 2

	if marker := c.Query("marker"); marker != "" {
		var markerCreatedAt time.Time
		err := svc.activeDB().QueryRow(c.Request.Context(),
			"SELECT created_at FROM instances WHERE id = $1 AND project_id = $2",
			marker, projectID,
		).Scan(&markerCreatedAt)
		if err == nil {
			markerCondition = fmt.Sprintf(" AND i.created_at < $%d", argIdx)
			queryArgs = append(queryArgs, markerCreatedAt)
			argIdx++
		}
	}

	queryArgs = append(queryArgs, limit, offset)

	rows, err := svc.activeDB().Query(c.Request.Context(), fmt.Sprintf(`
		SELECT i.id, i.name, i.status, i.power_state, i.project_id, i.user_id,
		       i.flavor_id, i.image_id, i.created_at, i.updated_at, i.launched_at,
		       f.vcpus, f.ram_mb, f.disk_gb, f.name as flavor_name
		FROM instances i
		LEFT JOIN flavors f ON i.flavor_id = f.id
		WHERE i.project_id = $1%s
		ORDER BY i.created_at DESC
		LIMIT $%d OFFSET $%d
	`, markerCondition, argIdx, argIdx+1), queryArgs...)

	if err != nil {
		log.Error().Err(err).Str("operation", "list_servers_detail").Msg("database error")
		common.SendError(c, common.NewInternalServerError("failed to list servers"))
		return
	}
	defer rows.Close()

	var servers []gin.H
	for rows.Next() {
		var id, name, status, projectID, userID, flavorID, flavorName string
		var imageID *string
		var powerState, vcpus, ramMB, diskGB int
		var createdAt, updatedAt time.Time
		var launchedAt *time.Time

		if err := rows.Scan(&id, &name, &status, &powerState, &projectID, &userID,
			&flavorID, &imageID, &createdAt, &updatedAt, &launchedAt,
			&vcpus, &ramMB, &diskGB, &flavorName); err != nil {
			log.Warn().Err(err).Msg("failed to scan server detail row")
			continue
		}

		imageIDStr := ""
		if imageID != nil {
			imageIDStr = *imageID
		}
		launchedAtStr := ""
		if launchedAt != nil {
			launchedAtStr = launchedAt.Format(time.RFC3339)
		}

		// Get addresses for this instance
		addresses := svc.getInstanceAddresses(c.Request.Context(), id, projectID)

		servers = append(servers, gin.H{
			"id":         id,
			"name":       name,
			"status":     status,
			"tenant_id":  projectID,
			"user_id":    userID,
			"created":    createdAt.Format(time.RFC3339),
			"updated":    updatedAt.Format(time.RFC3339),
			"addresses":  addresses,
			"OS-EXT-STS:power_state": powerState,
			"OS-SRV-USG:launched_at": launchedAtStr,
			"flavor": gin.H{
				"id":    flavorID,
				"vcpus": vcpus,
				"ram":   ramMB,
				"disk":  diskGB,
			},
			"image": gin.H{"id": imageIDStr},
		})
	}

	if servers == nil {
		servers = []gin.H{}
	}

	c.JSON(http.StatusOK, gin.H{"servers": servers})
}

// getInstanceAddresses retrieves network addresses for an instance from ports
func (svc *Service) getInstanceAddresses(ctx context.Context, instanceID, projectID string) gin.H {
	addresses := gin.H{}

	// Query ports for this instance
	rows, err := svc.activeDB().Query(ctx, `
		SELECT p.network_id, p.fixed_ips, n.name
		FROM ports p
		JOIN networks n ON p.network_id = n.id
		WHERE p.device_id = $1 AND p.project_id = $2
	`, instanceID, projectID)

	if err != nil {
		return addresses // Return empty dict on error
	}
	defer rows.Close()

	for rows.Next() {
		var networkID, networkName string
		var fixedIPsJSON []byte

		if err := rows.Scan(&networkID, &fixedIPsJSON, &networkName); err != nil {
			log.Warn().Err(err).Msg("failed to scan network row")
			continue
		}

		// Parse fixed IPs
		var fixedIPs []map[string]interface{}
		if err := json.Unmarshal(fixedIPsJSON, &fixedIPs); err != nil {
			continue
		}

		// Build address list for this network
		var addressList []gin.H
		for _, ipInfo := range fixedIPs {
			if ipAddr, ok := ipInfo["ip_address"].(string); ok {
				addressList = append(addressList, gin.H{
					"addr":    ipAddr,
					"version": 4,
					"OS-EXT-IPS:type": "fixed",
				})
			}
		}

		if len(addressList) > 0 {
			addresses[networkName] = addressList
		}
	}

	return addresses
}

// GetServer returns details for a single server
func (svc *Service) GetServer(c *gin.Context) {
	instanceID := c.Param("id")
	projectID := c.GetString("project_id")

	var id, name, status, projID string
	var userID, flavorID, imageID interface{}
	var powerState int
	var createdAt, updatedAt time.Time

	// Try to find by ID first, then by name
	// Use separate conditions to avoid type mismatch when id is UUID and param might be a name
	err := svc.activeDB().QueryRow(c.Request.Context(), `
		SELECT id, name, status, power_state, project_id, user_id, flavor_id, image_id, created_at, updated_at
		FROM instances
		WHERE project_id = $2 AND (
			(id::text = $1) OR (name = $1)
		)
	`, instanceID, projectID).Scan(&id, &name, &status, &powerState, &projID, &userID, &flavorID, &imageID, &createdAt, &updatedAt)

	if err == pgx.ErrNoRows {
		common.SendError(c, common.NewNotFoundError("instance"))
		return
	}
	if err != nil {
		log.Error().Err(err).Str("operation", "get_server").Msg("database error")
		common.SendError(c, common.NewInternalServerError("failed to get server"))
		return
	}

	// Get addresses from ports
	addresses := svc.getInstanceAddresses(c.Request.Context(), id, projectID)

	// Build response with nullable fields
	response := gin.H{
		"id":                     id,
		"name":                   name,
		"status":                 status,
		"tenant_id":              projID,
		"created":                createdAt.Format(time.RFC3339),
		"updated":                updatedAt.Format(time.RFC3339),
		"addresses":              addresses,
		"OS-EXT-STS:power_state": powerState,
	}

	if userID != nil {
		response["user_id"] = userID
	}
	if flavorID != nil {
		response["flavor"] = gin.H{"id": flavorID}
	}
	if imageID != nil {
		response["image"] = gin.H{"id": imageID}
	}

	c.JSON(http.StatusOK, gin.H{"server": response})
}

// DeleteServer deletes a server
func (svc *Service) DeleteServer(c *gin.Context) {
	logger := middleware.GetLogger(c)
	start := time.Now()

	instanceID := c.Param("id")
	projectID := c.GetString("project_id")

	middleware.LogOperationStart(c, "delete", "server", instanceID)

	// Get libvirt domain ID (support lookup by ID or name)
	var libvirtDomainID string
	queryStart := time.Now()
	err := svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT libvirt_domain_id FROM instances WHERE project_id = $2 AND ((id::text = $1) OR (name = $1))",
		instanceID, projectID,
	).Scan(&libvirtDomainID)
	middleware.LogDatabaseQuery(c, "SELECT libvirt_domain_id", time.Since(queryStart), err)

	if err == pgx.ErrNoRows {
		logger.Warn().Str("instance_id", instanceID).Msg("Instance not found")
		middleware.LogOperationEnd(c, "delete", "server", instanceID, time.Since(start), err)
		common.SendError(c, common.NewNotFoundError("instance"))
		return
	}

	// Delete VM from libvirt (if available)
	if svc.vmManager != nil && libvirtDomainID != "" {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
		defer cancel()

		libvirtStart := time.Now()
		if err := svc.vmManager.DeleteVM(ctx, libvirtDomainID); err != nil {
			logger.Error().Err(err).Str("libvirt_domain_id", libvirtDomainID).Msg("Failed to delete VM from libvirt")
			middleware.LogExternalService(c, "libvirt", "delete_vm", time.Since(libvirtStart), err)
			middleware.LogOperationEnd(c, "delete", "server", instanceID, time.Since(start), err)
			log.Error().Err(err).Str("operation", "delete_vm_libvirt").Msg("libvirt error")
			common.SendError(c, common.NewInternalServerError("failed to delete server"))
			return
		}
		middleware.LogExternalService(c, "libvirt", "delete_vm", time.Since(libvirtStart), nil)
		logger.Info().Str("libvirt_domain_id", libvirtDomainID).Msg("VM deleted from libvirt")
	}

	// Unbind ports before deleting them from the database
	if svc.neutronSvc != nil {
		ctx := c.Request.Context()
		rows, portErr := svc.activeDB().Query(ctx,
			"SELECT id, mac_address, network_id FROM ports WHERE device_id = $1 AND project_id = $2",
			instanceID, projectID)
		if portErr == nil {
			defer rows.Close()
			for rows.Next() {
				var portID, mac, networkID string
				if scanErr := rows.Scan(&portID, &mac, &networkID); scanErr == nil {
					if unbindErr := svc.neutronSvc.UnbindPort(portID, mac, networkID); unbindErr != nil {
						logger.Warn().Err(unbindErr).Str("port_id", portID).Msg("Failed to unbind port")
					}
				}
			}
		}
	}

	// Delete orphaned ports for this instance
	svc.activeDB().Exec(c.Request.Context(),
		"DELETE FROM ports WHERE device_id = $1 AND project_id = $2",
		instanceID, projectID)

	// Delete from database (support lookup by ID or name)
	queryStart = time.Now()
	_, err = svc.activeDB().Exec(c.Request.Context(),
		"DELETE FROM instances WHERE project_id = $2 AND ((id::text = $1) OR (name = $1))",
		instanceID, projectID,
	)
	middleware.LogDatabaseQuery(c, "DELETE instance", time.Since(queryStart), err)

	if err != nil {
		logger.Error().Err(err).Msg("Failed to delete instance from database")
		middleware.LogOperationEnd(c, "delete", "server", instanceID, time.Since(start), err)
		log.Error().Err(err).Str("operation", "delete_instance").Msg("database error")
		common.SendError(c, common.NewInternalServerError("failed to delete server"))
		return
	}

	logger.Info().Str("instance_id", instanceID).Msg("Instance deleted successfully")
	middleware.LogOperationEnd(c, "delete", "server", instanceID, time.Since(start), nil)

	c.Status(http.StatusNoContent)
}

// ServerAction performs an action on a server
func (svc *Service) ServerAction(c *gin.Context) {
	instanceID := c.Param("id")
	projectID := c.GetString("project_id")

	var req map[string]interface{}
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Warn().Err(err).Str("operation", "server_action_bind").Msg("Failed to bind JSON")
		common.SendError(c, common.NewBadRequestError("invalid request body"))
		return
	}

	log.Debug().Str("instance_id", instanceID).Str("project_id", projectID).Interface("action", req).Msg("ServerAction")

	// Handle console actions first (don't require libvirt)
	if vncConsole, ok := req["os-getVNCConsole"]; ok {
		svc.GetVNCConsoleAction(c, vncConsole)
		return
	}
	if consoleOutput, ok := req["os-getConsoleOutput"]; ok {
		svc.GetConsoleOutputAction(c, consoleOutput)
		return
	}
	if serialConsole, ok := req["os-getSerialConsole"]; ok {
		svc.GetSerialConsoleAction(c, serialConsole)
		return
	}
	if spiceConsole, ok := req["os-getSPICEConsole"]; ok {
		svc.GetSPICEConsoleAction(c, spiceConsole)
		return
	}
	if rdpConsole, ok := req["os-getRDPConsole"]; ok {
		svc.GetRDPConsoleAction(c, rdpConsole)
		return
	}

	// Handle advanced actions that don't require libvirt in all cases
	if _, ok := req["suspend"]; ok {
		svc.SuspendInstance(c)
		return
	} else if _, ok := req["resume"]; ok {
		svc.ResumeInstance(c)
		return
	} else if _, ok := req["shelve"]; ok {
		svc.ShelveInstance(c)
		return
	} else if _, ok := req["shelveOffload"]; ok {
		svc.ShelveInstance(c) // Same as shelve for now
		return
	} else if _, ok := req["unshelve"]; ok {
		svc.UnshelveInstance(c)
		return
	} else if resizeData, ok := req["resize"]; ok {
		svc.ResizeInstanceAction(c, resizeData)
		return
	} else if _, ok := req["confirmResize"]; ok {
		svc.ConfirmResizeInstance(c)
		return
	} else if _, ok := req["revertResize"]; ok {
		svc.RevertResizeInstance(c)
		return
	} else if rebuildData, ok := req["rebuild"]; ok {
		svc.RebuildInstanceAction(c, rebuildData)
		return
	} else if rescueData, ok := req["rescue"]; ok {
		svc.RescueInstanceAction(c, rescueData)
		return
	} else if createImageData, ok := req["createImage"]; ok {
		svc.CreateImageAction(c, createImageData)
		return
	} else if _, ok := req["pause"]; ok {
		svc.PauseInstanceAction(c)
		return
	} else if _, ok := req["unpause"]; ok {
		svc.UnpauseInstanceAction(c)
		return
	} else if _, ok := req["lock"]; ok {
		svc.LockInstanceAction(c)
		return
	} else if _, ok := req["unlock"]; ok {
		svc.UnlockInstanceAction(c)
		return
	} else if _, ok := req["forceDelete"]; ok {
		svc.ForceDeleteInstanceAction(c)
		return
	} else if _, ok := req["evacuate"]; ok {
		svc.EvacuateInstance(c)
		return
	} else if _, ok := req["migrate"]; ok {
		svc.MigrateInstance(c)
		return
	} else if liveMigrate, ok := req["os-migrateLive"]; ok {
		// Pass the parsed live migrate data through context
		c.Set("action_data", liveMigrate)
		svc.LiveMigrateInstance(c)
		return
	} else if addSG, ok := req["addSecurityGroup"]; ok {
		c.Set("action_data", addSG)
		svc.AddSecurityGroup(c)
		return
	} else if removeSG, ok := req["removeSecurityGroup"]; ok {
		c.Set("action_data", removeSG)
		svc.RemoveSecurityGroup(c)
		return
	} else if changePass, ok := req["changePassword"]; ok {
		c.Set("action_data", changePass)
		svc.ChangePassword(c)
		return
	} else if _, ok := req["restore"]; ok {
		svc.RestoreInstance(c)
		return
	} else if createBackup, ok := req["createBackup"]; ok {
		c.Set("action_data", createBackup)
		svc.CreateBackupAction(c)
		return
	} else if resetState, ok := req["os-resetState"]; ok {
		c.Set("action_data", resetState)
		svc.ResetStateAction(c)
		return
	} else if _, ok := req["os-resetNetwork"]; ok {
		svc.ResetNetworkAction(c)
		return
	}

	// Get libvirt domain ID for remaining actions (support lookup by ID or name)
	var libvirtDomainID interface{}
	err := svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT libvirt_domain_id FROM instances WHERE project_id = $2 AND ((id::text = $1) OR (name = $1))",
		instanceID, projectID,
	).Scan(&libvirtDomainID)

	if err == pgx.ErrNoRows {
		log.Warn().Str("instance_id", instanceID).Msg("Instance not found in ServerAction")
		common.SendError(c, common.NewNotFoundError("instance"))
		return
	}
	if err != nil {
		log.Error().Err(err).Str("operation", "server_action_query").Msg("database error")
		common.SendError(c, common.NewInternalServerError("failed to query instance"))
		return
	}

	libvirtDomainStr := ""
	if libvirtDomainID != nil {
		libvirtDomainStr = fmt.Sprintf("%v", libvirtDomainID)
	}

	log.Debug().
		Bool("libvirt_valid", libvirtDomainStr != "").
		Str("libvirt_domain", libvirtDomainStr).
		Bool("vm_manager", svc.vmManager != nil).
		Str("mode", svc.libvirtMode).
		Msg("ServerAction state")

	// In stub mode, just update database status (don't call vmManager even if it exists)
	if svc.libvirtMode == "stub" || libvirtDomainStr == "" {
		// Handle actions in stub mode by updating database only
		if _, ok := req["reboot"]; ok {
			// Just mark as rebooting then active
			svc.activeDB().Exec(c.Request.Context(),
				"UPDATE instances SET status = $1, updated_at = $2 WHERE (id::text = $3 OR name = $3) AND project_id = $4",
				"REBOOT", time.Now(), instanceID, projectID)
			svc.wg.Add(1)
			go func() {
				defer svc.wg.Done()
				select {
				case <-time.After(1 * time.Second):
				case <-svc.ctx.Done():
					return
				}
				ctx, cancel := context.WithTimeout(svc.ctx, 5*time.Second)
				defer cancel()
				svc.activeDB().Exec(ctx,
					"UPDATE instances SET status = $1, updated_at = $2 WHERE (id::text = $3 OR name = $3) AND project_id = $4",
					"ACTIVE", time.Now(), instanceID, projectID)
			}()
		} else if _, ok := req["os-stop"]; ok {
			svc.activeDB().Exec(c.Request.Context(),
				"UPDATE instances SET status = $1, power_state = $2, updated_at = $3 WHERE (id::text = $4 OR name = $4) AND project_id = $5",
				"SHUTOFF", 4, time.Now(), instanceID, projectID)
		} else if _, ok := req["os-start"]; ok {
			svc.activeDB().Exec(c.Request.Context(),
				"UPDATE instances SET status = $1, power_state = $2, updated_at = $3 WHERE (id::text = $4 OR name = $4) AND project_id = $5",
				"ACTIVE", 1, time.Now(), instanceID, projectID)
		} else {
			common.SendError(c, common.NewBadRequestError("unknown action"))
			return
		}
		c.Status(http.StatusAccepted)
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	// Handle different actions with real libvirt
	if _, ok := req["reboot"]; ok {
		if err := svc.vmManager.RebootVM(ctx, libvirtDomainStr); err != nil {
			log.Error().Err(err).Str("operation", "reboot_vm").Msg("libvirt error")
			common.SendError(c, common.NewInternalServerError("failed to reboot server"))
			return
		}
	} else if _, ok := req["os-stop"]; ok {
		if err := svc.vmManager.StopVM(ctx, libvirtDomainStr); err != nil {
			log.Error().Err(err).Str("operation", "stop_vm").Msg("libvirt error")
			common.SendError(c, common.NewInternalServerError("failed to stop server"))
			return
		}
	} else if _, ok := req["os-start"]; ok {
		if err := svc.vmManager.StartVM(ctx, libvirtDomainStr); err != nil {
			log.Error().Err(err).Str("operation", "start_vm").Msg("libvirt error")
			common.SendError(c, common.NewInternalServerError("failed to start server"))
			return
		}
	} else {
		common.SendError(c, common.NewBadRequestError("unknown action"))
		return
	}

	c.Status(http.StatusAccepted)
}

// ListFlavors lists all flavors (brief)
func (svc *Service) ListFlavors(c *gin.Context) {
	rows, err := svc.activeDB().Query(c.Request.Context(),
		"SELECT id, name FROM flavors WHERE is_public = true ORDER BY ram_mb",
	)
	if err != nil {
		log.Error().Err(err).Str("operation", "list_flavors").Msg("database error")
		common.SendError(c, common.NewInternalServerError("failed to list flavors"))
		return
	}
	defer rows.Close()

	var flavors []gin.H
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			log.Warn().Err(err).Msg("failed to scan flavor row")
			continue
		}
		flavors = append(flavors, gin.H{
			"id":   id,
			"name": name,
			"links": []gin.H{
				{"rel": "self", "href": fmt.Sprintf("%s/v2.1/flavors/%s", common.BaseURL(c, 8774), id)},
			},
		})
	}

	if flavors == nil {
		flavors = []gin.H{}
	}

	c.JSON(http.StatusOK, gin.H{"flavors": flavors})
}

// ListFlavorsDetail lists all flavors (detailed)
func (svc *Service) ListFlavorsDetail(c *gin.Context) {
	ctx := c.Request.Context()

	// Parse pagination parameters
	marker := c.Query("marker")
	limitStr := c.Query("limit")

	// Build query with pagination support
	query := "SELECT id, name, vcpus, ram_mb, disk_gb, is_public FROM flavors WHERE is_public = true"
	var args []interface{}
	argIndex := 1

	// Add marker filter using created_at cursor (avoids broken UUID lexicographic ordering)
	if marker != "" {
		var markerTime interface{}
		lookupErr := svc.activeDB().QueryRow(ctx,
			"SELECT created_at FROM flavors WHERE id = $1", marker).Scan(&markerTime)
		if lookupErr == nil {
			query += fmt.Sprintf(" AND created_at > $%d", argIndex)
			args = append(args, markerTime)
			argIndex++
		}
		// If the marker flavor is not found, ignore the marker and return from the start.
	}

	query += " ORDER BY created_at, id"

	// Add limit
	if limitStr != "" {
		if limit, err := strconv.Atoi(limitStr); err == nil && limit > 0 {
			query += fmt.Sprintf(" LIMIT $%d", argIndex)
			args = append(args, limit)
		}
	}

	rows, err := svc.activeDB().Query(ctx, query, args...)
	if err != nil {
		log.Error().Err(err).Str("operation", "list_flavors_detail").Msg("database error")
		common.SendError(c, common.NewInternalServerError("failed to list flavors"))
		return
	}
	defer rows.Close()

	var flavors []gin.H
	for rows.Next() {
		var id, name string
		var vcpus, ramMB, diskGB int
		var isPublic bool

		if err := rows.Scan(&id, &name, &vcpus, &ramMB, &diskGB, &isPublic); err != nil {
			log.Warn().Err(err).Msg("failed to scan flavor detail row")
			continue
		}

		flavors = append(flavors, gin.H{
			"id":                         id,
			"name":                       name,
			"vcpus":                      vcpus,
			"ram":                        ramMB,
			"disk":                       diskGB,
			"OS-FLV-EXT-DATA:ephemeral":  0,
			"OS-FLV-DISABLED:disabled":   false,
			"os-flavor-access:is_public": isPublic,
			"rxtx_factor":                1.0,
			"swap":                       "",
		})
	}

	if flavors == nil {
		flavors = []gin.H{}
	}

	c.JSON(http.StatusOK, gin.H{"flavors": flavors})
}

// GetFlavor returns a single flavor
func (svc *Service) GetFlavor(c *gin.Context) {
	flavorID := c.Param("id")
	ctx := c.Request.Context()

	// Validate ID is not empty
	if flavorID == "" {
		common.SendError(c, common.NewBadRequestError("Flavor ID cannot be empty"))
		return
	}

	// Try cache first
	if svc.cache != nil {
		cacheKey := "flavor:" + flavorID
		var cached gin.H
		if err := svc.cache.Get(ctx, cacheKey, &cached); err == nil {
			// Cache hit
			c.JSON(http.StatusOK, gin.H{"flavor": cached})
			return
		}
	}

	// Cache miss - query database
	var id, name string
	var vcpus, ramMB, diskGB int
	var isPublic bool

	// Support lookup by UUID or name
	err := svc.activeDB().QueryRow(ctx,
		"SELECT id, name, vcpus, ram_mb, disk_gb, is_public FROM flavors WHERE id::text = $1 OR name = $1 LIMIT 1",
		flavorID,
	).Scan(&id, &name, &vcpus, &ramMB, &diskGB, &isPublic)

	if err == pgx.ErrNoRows {
		common.SendError(c, common.NewNotFoundError("flavor"))
		return
	}
	if err != nil {
		log.Error().Err(err).Str("operation", "get_flavor").Msg("database error")
		common.SendError(c, common.NewInternalServerError("failed to get flavor"))
		return
	}

	flavor := gin.H{
		"id":                         id,
		"name":                       name,
		"vcpus":                      vcpus,
		"ram":                        ramMB,
		"disk":                       diskGB,
		"OS-FLV-EXT-DATA:ephemeral":  0,
		"OS-FLV-DISABLED:disabled":   false,
		"os-flavor-access:is_public": isPublic,
		"rxtx_factor":                1.0,
		"swap":                       "",
	}

	// Store in cache (24 hour TTL - flavors rarely change)
	if svc.cache != nil {
		cacheKey := "flavor:" + id
		svc.cache.Set(ctx, cacheKey, flavor, 24*time.Hour)
	}

	c.JSON(http.StatusOK, gin.H{"flavor": flavor})
}

// ListImages - stub (proxy to Glance)
func (svc *Service) ListImages(c *gin.Context) {
	c.JSON(200, gin.H{"images": []gin.H{}})
}

// ListImagesDetail - stub (proxy to Glance)
func (svc *Service) ListImagesDetail(c *gin.Context) {
	c.JSON(200, gin.H{"images": []gin.H{}})
}

// ListHypervisors lists hypervisors (mock for Horizon)
func (svc *Service) ListHypervisors(c *gin.Context) {
	c.JSON(200, gin.H{"hypervisors": []gin.H{
		{
			"id":                  1,
			"hypervisor_hostname": "o3k-node-1",
			"state":               "up",
			"status":              "enabled",
			"hypervisor_type":     "QEMU",
			"hypervisor_version":  2012000,
			"vcpus":               16,
			"memory_mb":           32768,
			"local_gb":            1000,
			"vcpus_used":          0,
			"memory_mb_used":      0,
			"local_gb_used":       0,
			"free_disk_gb":        900,
			"free_ram_mb":         28672,
			"running_vms":         0,
		},
	}})
}

// ListHypervisorsDetail lists hypervisors with details (mock for Horizon)
func (svc *Service) ListHypervisorsDetail(c *gin.Context) {
	cpuInfoJSON := `{"arch":"x86_64","model":"Skylake-Server-IBRS","vendor":"Intel","features":[],"topology":{"cores":8,"threads":2,"sockets":1}}`

	c.JSON(200, gin.H{"hypervisors": []gin.H{
		{
			"id":                  1,
			"hypervisor_hostname": "o3k-node-1",
			"state":               "up",
			"status":              "enabled",
			"vcpus":               16,
			"memory_mb":           32768,
			"local_gb":            1000,
			"vcpus_used":          0,
			"memory_mb_used":      0,
			"local_gb_used":       0,
			"free_disk_gb":        900,
			"free_ram_mb":         28672,
			"hypervisor_type":     "QEMU",
			"hypervisor_version":  2012000,
			"running_vms":         0,
			"cpu_info":            cpuInfoJSON,
		},
	}})
}

// GetHypervisorStatistics returns aggregated hypervisor statistics (for Horizon)
func (svc *Service) GetHypervisorStatistics(c *gin.Context) {
	// Count running instances
	var runningVMs int
	svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT COUNT(*) FROM instances WHERE power_state = 1",
	).Scan(&runningVMs)

	// Return aggregated stats
	c.JSON(200, gin.H{
		"hypervisor_statistics": gin.H{
			"count":              1,
			"current_workload":   0,
			"disk_available_least": 800,
			"free_disk_gb":       900,
			"free_ram_mb":        28672,
			"local_gb":           1000,
			"local_gb_used":      100,
			"memory_mb":          32768,
			"memory_mb_used":     4096,
			"running_vms":        runningVMs,
			"vcpus":              16,
			"vcpus_used":         runningVMs * 2, // Assume 2 vCPUs per VM
		},
	})
}

// ListAvailabilityZones lists availability zones
func (svc *Service) ListAvailabilityZones(c *gin.Context) {
	// Query distinct availability zones from host_aggregates
	rows, err := svc.activeDB().Query(c.Request.Context(),
		"SELECT DISTINCT availability_zone FROM host_aggregates WHERE availability_zone IS NOT NULL AND availability_zone != ''")
	if err != nil {
		log.Error().Err(err).Str("operation", "list_availability_zones").Msg("database error")
		common.SendError(c, common.NewInternalServerError("failed to query availability zones"))
		return
	}
	defer rows.Close()

	var zones []gin.H
	for rows.Next() {
		var zoneName string
		if err := rows.Scan(&zoneName); err != nil {
			log.Warn().Err(err).Msg("failed to scan availability zone row")
			continue
		}
		zones = append(zones, gin.H{
			"zoneName":  zoneName,
			"zoneState": gin.H{"available": true},
			"hosts":     nil,
		})
	}

	// Fallback to "nova" if no aggregates exist
	if len(zones) == 0 {
		zones = append(zones, gin.H{
			"zoneName":  "nova",
			"zoneState": gin.H{"available": true},
			"hosts":     nil,
		})
	}

	c.JSON(200, gin.H{"availabilityZoneInfo": zones})
}

// ListAvailabilityZonesDetail lists availability zones with host details
func (svc *Service) ListAvailabilityZonesDetail(c *gin.Context) {
	// Query availability zones with hosts from host_aggregates
	rows, err := svc.activeDB().Query(c.Request.Context(),
		"SELECT availability_zone, hosts FROM host_aggregates WHERE availability_zone IS NOT NULL AND availability_zone != ''")
	if err != nil {
		log.Error().Err(err).Str("operation", "list_availability_zones_detail").Msg("database error")
		common.SendError(c, common.NewInternalServerError("failed to query availability zones"))
		return
	}
	defer rows.Close()

	// Build zone map: zone_name -> []host
	zoneHosts := make(map[string][]string)
	for rows.Next() {
		var zoneName string
		var hosts []string
		if err := rows.Scan(&zoneName, &hosts); err != nil {
			log.Warn().Err(err).Msg("failed to scan availability zone host row")
			continue
		}
		zoneHosts[zoneName] = append(zoneHosts[zoneName], hosts...)
	}

	var zones []gin.H
	if len(zoneHosts) == 0 {
		// Fallback to "nova" with default host
		zones = append(zones, gin.H{
			"zoneName":  "nova",
			"zoneState": gin.H{"available": true},
			"hosts": gin.H{
				"o3k-compute-1": gin.H{
					"nova-compute": gin.H{
						"active":    true,
						"available": true,
					},
				},
			},
		})
	} else {
		// Build response for each zone
		for zoneName, hosts := range zoneHosts {
			zoneHostsMap := gin.H{}
			for _, host := range hosts {
				if host != "" {
					zoneHostsMap[host] = gin.H{
						"nova-compute": gin.H{
							"active":    true,
							"available": true,
						},
					}
				}
			}
			zones = append(zones, gin.H{
				"zoneName":  zoneName,
				"zoneState": gin.H{"available": true},
				"hosts":     zoneHostsMap,
			})
		}
	}

	c.JSON(200, gin.H{"availabilityZoneInfo": zones})
}

// GetLimits returns compute limits and quota information
func (svc *Service) GetLimits(c *gin.Context) {
	ctx := c.Request.Context()
	projectID := c.GetString("project_id")

	// Query current usage from database
	var instancesUsed, coresUsed, ramUsed int
	if err := svc.activeDB().QueryRow(ctx,
		`SELECT
			COUNT(*),
			COALESCE(SUM(f.vcpus), 0),
			COALESCE(SUM(f.ram_mb), 0)
		FROM instances i
		LEFT JOIN flavors f ON i.flavor_id = f.id
		WHERE i.project_id = $1 AND i.status != 'DELETED'`,
		projectID,
	).Scan(&instancesUsed, &coresUsed, &ramUsed); err != nil {
		instancesUsed, coresUsed, ramUsed = 0, 0, 0
	}

	// Query project quotas from the quotas table (row-per-resource schema).
	// Defaults are used for any resource not explicitly configured.
	quotaDefaults := map[string]int{
		"instances":            100,
		"cores":                200,
		"ram":                  512000,
		"keypairs":             100,
		"server_groups":        10,
		"server_group_members": 10,
		"floatingip":           10,
		"security_groups":      50,
		"security_group_rules": 100,
	}
	quotas := make(map[string]int)
	for k, v := range quotaDefaults {
		quotas[k] = v
	}

	quotaRows, err := svc.activeDB().Query(ctx,
		`SELECT resource, hard_limit FROM quotas WHERE project_id = $1`, projectID)
	if err == nil {
		defer quotaRows.Close()
		for quotaRows.Next() {
			var resource string
			var hardLimit int
			if scanErr := quotaRows.Scan(&resource, &hardLimit); scanErr == nil {
				quotas[resource] = hardLimit
			}
		}
	}

	// Return limits response
	c.JSON(200, gin.H{
		"limits": gin.H{
			"rate": []gin.H{}, // No rate limiting implemented
			"absolute": gin.H{
				// Quota limits from the quotas table (with defaults)
				"maxTotalInstances":     quotas["instances"],
				"maxTotalCores":         quotas["cores"],
				"maxTotalRAMSize":       quotas["ram"],
				"maxTotalKeypairs":      quotas["keypairs"],
				"maxServerMeta":         128,
				"maxPersonality":        5,
				"maxPersonalitySize":    10240,
				"maxServerGroups":       quotas["server_groups"],
				"maxServerGroupMembers": quotas["server_group_members"],
				"maxTotalFloatingIps":   quotas["floatingip"],
				"maxSecurityGroups":     quotas["security_groups"],
				"maxSecurityGroupRules": quotas["security_group_rules"],
				"maxImageMeta":          128,

				// Current usage
				"totalInstancesUsed":      instancesUsed,
				"totalCoresUsed":          coresUsed,
				"totalRAMUsed":            ramUsed,
				"totalFloatingIpsUsed":    0,
				"totalSecurityGroupsUsed": 0,
				"totalServerGroupsUsed":   0,
			},
		},
	})
}

// ListServices returns list of compute services
func (svc *Service) ListServices(c *gin.Context) {
	// Format: "2006-01-02T15:04:05.000000" (without Z, microseconds)
	now := time.Now().Format("2006-01-02T15:04:05.000000")

	// Return list of compute services for Horizon System Info panel
	c.JSON(200, gin.H{
		"services": []gin.H{
			{
				"id":                 1,
				"binary":             "nova-compute",
				"host":               "o3k-compute-1",
				"zone":               "nova",
				"status":             "enabled",
				"state":              "up",
				"updated_at":         now,
				"disabled_reason":    nil,
				"forced_down":        false,
			},
			{
				"id":                 2,
				"binary":             "nova-scheduler",
				"host":               "o3k-controller",
				"zone":               "internal",
				"status":             "enabled",
				"state":              "up",
				"updated_at":         now,
				"disabled_reason":    nil,
				"forced_down":        false,
			},
			{
				"id":                 3,
				"binary":             "nova-conductor",
				"host":               "o3k-controller",
				"zone":               "internal",
				"status":             "enabled",
				"state":              "up",
				"updated_at":         now,
				"disabled_reason":    nil,
				"forced_down":        false,
			},
		},
	})
}

// GetServerMetadata returns metadata for a server (GET /v2.1/servers/:id/metadata)
func (svc *Service) GetServerMetadata(c *gin.Context) {
	serverID := c.Param("id")

	// Check if server exists and belongs to this project
	var exists bool
	err := svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT EXISTS(SELECT 1 FROM instances WHERE id = $1 AND project_id = $2)",
		serverID, c.GetString("project_id"),
	).Scan(&exists)

	if err != nil || !exists {
		common.SendError(c, common.NewNotFoundError("instance"))
		return
	}

	// Fetch metadata
	rows, err := svc.activeDB().Query(c.Request.Context(),
		"SELECT meta_key, meta_value FROM instance_metadata WHERE instance_id = $1",
		serverID,
	)
	if err != nil {
		log.Error().Err(err).Str("operation", "get_server_metadata").Msg("database error")
		common.SendError(c, common.NewInternalServerError("failed to fetch metadata"))
		return
	}
	defer rows.Close()

	metadata := make(map[string]string)
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			log.Error().Err(err).Str("operation", "scan_metadata").Msg("database error")
			common.SendError(c, common.NewInternalServerError("failed to read metadata"))
			return
		}
		metadata[key] = value
	}

	c.JSON(http.StatusOK, gin.H{"metadata": metadata})
}

// UpdateServerMetadata updates/merges server metadata (POST /v2.1/servers/:id/metadata)
func (svc *Service) UpdateServerMetadata(c *gin.Context) {
	serverID := c.Param("id")

	var req struct {
		Metadata map[string]string `json:"metadata" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		common.SendError(c, common.NewBadRequestError("invalid request body"))
		return
	}

	// Check if server exists and belongs to this project
	var exists bool
	err := svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT EXISTS(SELECT 1 FROM instances WHERE id = $1 AND project_id = $2)",
		serverID, c.GetString("project_id"),
	).Scan(&exists)

	if err != nil || !exists {
		common.SendError(c, common.NewNotFoundError("instance"))
		return
	}

	// Upsert each metadata key-value pair
	for key, value := range req.Metadata {
		_, err := svc.activeDB().Exec(c.Request.Context(),
			`INSERT INTO instance_metadata (instance_id, meta_key, meta_value)
			 VALUES ($1, $2, $3)
			 ON CONFLICT (instance_id, meta_key)
			 DO UPDATE SET meta_value = $3, created_at = CURRENT_TIMESTAMP`,
			serverID, key, value,
		)
		if err != nil {
			log.Error().Err(err).Str("operation", "upsert_metadata").Msg("database error")
			common.SendError(c, common.NewInternalServerError("failed to update metadata"))
			return
		}
	}

	// Fetch and return all metadata
	rows, err := svc.activeDB().Query(c.Request.Context(),
		"SELECT meta_key, meta_value FROM instance_metadata WHERE instance_id = $1",
		serverID,
	)
	if err != nil {
		log.Error().Err(err).Str("operation", "fetch_metadata_after_update").Msg("database error")
		common.SendError(c, common.NewInternalServerError("failed to fetch metadata"))
		return
	}
	defer rows.Close()

	metadata := make(map[string]string)
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			log.Error().Err(err).Str("operation", "scan_metadata_after_update").Msg("database error")
			common.SendError(c, common.NewInternalServerError("failed to read metadata"))
			return
		}
		metadata[key] = value
	}

	c.JSON(http.StatusOK, gin.H{"metadata": metadata})
}

// ResetServerMetadata replaces all server metadata (PUT /v2.1/servers/:id/metadata)
func (svc *Service) ResetServerMetadata(c *gin.Context) {
	serverID := c.Param("id")
	projectID := c.GetString("project_id")

	var req struct {
		Metadata map[string]string `json:"metadata" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		common.SendError(c, common.NewBadRequestError("invalid request body"))
		return
	}

	// Check if server exists and belongs to this project
	var exists bool
	err := svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT EXISTS(SELECT 1 FROM instances WHERE id = $1 AND project_id = $2)",
		serverID, projectID,
	).Scan(&exists)

	if err != nil || !exists {
		common.SendError(c, common.NewNotFoundError("instance"))
		return
	}

	// Delete all existing metadata then insert new metadata atomically
	err = database.WithTx(c.Request.Context(), func(tx pgx.Tx) error {
		if _, err := tx.Exec(c.Request.Context(),
			"DELETE FROM instance_metadata WHERE instance_id = $1",
			serverID,
		); err != nil {
			return fmt.Errorf("clear_metadata: %w", err)
		}
		for key, value := range req.Metadata {
			if _, err := tx.Exec(c.Request.Context(),
				`INSERT INTO instance_metadata (instance_id, meta_key, meta_value)
				 VALUES ($1, $2, $3)`,
				serverID, key, value,
			); err != nil {
				return fmt.Errorf("insert_metadata: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		log.Error().Err(err).Str("operation", "reset_metadata").Msg("database error")
		common.SendError(c, common.NewInternalServerError("failed to reset metadata"))
		return
	}

	c.JSON(http.StatusOK, gin.H{"metadata": req.Metadata})
}

// RebuildInstanceAction handles the rebuild action
func (svc *Service) RebuildInstanceAction(c *gin.Context, rebuildData interface{}) {
	instanceID := c.Param("id")
	projectID := c.GetString("project_id")

	rebuildMap, ok := rebuildData.(map[string]interface{})
	if !ok {
		common.SendError(c, common.NewBadRequestError("invalid rebuild data"))
		return
	}

	imageRef, _ := rebuildMap["imageRef"].(string)
	name, _ := rebuildMap["name"].(string)

	if imageRef == "" {
		common.SendError(c, common.NewBadRequestError("imageRef is required"))
		return
	}

	// Update instance in database
	now := time.Now()
	_, err := svc.activeDB().Exec(c.Request.Context(),
		"UPDATE instances SET image_id = $1, name = COALESCE(NULLIF($2, ''), name), status = $3, updated_at = $4 WHERE id = $5 AND project_id = $6",
		imageRef, name, "REBUILD", now, instanceID, projectID,
	)
	if err != nil {
		log.Error().Err(err).Str("operation", "rebuild_instance").Msg("database error")
		common.SendError(c, common.NewInternalServerError("failed to rebuild instance"))
		return
	}

	// In stub mode, simulate rebuild completion
	if svc.libvirtMode == "stub" {
		svc.wg.Add(1)
		go func() {
			defer svc.wg.Done()
			select {
			case <-time.After(2 * time.Second):
			case <-svc.ctx.Done():
				return
			}
			ctx, cancel := context.WithTimeout(svc.ctx, 5*time.Second)
			defer cancel()
			svc.activeDB().Exec(ctx,
				"UPDATE instances SET status = $1, updated_at = $2 WHERE id = $3 AND project_id = $4",
				"ACTIVE", time.Now(), instanceID, projectID)
		}()
	}

	// Return updated server details
	var server gin.H
	var flavorID, userID, serverName, status string
	var nullImageID sql.NullString
	var createdAt, updatedAt time.Time
	err = svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT id, name, flavor_id, image_id, user_id, status, created_at, updated_at FROM instances WHERE id = $1 AND project_id = $2",
		instanceID, projectID,
	).Scan(&instanceID, &serverName, &flavorID, &nullImageID, &userID, &status, &createdAt, &updatedAt)

	if err != nil {
		log.Error().Err(err).Str("operation", "get_instance_after_rebuild").Msg("database error")
		common.SendError(c, common.NewInternalServerError("failed to get instance after rebuild"))
		return
	}

	imageResponse := gin.H{"id": ""}
	if nullImageID.Valid {
		imageResponse["id"] = nullImageID.String
	}

	server = gin.H{
		"id":         instanceID,
		"name":       serverName,
		"status":     status,
		"tenant_id":  projectID,
		"user_id":    userID,
		"created":    createdAt.Format(time.RFC3339),
		"updated":    updatedAt.Format(time.RFC3339),
		"image":      imageResponse,
		"flavor": gin.H{
			"id": flavorID,
		},
	}

	c.JSON(http.StatusOK, gin.H{"server": server})
}

// RescueInstanceAction handles the rescue action
func (svc *Service) RescueInstanceAction(c *gin.Context, rescueData interface{}) {
	instanceID := c.Param("id")
	projectID := c.GetString("project_id")

	// Update instance status to RESCUE
	now := time.Now()
	_, err := svc.activeDB().Exec(c.Request.Context(),
		"UPDATE instances SET status = $1, updated_at = $2 WHERE id = $3 AND project_id = $4",
		"RESCUE", now, instanceID, projectID,
	)
	if err != nil {
		log.Error().Err(err).Str("operation", "rescue_instance").Msg("database error")
		common.SendError(c, common.NewInternalServerError("failed to rescue instance"))
		return
	}

	// Return admin password (in real OpenStack this would be a generated rescue password)
	c.JSON(http.StatusOK, gin.H{
		"adminPass": common.GeneratePassword(16),
	})
}

// CreateImageAction handles the createImage action
func (svc *Service) CreateImageAction(c *gin.Context, createImageData interface{}) {
	projectID := c.GetString("project_id")

	createImageMap, ok := createImageData.(map[string]interface{})
	if !ok {
		common.SendError(c, common.NewBadRequestError("invalid createImage data"))
		return
	}

	imageName, _ := createImageMap["name"].(string)
	if imageName == "" {
		common.SendError(c, common.NewBadRequestError("name is required"))
		return
	}

	// Get metadata if provided
	metadata := make(map[string]string)
	if metadataRaw, ok := createImageMap["metadata"].(map[string]interface{}); ok {
		for k, v := range metadataRaw {
			if vStr, ok := v.(string); ok {
				metadata[k] = vStr
			}
		}
	}

	// Create image record in database
	imageID := uuid.New().String()
	now := time.Now()
	_, err := svc.activeDB().Exec(c.Request.Context(), `
		INSERT INTO images (id, name, project_id, status, container_format, disk_format, size_bytes, visibility, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`, imageID, imageName, projectID, "active", "bare", "qcow2", 0, "private", now, now)

	if err != nil {
		log.Error().Err(err).Str("operation", "create_image").Msg("database error")
		common.SendError(c, common.NewInternalServerError("failed to create image"))
		return
	}

	// Store metadata if provided
	for key, value := range metadata {
		svc.activeDB().Exec(c.Request.Context(), `
			INSERT INTO image_properties (image_id, name, value)
			VALUES ($1, $2, $3)
		`, imageID, key, value)
	}

	// Return Location header with image URL
	imageLocation := fmt.Sprintf("%s/v2/images/%s", common.BaseURL(c, 9292), imageID)
	c.Header("Location", imageLocation)
	c.Status(http.StatusAccepted)
}

// PauseInstanceAction handles the pause action
func (svc *Service) PauseInstanceAction(c *gin.Context) {
	instanceID := c.Param("id")
	projectID := c.GetString("project_id")

	// Update instance status to PAUSED
	_, err := svc.activeDB().Exec(c.Request.Context(),
		"UPDATE instances SET status = $1, updated_at = $2 WHERE id = $3 AND project_id = $4",
		"PAUSED", time.Now(), instanceID, projectID,
	)
	if err != nil {
		log.Error().Err(err).Str("operation", "pause_instance").Msg("database error")
		common.SendError(c, common.NewInternalServerError("failed to pause instance"))
		return
	}

	c.Status(http.StatusAccepted)
}

// UnpauseInstanceAction handles the unpause action
func (svc *Service) UnpauseInstanceAction(c *gin.Context) {
	instanceID := c.Param("id")
	projectID := c.GetString("project_id")

	// Update instance status to ACTIVE
	_, err := svc.activeDB().Exec(c.Request.Context(),
		"UPDATE instances SET status = $1, updated_at = $2 WHERE id = $3 AND project_id = $4",
		"ACTIVE", time.Now(), instanceID, projectID,
	)
	if err != nil {
		log.Error().Err(err).Str("operation", "unpause_instance").Msg("database error")
		common.SendError(c, common.NewInternalServerError("failed to unpause instance"))
		return
	}

	c.Status(http.StatusAccepted)
}

// LockInstanceAction handles the lock action
func (svc *Service) LockInstanceAction(c *gin.Context) {
	instanceID := c.Param("id")
	projectID := c.GetString("project_id")

	// Update instance locked status
	_, err := svc.activeDB().Exec(c.Request.Context(),
		"UPDATE instances SET locked = true, updated_at = $1 WHERE id = $2 AND project_id = $3",
		time.Now(), instanceID, projectID,
	)
	if err != nil {
		log.Error().Err(err).Str("operation", "lock_instance").Msg("database error")
		common.SendError(c, common.NewInternalServerError("failed to lock instance"))
		return
	}

	c.Status(http.StatusAccepted)
}

// UnlockInstanceAction handles the unlock action
func (svc *Service) UnlockInstanceAction(c *gin.Context) {
	instanceID := c.Param("id")
	projectID := c.GetString("project_id")

	// Update instance locked status
	_, err := svc.activeDB().Exec(c.Request.Context(),
		"UPDATE instances SET locked = false, updated_at = $1 WHERE id = $2 AND project_id = $3",
		time.Now(), instanceID, projectID,
	)
	if err != nil {
		log.Error().Err(err).Str("operation", "unlock_instance").Msg("database error")
		common.SendError(c, common.NewInternalServerError("failed to unlock instance"))
		return
	}

	c.Status(http.StatusAccepted)
}

// ForceDeleteInstanceAction handles the forceDelete action
func (svc *Service) ForceDeleteInstanceAction(c *gin.Context) {
	instanceID := c.Param("id")
	projectID := c.GetString("project_id")

	// In stub mode, just delete from database
	if svc.libvirtMode == "stub" || svc.vmManager == nil {
		_, err := svc.activeDB().Exec(c.Request.Context(),
			"DELETE FROM instances WHERE id = $1 AND project_id = $2",
			instanceID, projectID,
		)
		if err != nil {
			log.Error().Err(err).Str("operation", "force_delete_instance").Msg("database error")
			common.SendError(c, common.NewInternalServerError("failed to delete instance"))
			return
		}
		c.Status(http.StatusNoContent)
		return
	}

	// In real mode, destroy VM then delete from database
	var libvirtDomainID interface{}
	err := svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT libvirt_domain_id FROM instances WHERE id = $1 AND project_id = $2",
		instanceID, projectID,
	).Scan(&libvirtDomainID)

	if err == nil && libvirtDomainID != nil {
		domainStr := fmt.Sprintf("%v", libvirtDomainID)
		if domainStr != "" {
			ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
			defer cancel()
			svc.vmManager.DeleteVM(ctx, domainStr)
		}
	}

	_, err = svc.activeDB().Exec(c.Request.Context(),
		"DELETE FROM instances WHERE id = $1 AND project_id = $2",
		instanceID, projectID,
	)
	if err != nil {
		log.Error().Err(err).Str("operation", "force_delete_instance_db").Msg("database error")
		common.SendError(c, common.NewInternalServerError("failed to delete instance"))
		return
	}

	c.Status(http.StatusNoContent)
}
