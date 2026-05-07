package neutron

import (
	"net/http"

	"github.com/cobaltcore-dev/o3k/internal/common"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

// ListNetworkIPAvailabilities lists IP availability for all networks
func (svc *Service) ListNetworkIPAvailabilities(c *gin.Context) {
	projectID := c.GetString("project_id")

	// Query networks with subnet information
	rows, err := svc.activeDB().Query(c.Request.Context(), `
		SELECT n.id, n.name, n.project_id,
		       COALESCE(COUNT(DISTINCT s.id), 0) as subnet_count
		FROM networks n
		LEFT JOIN subnets s ON n.id = s.network_id
		WHERE n.project_id = $1
		GROUP BY n.id, n.name, n.project_id
		ORDER BY n.created_at DESC
	`, projectID)

	if err != nil {
		log.Error().Err(err).Str("operation", "list_network_ip_availabilities").Msg("failed to query network IP availabilities")
		common.SendError(c, common.NewInternalServerError("failed to list network IP availabilities"))
		return
	}
	defer rows.Close()

	var availabilities []gin.H
	for rows.Next() {
		var networkID, name, projID string
		var subnetCount int

		if err := rows.Scan(&networkID, &name, &projID, &subnetCount); err != nil {
			continue
		}

		// In stub mode, provide approximate availability
		// In real mode, would query actual IP usage from IPAM
		totalIPs := 253 * subnetCount // Rough estimate: /24 subnet = 253 usable IPs
		usedIPs := 0                  // Stub: assume no IPs used

		availabilities = append(availabilities, gin.H{
			"network_id":   networkID,
			"network_name": name,
			"project_id":   projID,
			"total_ips":    totalIPs,
			"used_ips":     usedIPs,
			"subnet_ip_availability": []gin.H{
				// Would list per-subnet availability in real mode
			},
		})
	}
	if err := rows.Err(); err != nil {
		log.Error().Err(err).Str("operation", "list_network_ip_availabilities").Msg("rows iteration error")
		common.SendError(c, common.NewInternalServerError("failed to list network IP availabilities"))
		return
	}

	if availabilities == nil {
		availabilities = []gin.H{}
	}

	c.JSON(http.StatusOK, gin.H{"network_ip_availabilities": availabilities})
}

// GetNetworkIPAvailability returns IP availability for a specific network
func (svc *Service) GetNetworkIPAvailability(c *gin.Context) {
	networkID := c.Param("id")
	projectID := c.GetString("project_id")

	// Verify network exists and belongs to project
	var name string
	err := svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT name FROM networks WHERE id = $1 AND project_id = $2",
		networkID, projectID,
	).Scan(&name)

	if err != nil {
		common.SendError(c, common.NewNotFoundError("network"))
		return
	}

	// Query subnets for this network
	rows, err := svc.activeDB().Query(c.Request.Context(), `
		SELECT id, cidr
		FROM subnets
		WHERE network_id = $1
		ORDER BY created_at
	`, networkID)

	if err != nil {
		log.Error().Err(err).Str("operation", "get_network_ip_availability").Msg("failed to query subnets")
		common.SendError(c, common.NewInternalServerError("failed to get network IP availability"))
		return
	}
	defer rows.Close()

	subnetAvailabilities := []gin.H{}
	totalIPs := 0
	usedIPs := 0

	for rows.Next() {
		var subnetID, cidr string
		if err := rows.Scan(&subnetID, &cidr); err != nil {
			log.Warn().Err(err).Msg("failed to scan subnet row")
			continue
		}

		// Stub: assume /24 subnet with 253 usable IPs
		subnetTotal := 253
		subnetUsed := 0

		totalIPs += subnetTotal

		subnetAvailabilities = append(subnetAvailabilities, gin.H{
			"subnet_id": subnetID,
			"cidr":      cidr,
			"total_ips": subnetTotal,
			"used_ips":  subnetUsed,
		})
	}
	if err := rows.Err(); err != nil {
		log.Error().Err(err).Str("operation", "get_network_ip_availability").Msg("rows iteration error")
		common.SendError(c, common.NewInternalServerError("failed to get network IP availability"))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"network_ip_availability": gin.H{
			"network_id":             networkID,
			"network_name":           name,
			"project_id":             projectID,
			"total_ips":              totalIPs,
			"used_ips":               usedIPs,
			"subnet_ip_availability": subnetAvailabilities,
		},
	})
}
