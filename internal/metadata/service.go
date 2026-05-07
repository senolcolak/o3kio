package metadata

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"

	"github.com/cobaltcore-dev/o3k/internal/common"
	"github.com/cobaltcore-dev/o3k/internal/database"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog/log"
)

// Service handles EC2-compatible metadata service endpoints
type Service struct {
	// In production, this would run on 169.254.169.254
	// For testing, it runs on localhost:8775
	bindAddr string
	db       database.DBIF
	testMode bool
}

// NewService creates a new metadata service
func NewService(bindAddr string, testMode bool) *Service {
	return &Service{
		bindAddr: bindAddr,
		testMode: testMode,
	}
}

// NewServiceWithDB creates a new metadata service with an injected DB (for testing).
func NewServiceWithDB(db database.DBIF, bindAddr string) *Service {
	return &Service{
		bindAddr: bindAddr,
		db:       db,
		testMode: true,
	}
}

// activeDB returns the injected DB or falls back to the global.
func (svc *Service) activeDB() database.DBIF {
	if svc.db != nil {
		return svc.db
	}
	return database.DB
}

// RegisterRoutes registers metadata service routes
func (svc *Service) RegisterRoutes(r *gin.RouterGroup) {
	// EC2-compatible metadata API (OpenStack follows this format)
	// Cloud-init tries these paths in order:
	//   /openstack/latest/meta_data.json
	//   /2009-04-04/meta-data/

	// OpenStack-style metadata
	openstack := r.Group("/openstack")
	{
		openstack.GET("/latest/meta_data.json", svc.GetMetaDataJSON)
		openstack.GET("/latest/user_data", svc.GetUserData)
		openstack.GET("/latest/network_data.json", svc.GetNetworkDataJSON)
		openstack.GET("/latest/vendor_data.json", svc.GetVendorDataJSON)
	}

	// EC2-style metadata (for compatibility)
	ec2 := r.Group("/2009-04-04")
	{
		ec2.GET("/meta-data/", svc.GetMetaDataRoot)
		ec2.GET("/meta-data/:key", svc.GetMetaDataKey)
		ec2.GET("/user-data", svc.GetUserData)
	}

	// Version discovery
	r.GET("/", svc.ListVersions)
}

// instanceFromRequest looks up the instance for the current request.
// In test mode, the X-Instance-ID header is accepted directly.
// In production, the source IP is used to look up the port's device_id.
func (svc *Service) instanceFromRequest(c *gin.Context) (string, error) {
	if svc.testMode {
		if instanceID := c.GetHeader("X-Instance-ID"); instanceID != "" {
			return instanceID, nil
		}
	}

	clientIP := c.ClientIP()
	var instanceID string
	err := svc.activeDB().QueryRow(c.Request.Context(),
		`SELECT device_id FROM ports WHERE fixed_ips @> jsonb_build_array(jsonb_build_object('ip_address', $1::text)) LIMIT 1`,
		clientIP,
	).Scan(&instanceID)
	if err != nil {
		return "", fmt.Errorf("no instance found for IP %s", clientIP)
	}
	return instanceID, nil
}

// GetMetaDataJSON returns instance metadata in JSON format (OpenStack style)
func (svc *Service) GetMetaDataJSON(c *gin.Context) {
	instanceID, err := svc.instanceFromRequest(c)
	if err != nil {
		common.SendError(c, common.NewNotFoundError("instance"))
		return
	}

	var name, hostname, projectID, userID string
	var uuid string
	err = svc.activeDB().QueryRow(c.Request.Context(), `
		SELECT id, name, name, project_id, user_id
		FROM instances
		WHERE id = $1
	`, instanceID).Scan(&uuid, &name, &hostname, &projectID, &userID)

	if err == pgx.ErrNoRows {
		common.SendError(c, common.NewNotFoundError("instance"))
		return
	}
	if err != nil {
		log.Error().Err(err).Str("operation", "get_metadata_json").Str("instance_id", instanceID).Msg("failed to query instance")
		common.SendError(c, common.NewInternalServerError("failed to get instance metadata"))
		return
	}

	// Build metadata structure
	metadata := gin.H{
		"uuid":              uuid,
		"name":              name,
		"hostname":          hostname,
		"project_id":        projectID,
		"availability_zone": "nova", // Default AZ
		"launch_index":      0,
		"meta":              gin.H{}, // Custom metadata key-value pairs
		"public_keys":       gin.H{}, // SSH keys
	}

	// Fetch custom metadata
	rows, err := svc.activeDB().Query(c.Request.Context(), `
		SELECT meta_key, meta_value
		FROM instance_metadata
		WHERE instance_id = $1
	`, instanceID)
	if err == nil {
		defer rows.Close()
		metaMap := make(map[string]string)
		for rows.Next() {
			var key, value string
			if err := rows.Scan(&key, &value); err == nil {
				metaMap[key] = value
			}
		}
		// rows.Err() non-critical for metadata
		if len(metaMap) > 0 {
			metadata["meta"] = metaMap
		}
	}

	// Fetch SSH keys
	keyRows, err := svc.activeDB().Query(c.Request.Context(), `
		SELECT k.name, k.public_key
		FROM keypairs k
		WHERE k.user_id = $1
	`, userID)
	if err == nil {
		defer keyRows.Close()
		keysMap := make(map[string]string)
		for keyRows.Next() {
			var name, pubkey string
			if err := keyRows.Scan(&name, &pubkey); err == nil {
				keysMap[name] = pubkey
			}
		}
		if len(keysMap) > 0 {
			metadata["public_keys"] = keysMap
		}
	}

	c.JSON(http.StatusOK, metadata)
}

// GetUserData returns cloud-init user-data
func (svc *Service) GetUserData(c *gin.Context) {
	instanceID, err := svc.instanceFromRequest(c)
	if err != nil {
		c.String(http.StatusNotFound, "")
		return
	}

	var userData string
	err = svc.activeDB().QueryRow(c.Request.Context(), `
		SELECT userdata
		FROM instance_userdata
		WHERE instance_id = $1
	`, instanceID).Scan(&userData)

	if err == pgx.ErrNoRows {
		// No user-data is not an error, just return empty
		c.String(http.StatusOK, "")
		return
	}
	if err != nil {
		c.String(http.StatusInternalServerError, "")
		return
	}

	// User-data can be shell script, cloud-config, etc.
	c.String(http.StatusOK, userData)
}

// GetNetworkDataJSON returns network configuration in JSON format
func (svc *Service) GetNetworkDataJSON(c *gin.Context) {
	instanceID, err := svc.instanceFromRequest(c)
	if err != nil {
		common.SendError(c, common.NewNotFoundError("instance"))
		return
	}

	// Query all ports for this instance
	rows, err := svc.activeDB().Query(c.Request.Context(), `
		SELECT p.id, p.mac_address, p.fixed_ips, p.network_id, n.name, s.cidr, s.gateway_ip, s.dns_nameservers
		FROM ports p
		JOIN networks n ON p.network_id = n.id
		LEFT JOIN subnets s ON p.network_id = s.network_id
		WHERE p.device_id = $1
		ORDER BY p.created_at
	`, instanceID)

	if err != nil {
		log.Error().Err(err).Str("operation", "get_network_data_json").Str("instance_id", instanceID).Msg("failed to query instance ports")
		common.SendError(c, common.NewInternalServerError("failed to get network data"))
		return
	}
	defer rows.Close()

	links := []gin.H{}
	networks := []gin.H{}

	linkID := 0
	for rows.Next() {
		var portID, mac, networkID, networkName, cidr, gateway string
		var fixedIPsJSON []byte
		var dnsServers []string

		if err := rows.Scan(&portID, &mac, &fixedIPsJSON, &networkID, &networkName, &cidr, &gateway, &dnsServers); err != nil {
			continue
		}

		// Extract IP from JSONB fixed_ips array: [{"ip_address": "...", "subnet_id": "..."}]
		ip := ""
		var fixedIPs []map[string]any
		if err := json.Unmarshal(fixedIPsJSON, &fixedIPs); err == nil && len(fixedIPs) > 0 {
			if v, ok := fixedIPs[0]["ip_address"].(string); ok {
				ip = v
			}
		}

		// Create link (interface)
		links = append(links, gin.H{
			"id":                   fmt.Sprintf("tap%d", linkID),
			"type":                 "bridge",
			"ethernet_mac_address": mac,
			"mtu":                  1500,
		})

		// Create network config
		netConfig := gin.H{
			"id":         fmt.Sprintf("network%d", linkID),
			"link":       fmt.Sprintf("tap%d", linkID),
			"type":       "ipv4",
			"ip_address": ip,
			"netmask":    prefixToNetmask(cidr),
		}

		if gateway != "" {
			netConfig["gateway"] = gateway
		}

		if len(dnsServers) > 0 {
			netConfig["dns_nameservers"] = dnsServers
		}

		networks = append(networks, netConfig)
		linkID++
	}
	if err := rows.Err(); err != nil {
		log.Error().Err(err).Str("operation", "get_network_data_json").Msg("rows iteration error")
		common.SendError(c, common.NewInternalServerError("failed to get network data"))
		return
	}

	// Return network_data.json structure
	c.JSON(http.StatusOK, gin.H{
		"links":    links,
		"networks": networks,
		"services": []gin.H{},
	})
}

// GetVendorDataJSON returns vendor-specific data (usually empty)
func (svc *Service) GetVendorDataJSON(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{})
}

// GetMetaDataRoot returns list of available metadata keys (EC2 style)
func (svc *Service) GetMetaDataRoot(c *gin.Context) {
	keys := []string{
		"hostname",
		"instance-id",
		"instance-type",
		"local-ipv4",
		"public-keys/",
	}
	c.String(http.StatusOK, strings.Join(keys, "\n"))
}

// GetMetaDataKey returns a specific metadata key (EC2 style)
func (svc *Service) GetMetaDataKey(c *gin.Context) {
	key := c.Param("key")
	instanceID, err := svc.instanceFromRequest(c)
	if err != nil {
		c.String(http.StatusNotFound, "")
		return
	}

	switch key {
	case "instance-id":
		c.String(http.StatusOK, instanceID)
	case "hostname":
		var hostname string
		err := svc.activeDB().QueryRow(c.Request.Context(), `
			SELECT name FROM instances WHERE id = $1
		`, instanceID).Scan(&hostname)
		if err == nil {
			c.String(http.StatusOK, hostname)
		} else {
			c.String(http.StatusNotFound, "")
		}
	case "instance-type":
		var flavorName string
		err := svc.activeDB().QueryRow(c.Request.Context(), `
			SELECT f.name
			FROM instances i
			JOIN flavors f ON i.flavor_id = f.id
			WHERE i.id = $1
		`, instanceID).Scan(&flavorName)
		if err == nil {
			c.String(http.StatusOK, flavorName)
		} else {
			c.String(http.StatusNotFound, "")
		}
	case "local-ipv4":
		var fixedIPsJSON []byte
		err := svc.activeDB().QueryRow(c.Request.Context(), `
			SELECT fixed_ips FROM ports WHERE device_id = $1 LIMIT 1
		`, instanceID).Scan(&fixedIPsJSON)
		if err == nil {
			ip := ""
			var fixedIPs []map[string]any
			if err := json.Unmarshal(fixedIPsJSON, &fixedIPs); err == nil && len(fixedIPs) > 0 {
				if v, ok := fixedIPs[0]["ip_address"].(string); ok {
					ip = v
				}
			}
			c.String(http.StatusOK, ip)
		} else {
			c.String(http.StatusNotFound, "")
		}
	default:
		c.String(http.StatusNotFound, "")
	}
}

// ListVersions returns available metadata API versions
func (svc *Service) ListVersions(c *gin.Context) {
	versions := []string{
		"2009-04-04",
		"openstack",
	}
	c.String(http.StatusOK, strings.Join(versions, "\n"))
}

// prefixToNetmask converts a CIDR string (e.g. "10.0.0.0/24") to a dotted netmask
// (e.g. "255.255.255.0"). Returns "255.255.255.0" on parse error.
func prefixToNetmask(cidr string) string {
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return "255.255.255.0"
	}
	m := ipNet.Mask
	return fmt.Sprintf("%d.%d.%d.%d", m[0], m[1], m[2], m[3])
}
