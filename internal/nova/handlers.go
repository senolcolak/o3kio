package nova

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/cobaltcore-dev/o3k/internal/database"
	"github.com/cobaltcore-dev/o3k/pkg/hypervisor"
)

// Service handles Nova API endpoints
type Service struct {
	libvirtURI  string
	libvirtMode string
	vmManager   *hypervisor.VMManager
}

// NewService creates a new Nova service
func NewService(libvirtURI, libvirtMode string) *Service {
	return &Service{
		libvirtURI:  libvirtURI,
		libvirtMode: libvirtMode,
	}
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
		v21.DELETE("/servers/:id", svc.DeleteServer)
		v21.POST("/servers/:id/action", svc.ServerAction)
		v21.GET("/servers/:id/diagnostics", svc.GetServerDiagnostics)
		v21.GET("/servers/:id/os-instance-actions", svc.ListInstanceActions)
		v21.GET("/servers/:id/os-instance-actions/:request_id", svc.GetInstanceAction)

		// Server metadata
		v21.GET("/servers/:id/metadata", svc.GetServerMetadata)
		v21.POST("/servers/:id/metadata", svc.UpdateServerMetadata)
		v21.PUT("/servers/:id/metadata", svc.ResetServerMetadata)

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
				"version":     "2.79",
				"min_version": "2.1",
				"links": []gin.H{
					{"rel": "self", "href": "http://localhost:8774/v2.1"},
				},
			},
		},
	})
}

// GetVersion returns version details
func (svc *Service) GetVersion(c *gin.Context) {
	c.Header("OpenStack-API-Version", "compute 2.79")
	c.Header("OpenStack-API-Minimum-Version", "compute 2.1")
	c.JSON(200, gin.H{
		"version": gin.H{
			"id":          "v2.1",
			"status":      "CURRENT",
			"version":     "2.79",
			"min_version": "2.1",
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
	var req CreateServerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{
			"message": "invalid request body",
			"code":    400,
			"title":   "Bad Request",
		}})
		return
	}

	projectID := c.GetString("project_id")
	userID := c.GetString("user_id")

	// Fetch flavor (support lookup by UUID or name)
	var flavor database.Flavor
	err := database.DB.QueryRow(c.Request.Context(),
		"SELECT id, name, vcpus, ram_mb, disk_gb FROM flavors WHERE id::text = $1 OR name = $1 LIMIT 1",
		req.Server.FlavorRef,
	).Scan(&flavor.ID, &flavor.Name, &flavor.VCPUs, &flavor.RAMMB, &flavor.DiskGB)

	if err == pgx.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{
			"message": "flavor not found",
			"code":    404,
			"title":   "Not Found",
		}})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Check quotas before creating instance
	if err := CheckQuota(c, "instances", 1); err != nil {
		if _, ok := err.(*QuotaExceededError); ok {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": gin.H{
				"message": "Quota exceeded for resource: instances",
				"code":    413,
				"title":   "Request Entity Too Large",
			}})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Check cores quota
	if err := CheckQuota(c, "cores", flavor.VCPUs); err != nil {
		if _, ok := err.(*QuotaExceededError); ok {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": gin.H{
				"message": "Quota exceeded for resource: cores",
				"code":    413,
				"title":   "Request Entity Too Large",
			}})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Check RAM quota
	if err := CheckQuota(c, "ram", flavor.RAMMB); err != nil {
		if _, ok := err.(*QuotaExceededError); ok {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": gin.H{
				"message": "Quota exceeded for resource: ram",
				"code":    413,
				"title":   "Request Entity Too Large",
			}})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
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

	_, err = database.DB.Exec(c.Request.Context(), `
		INSERT INTO instances (id, name, project_id, user_id, flavor_id, image_id, status, power_state, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`, instanceID, req.Server.Name, projectID, userID, flavor.ID, imageID, "BUILD", 0, now, now)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Log instance action
	requestID := uuid.New().String()
	_, _ = database.DB.Exec(c.Request.Context(), `
		INSERT INTO instance_actions (instance_id, action, request_id, user_id, project_id, start_time, message)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, instanceID, "create", requestID, userID, projectID, now, "Instance created")

	// Create VM asynchronously (or synchronously if libvirt is available)
	if svc.vmManager != nil {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			// Generate VM XML
			spec := hypervisor.VMSpec{
				UUID:      instanceID,
				Name:      fmt.Sprintf("instance-%s", instanceID[:8]),
				VCPUs:     flavor.VCPUs,
				MemoryMB:  flavor.RAMMB,
				DiskGB:    flavor.DiskGB,
				ImagePath: fmt.Sprintf("/var/lib/o3k/images/%s.qcow2", req.Server.ImageRef),
				Networks:  []hypervisor.NetworkConfig{}, // TODO: Populate from Neutron
			}

			xml := hypervisor.GenerateVMXML(spec)

			// Create VM
			libvirtUUID, err := svc.vmManager.CreateVM(ctx, xml)
			if err != nil {
				// Update instance status to ERROR
				database.DB.Exec(context.Background(),
					"UPDATE instances SET status = $1, updated_at = $2 WHERE id = $3",
					"ERROR", time.Now(), instanceID)
				return
			}

			// Update instance with libvirt UUID
			database.DB.Exec(context.Background(), `
				UPDATE instances
				SET status = $1, power_state = $2, libvirt_domain_id = $3, launched_at = $4, updated_at = $5
				WHERE id = $6
			`, "ACTIVE", 1, libvirtUUID, time.Now(), time.Now(), instanceID)
		}()
	}

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
			"adminPass":  "generated-password",
		},
	})
}

// ListServers lists all servers (brief)
func (svc *Service) ListServers(c *gin.Context) {
	projectID := c.GetString("project_id")

	rows, err := database.DB.Query(c.Request.Context(),
		"SELECT id, name FROM instances WHERE project_id = $1 ORDER BY created_at DESC",
		projectID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var servers []gin.H
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			continue
		}
		servers = append(servers, gin.H{
			"id":   id,
			"name": name,
			"links": []gin.H{
				{"rel": "self", "href": fmt.Sprintf("http://localhost:8774/v2.1/servers/%s", id)},
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

	rows, err := database.DB.Query(c.Request.Context(), `
		SELECT i.id, i.name, i.status, i.power_state, i.project_id, i.user_id,
		       i.flavor_id, i.image_id, i.created_at, i.updated_at, i.launched_at,
		       f.vcpus, f.ram_mb, f.disk_gb, f.name as flavor_name
		FROM instances i
		LEFT JOIN flavors f ON i.flavor_id = f.id
		WHERE i.project_id = $1
		ORDER BY i.created_at DESC
	`, projectID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var servers []gin.H
	for rows.Next() {
		var id, name, status, projectID, userID, flavorID, imageID, flavorName string
		var powerState, vcpus, ramMB, diskGB int
		var createdAt, updatedAt, launchedAt time.Time

		if err := rows.Scan(&id, &name, &status, &powerState, &projectID, &userID,
			&flavorID, &imageID, &createdAt, &updatedAt, &launchedAt,
			&vcpus, &ramMB, &diskGB, &flavorName); err != nil {
			continue
		}

		servers = append(servers, gin.H{
			"id":         id,
			"name":       name,
			"status":     status,
			"tenant_id":  projectID,
			"user_id":    userID,
			"created":    createdAt.Format(time.RFC3339),
			"updated":    updatedAt.Format(time.RFC3339),
			"OS-EXT-STS:power_state": powerState,
			"OS-SRV-USG:launched_at": launchedAt.Format(time.RFC3339),
			"flavor": gin.H{
				"id":    flavorID,
				"vcpus": vcpus,
				"ram":   ramMB,
				"disk":  diskGB,
			},
			"image": gin.H{"id": imageID},
		})
	}

	if servers == nil {
		servers = []gin.H{}
	}

	c.JSON(http.StatusOK, gin.H{"servers": servers})
}

// GetServer returns details for a single server
func (svc *Service) GetServer(c *gin.Context) {
	instanceID := c.Param("id")
	projectID := c.GetString("project_id")

	var id, name, status, projID string
	var userID, flavorID, imageID sql.NullString
	var powerState int
	var createdAt, updatedAt time.Time

	// Try to find by ID first, then by name
	// Use separate conditions to avoid type mismatch when id is UUID and param might be a name
	err := database.DB.QueryRow(c.Request.Context(), `
		SELECT id, name, status, power_state, project_id, user_id, flavor_id, image_id, created_at, updated_at
		FROM instances
		WHERE project_id = $2 AND (
			(id::text = $1) OR (name = $1)
		)
	`, instanceID, projectID).Scan(&id, &name, &status, &powerState, &projID, &userID, &flavorID, &imageID, &createdAt, &updatedAt)

	if err == pgx.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "instance not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Build response with nullable fields
	response := gin.H{
		"id":                     id,
		"name":                   name,
		"status":                 status,
		"tenant_id":              projID,
		"created":                createdAt.Format(time.RFC3339),
		"updated":                updatedAt.Format(time.RFC3339),
		"OS-EXT-STS:power_state": powerState,
	}

	if userID.Valid {
		response["user_id"] = userID.String
	}
	if flavorID.Valid {
		response["flavor"] = gin.H{"id": flavorID.String}
	}
	if imageID.Valid {
		response["image"] = gin.H{"id": imageID.String}
	}

	c.JSON(http.StatusOK, gin.H{"server": response})
}

// DeleteServer deletes a server
func (svc *Service) DeleteServer(c *gin.Context) {
	instanceID := c.Param("id")
	projectID := c.GetString("project_id")

	// Get libvirt domain ID (support lookup by ID or name)
	var libvirtDomainID string
	err := database.DB.QueryRow(c.Request.Context(),
		"SELECT libvirt_domain_id FROM instances WHERE project_id = $2 AND ((id::text = $1) OR (name = $1))",
		instanceID, projectID,
	).Scan(&libvirtDomainID)

	if err == pgx.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "instance not found"})
		return
	}

	// Delete VM from libvirt (if available)
	if svc.vmManager != nil && libvirtDomainID != "" {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
		defer cancel()

		if err := svc.vmManager.DeleteVM(ctx, libvirtDomainID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	// Delete from database (support lookup by ID or name)
	_, err = database.DB.Exec(c.Request.Context(),
		"DELETE FROM instances WHERE project_id = $2 AND ((id::text = $1) OR (name = $1))",
		instanceID, projectID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.Status(http.StatusNoContent)
}

// ServerAction performs an action on a server
func (svc *Service) ServerAction(c *gin.Context) {
	instanceID := c.Param("id")
	projectID := c.GetString("project_id")

	var req map[string]interface{}
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("ERROR in ServerAction: failed to bind JSON: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	log.Printf("ServerAction: instanceID=%s, projectID=%s, action=%v", instanceID, projectID, req)

	// Handle console actions first (don't require libvirt)
	if vncConsole, ok := req["os-getVNCConsole"]; ok {
		svc.GetVNCConsoleAction(c, vncConsole)
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
	}

	// Get libvirt domain ID for remaining actions (support lookup by ID or name)
	var libvirtDomainID sql.NullString
	err := database.DB.QueryRow(c.Request.Context(),
		"SELECT libvirt_domain_id FROM instances WHERE project_id = $2 AND ((id::text = $1) OR (name = $1))",
		instanceID, projectID,
	).Scan(&libvirtDomainID)

	if err == pgx.ErrNoRows {
		log.Printf("ERROR in ServerAction: instance not found: %s", instanceID)
		c.JSON(http.StatusNotFound, gin.H{"error": "instance not found"})
		return
	}
	if err != nil {
		log.Printf("ERROR in ServerAction: database error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("database error: %v", err)})
		return
	}

	log.Printf("ServerAction: libvirtDomainID.Valid=%v, libvirtDomainID.String=%s, vmManager=%v, mode=%s",
		libvirtDomainID.Valid, libvirtDomainID.String, svc.vmManager != nil, svc.libvirtMode)

	// In stub mode, just update database status (don't call vmManager even if it exists)
	if svc.libvirtMode == "stub" || !libvirtDomainID.Valid || libvirtDomainID.String == "" {
		// Handle actions in stub mode by updating database only
		if _, ok := req["reboot"]; ok {
			// Just mark as rebooting then active
			database.DB.Exec(c.Request.Context(),
				"UPDATE instances SET status = $1, updated_at = $2 WHERE (id::text = $3 OR name = $3) AND project_id = $4",
				"REBOOT", time.Now(), instanceID, projectID)
			go func() {
				time.Sleep(1 * time.Second)
				database.DB.Exec(context.Background(),
					"UPDATE instances SET status = $1, updated_at = $2 WHERE (id::text = $3 OR name = $3) AND project_id = $4",
					"ACTIVE", time.Now(), instanceID, projectID)
			}()
		} else if _, ok := req["os-stop"]; ok {
			database.DB.Exec(c.Request.Context(),
				"UPDATE instances SET status = $1, power_state = $2, updated_at = $3 WHERE (id::text = $4 OR name = $4) AND project_id = $5",
				"SHUTOFF", 4, time.Now(), instanceID, projectID)
		} else if _, ok := req["os-start"]; ok {
			database.DB.Exec(c.Request.Context(),
				"UPDATE instances SET status = $1, power_state = $2, updated_at = $3 WHERE (id::text = $4 OR name = $4) AND project_id = $5",
				"ACTIVE", 1, time.Now(), instanceID, projectID)
		} else {
			c.JSON(http.StatusBadRequest, gin.H{"error": "unknown action"})
			return
		}
		c.Status(http.StatusAccepted)
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	// Handle different actions with real libvirt
	if _, ok := req["reboot"]; ok {
		if err := svc.vmManager.RebootVM(ctx, libvirtDomainID.String); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	} else if _, ok := req["os-stop"]; ok {
		if err := svc.vmManager.StopVM(ctx, libvirtDomainID.String); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	} else if _, ok := req["os-start"]; ok {
		if err := svc.vmManager.StartVM(ctx, libvirtDomainID.String); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	} else {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unknown action"})
		return
	}

	c.Status(http.StatusAccepted)
}

// ListFlavors lists all flavors (brief)
func (svc *Service) ListFlavors(c *gin.Context) {
	rows, err := database.DB.Query(c.Request.Context(),
		"SELECT id, name FROM flavors WHERE is_public = true ORDER BY ram_mb",
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var flavors []gin.H
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			continue
		}
		flavors = append(flavors, gin.H{
			"id":   id,
			"name": name,
			"links": []gin.H{
				{"rel": "self", "href": fmt.Sprintf("http://localhost:8774/v2.1/flavors/%s", id)},
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
	rows, err := database.DB.Query(c.Request.Context(),
		"SELECT id, name, vcpus, ram_mb, disk_gb, is_public FROM flavors WHERE is_public = true ORDER BY ram_mb",
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var flavors []gin.H
	for rows.Next() {
		var id, name string
		var vcpus, ramMB, diskGB int
		var isPublic bool

		if err := rows.Scan(&id, &name, &vcpus, &ramMB, &diskGB, &isPublic); err != nil {
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

	var id, name string
	var vcpus, ramMB, diskGB int
	var isPublic bool

	// Support lookup by UUID or name
	err := database.DB.QueryRow(c.Request.Context(),
		"SELECT id, name, vcpus, ram_mb, disk_gb, is_public FROM flavors WHERE id::text = $1 OR name = $1 LIMIT 1",
		flavorID,
	).Scan(&id, &name, &vcpus, &ramMB, &diskGB, &isPublic)

	if err == pgx.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{
			"message": "flavor not found",
			"code":    404,
			"title":   "Not Found",
		}})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"flavor": gin.H{
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
		},
	})
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
	database.DB.QueryRow(c.Request.Context(),
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
	c.JSON(200, gin.H{"availabilityZoneInfo": []gin.H{
		{
			"zoneName":  "nova",
			"zoneState": gin.H{"available": true},
			"hosts":     nil,
		},
	}})
}

// ListAvailabilityZonesDetail lists availability zones with host details
func (svc *Service) ListAvailabilityZonesDetail(c *gin.Context) {
	// Detail view includes compute hosts per zone
	c.JSON(200, gin.H{"availabilityZoneInfo": []gin.H{
		{
			"zoneName":  "nova",
			"zoneState": gin.H{"available": true},
			"hosts": gin.H{
				"o3k-compute-1": gin.H{
					"nova-compute": gin.H{
						"active":     true,
						"available":  true,
						"updated_at": time.Now(),
					},
				},
			},
		},
	}})
}

// GetLimits returns compute limits and quota information
func (svc *Service) GetLimits(c *gin.Context) {
	projectID := c.GetString("project_id")

	// Query current usage from database
	var instancesUsed, coresUsed, ramUsed int
	database.DB.QueryRow(c.Request.Context(),
		`SELECT
			COUNT(*),
			COALESCE(SUM(vcpus), 0),
			COALESCE(SUM(memory_mb), 0)
		FROM instances
		WHERE project_id = $1 AND status != 'DELETED'`,
		projectID,
	).Scan(&instancesUsed, &coresUsed, &ramUsed)

	// Return limits response
	c.JSON(200, gin.H{
		"limits": gin.H{
			"rate": []gin.H{}, // No rate limiting implemented
			"absolute": gin.H{
				// Quota limits (hardcoded for now, should come from quota table)
				"maxTotalInstances":       100,
				"maxTotalCores":           200,
				"maxTotalRAMSize":         512000, // 500GB in MB
				"maxTotalKeypairs":        100,
				"maxServerMeta":           128,
				"maxPersonality":          5,
				"maxPersonalitySize":      10240,
				"maxServerGroups":         10,
				"maxServerGroupMembers":   10,
				"maxTotalFloatingIps":     10,
				"maxSecurityGroups":       50,
				"maxSecurityGroupRules":   100,
				"maxImageMeta":            128,

				// Current usage
				"totalInstancesUsed":    instancesUsed,
				"totalCoresUsed":        coresUsed,
				"totalRAMUsed":          ramUsed,
				"totalFloatingIpsUsed":  0,
				"totalSecurityGroupsUsed": 0,
				"totalServerGroupsUsed": 0,
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

	// Check if server exists
	var exists bool
	err := database.DB.QueryRow(c.Request.Context(),
		"SELECT EXISTS(SELECT 1 FROM instances WHERE id = $1)",
		serverID,
	).Scan(&exists)

	if err != nil || !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{
			"message": "Instance not found",
			"code":    404,
			"title":   "Not Found",
		}})
		return
	}

	// Fetch metadata
	rows, err := database.DB.Query(c.Request.Context(),
		"SELECT meta_key, meta_value FROM instance_metadata WHERE instance_id = $1",
		serverID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{
			"message": "Failed to fetch metadata",
			"code":    500,
			"title":   "Internal Server Error",
		}})
		return
	}
	defer rows.Close()

	metadata := make(map[string]string)
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{
				"message": "Failed to scan metadata",
				"code":    500,
				"title":   "Internal Server Error",
			}})
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
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{
			"message": "Invalid request body: " + err.Error(),
			"code":    400,
			"title":   "Bad Request",
		}})
		return
	}

	// Check if server exists
	var exists bool
	err := database.DB.QueryRow(c.Request.Context(),
		"SELECT EXISTS(SELECT 1 FROM instances WHERE id = $1)",
		serverID,
	).Scan(&exists)

	if err != nil || !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{
			"message": "Instance not found",
			"code":    404,
			"title":   "Not Found",
		}})
		return
	}

	// Upsert each metadata key-value pair
	for key, value := range req.Metadata {
		_, err := database.DB.Exec(c.Request.Context(),
			`INSERT INTO instance_metadata (instance_id, meta_key, meta_value)
			 VALUES ($1, $2, $3)
			 ON CONFLICT (instance_id, meta_key)
			 DO UPDATE SET meta_value = $3, created_at = CURRENT_TIMESTAMP`,
			serverID, key, value,
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{
				"message": "Failed to update metadata: " + err.Error(),
				"code":    500,
				"title":   "Internal Server Error",
			}})
			return
		}
	}

	// Fetch and return all metadata
	rows, err := database.DB.Query(c.Request.Context(),
		"SELECT meta_key, meta_value FROM instance_metadata WHERE instance_id = $1",
		serverID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{
			"message": "Failed to fetch metadata",
			"code":    500,
			"title":   "Internal Server Error",
		}})
		return
	}
	defer rows.Close()

	metadata := make(map[string]string)
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{
				"message": "Failed to scan metadata",
				"code":    500,
				"title":   "Internal Server Error",
			}})
			return
		}
		metadata[key] = value
	}

	c.JSON(http.StatusOK, gin.H{"metadata": metadata})
}

// ResetServerMetadata replaces all server metadata (PUT /v2.1/servers/:id/metadata)
func (svc *Service) ResetServerMetadata(c *gin.Context) {
	serverID := c.Param("id")

	var req struct {
		Metadata map[string]string `json:"metadata" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{
			"message": "Invalid request body: " + err.Error(),
			"code":    400,
			"title":   "Bad Request",
		}})
		return
	}

	// Check if server exists
	var exists bool
	err := database.DB.QueryRow(c.Request.Context(),
		"SELECT EXISTS(SELECT 1 FROM instances WHERE id = $1)",
		serverID,
	).Scan(&exists)

	if err != nil || !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{
			"message": "Instance not found",
			"code":    404,
			"title":   "Not Found",
		}})
		return
	}

	// Delete all existing metadata for this server
	_, err = database.DB.Exec(c.Request.Context(),
		"DELETE FROM instance_metadata WHERE instance_id = $1",
		serverID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{
			"message": "Failed to clear metadata: " + err.Error(),
			"code":    500,
			"title":   "Internal Server Error",
		}})
		return
	}

	// Insert new metadata
	for key, value := range req.Metadata {
		_, err := database.DB.Exec(c.Request.Context(),
			`INSERT INTO instance_metadata (instance_id, meta_key, meta_value)
			 VALUES ($1, $2, $3)`,
			serverID, key, value,
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{
				"message": "Failed to insert metadata: " + err.Error(),
				"code":    500,
				"title":   "Internal Server Error",
			}})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"metadata": req.Metadata})
}

// RebuildInstanceAction handles the rebuild action
func (svc *Service) RebuildInstanceAction(c *gin.Context, rebuildData interface{}) {
	instanceID := c.Param("id")
	projectID := c.GetString("project_id")

	rebuildMap, ok := rebuildData.(map[string]interface{})
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid rebuild data"})
		return
	}

	imageRef, _ := rebuildMap["imageRef"].(string)
	name, _ := rebuildMap["name"].(string)

	if imageRef == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "imageRef is required"})
		return
	}

	// Update instance in database
	now := time.Now()
	_, err := database.DB.Exec(c.Request.Context(),
		"UPDATE instances SET image_id = $1, name = COALESCE(NULLIF($2, ''), name), status = $3, updated_at = $4 WHERE id = $5 AND project_id = $6",
		imageRef, name, "REBUILD", now, instanceID, projectID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// In stub mode, simulate rebuild completion
	if svc.libvirtMode == "stub" {
		go func() {
			time.Sleep(2 * time.Second)
			database.DB.Exec(context.Background(),
				"UPDATE instances SET status = $1, updated_at = $2 WHERE id = $3 AND project_id = $4",
				"ACTIVE", time.Now(), instanceID, projectID)
		}()
	}

	// Return updated server details
	var server gin.H
	var flavorID, userID, imageID, serverName, status string
	var createdAt, updatedAt time.Time
	err = database.DB.QueryRow(c.Request.Context(),
		"SELECT id, name, flavor_id, image_id, user_id, status, created_at, updated_at FROM instances WHERE id = $1 AND project_id = $2",
		instanceID, projectID,
	).Scan(&instanceID, &serverName, &flavorID, &imageID, &userID, &status, &createdAt, &updatedAt)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	server = gin.H{
		"id":         instanceID,
		"name":       serverName,
		"status":     status,
		"tenant_id":  projectID,
		"user_id":    userID,
		"created":    createdAt.Format(time.RFC3339),
		"updated":    updatedAt.Format(time.RFC3339),
		"image": gin.H{
			"id": imageID,
		},
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
	_, err := database.DB.Exec(c.Request.Context(),
		"UPDATE instances SET status = $1, updated_at = $2 WHERE id = $3 AND project_id = $4",
		"RESCUE", now, instanceID, projectID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Return admin password (in real OpenStack this would be a generated rescue password)
	c.JSON(http.StatusOK, gin.H{
		"adminPass": "rescuepass123",
	})
}

// CreateImageAction handles the createImage action
func (svc *Service) CreateImageAction(c *gin.Context, createImageData interface{}) {
	projectID := c.GetString("project_id")

	createImageMap, ok := createImageData.(map[string]interface{})
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid createImage data"})
		return
	}

	imageName, _ := createImageMap["name"].(string)
	if imageName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
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
	_, err := database.DB.Exec(c.Request.Context(), `
		INSERT INTO images (id, name, project_id, status, container_format, disk_format, size_bytes, visibility, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`, imageID, imageName, projectID, "active", "bare", "qcow2", 0, "private", now, now)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Store metadata if provided
	for key, value := range metadata {
		database.DB.Exec(c.Request.Context(), `
			INSERT INTO image_properties (image_id, name, value)
			VALUES ($1, $2, $3)
		`, imageID, key, value)
	}

	// Return Location header with image URL
	imageLocation := fmt.Sprintf("http://localhost:9292/v2/images/%s", imageID)
	c.Header("Location", imageLocation)
	c.Status(http.StatusAccepted)
}

// PauseInstanceAction handles the pause action
func (svc *Service) PauseInstanceAction(c *gin.Context) {
	instanceID := c.Param("id")
	projectID := c.GetString("project_id")

	// Update instance status to PAUSED
	_, err := database.DB.Exec(c.Request.Context(),
		"UPDATE instances SET status = $1, updated_at = $2 WHERE id = $3 AND project_id = $4",
		"PAUSED", time.Now(), instanceID, projectID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.Status(http.StatusAccepted)
}

// UnpauseInstanceAction handles the unpause action
func (svc *Service) UnpauseInstanceAction(c *gin.Context) {
	instanceID := c.Param("id")
	projectID := c.GetString("project_id")

	// Update instance status to ACTIVE
	_, err := database.DB.Exec(c.Request.Context(),
		"UPDATE instances SET status = $1, updated_at = $2 WHERE id = $3 AND project_id = $4",
		"ACTIVE", time.Now(), instanceID, projectID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.Status(http.StatusAccepted)
}

// LockInstanceAction handles the lock action
func (svc *Service) LockInstanceAction(c *gin.Context) {
	instanceID := c.Param("id")
	projectID := c.GetString("project_id")

	// Update instance locked status
	_, err := database.DB.Exec(c.Request.Context(),
		"UPDATE instances SET locked = true, updated_at = $1 WHERE id = $2 AND project_id = $3",
		time.Now(), instanceID, projectID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.Status(http.StatusAccepted)
}

// UnlockInstanceAction handles the unlock action
func (svc *Service) UnlockInstanceAction(c *gin.Context) {
	instanceID := c.Param("id")
	projectID := c.GetString("project_id")

	// Update instance locked status
	_, err := database.DB.Exec(c.Request.Context(),
		"UPDATE instances SET locked = false, updated_at = $1 WHERE id = $2 AND project_id = $3",
		time.Now(), instanceID, projectID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
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
		_, err := database.DB.Exec(c.Request.Context(),
			"DELETE FROM instances WHERE id = $1 AND project_id = $2",
			instanceID, projectID,
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.Status(http.StatusNoContent)
		return
	}

	// In real mode, destroy VM then delete from database
	var libvirtDomainID sql.NullString
	err := database.DB.QueryRow(c.Request.Context(),
		"SELECT libvirt_domain_id FROM instances WHERE id = $1 AND project_id = $2",
		instanceID, projectID,
	).Scan(&libvirtDomainID)

	if err == nil && libvirtDomainID.Valid && libvirtDomainID.String != "" {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		svc.vmManager.DeleteVM(ctx, libvirtDomainID.String)
	}

	_, err = database.DB.Exec(c.Request.Context(),
		"DELETE FROM instances WHERE id = $1 AND project_id = $2",
		instanceID, projectID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.Status(http.StatusNoContent)
}

