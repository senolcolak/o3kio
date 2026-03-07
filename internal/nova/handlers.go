package nova

import (
	"context"
	"database/sql"
	"fmt"
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

		// Flavors
		v21.GET("/flavors", svc.ListFlavors)
		v21.GET("/flavors/detail", svc.ListFlavorsDetail)
		v21.GET("/flavors/:id", svc.GetFlavor)

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
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{
			"message": "instance not found",
			"code":    404,
			"title":   "Not Found",
		}})
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
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{
			"message": "instance not found",
			"code":    404,
			"title":   "Not Found",
		}})
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

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
	}

	// Get libvirt domain ID for remaining actions
	var libvirtDomainID string
	err := database.DB.QueryRow(c.Request.Context(),
		"SELECT libvirt_domain_id FROM instances WHERE id = $1 AND project_id = $2",
		instanceID, projectID,
	).Scan(&libvirtDomainID)

	if err == pgx.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "instance not found"})
		return
	}

	if svc.vmManager == nil || libvirtDomainID == "" {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "hypervisor not available"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	// Handle different actions
	if _, ok := req["reboot"]; ok {
		if err := svc.vmManager.RebootVM(ctx, libvirtDomainID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	} else if _, ok := req["os-stop"]; ok {
		if err := svc.vmManager.StopVM(ctx, libvirtDomainID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	} else if _, ok := req["os-start"]; ok {
		if err := svc.vmManager.StartVM(ctx, libvirtDomainID); err != nil {
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
		},
	}})
}

// ListHypervisorsDetail lists hypervisors with details (mock for Horizon)
func (svc *Service) ListHypervisorsDetail(c *gin.Context) {
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
			"hypervisor_type":     "QEMU",
			"hypervisor_version":  2012000,
			"running_vms":         0,
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
