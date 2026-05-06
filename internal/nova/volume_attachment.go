package nova

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog/log"
	"github.com/cobaltcore-dev/o3k/internal/common"
	"github.com/cobaltcore-dev/o3k/pkg/hypervisor"
)

// VolumeAttachment represents a volume attached to an instance
type VolumeAttachment struct {
	ID          string    `json:"id"`
	VolumeID    string    `json:"volumeId"`
	InstanceID  string    `json:"serverId"`
	Device      string    `json:"device"`
	AttachedAt  time.Time `json:"attachedAt"`
}

// AttachVolumeRequest represents a volume attachment request
type AttachVolumeRequest struct {
	VolumeAttachment struct {
		VolumeID string `json:"volumeId" binding:"required"`
		Device   string `json:"device"` // Optional, auto-assigned if not provided
	} `json:"volumeAttachment"`
}

// AttachVolume attaches a volume to an instance
func (svc *Service) AttachVolume(c *gin.Context) {
	instanceID := c.Param("id")
	projectID := c.GetString("project_id")

	var req AttachVolumeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.SendError(c, common.NewBadRequestError("invalid request body"))
		return
	}

	// Verify instance exists and get libvirt domain ID
	var libvirtDomainID string
	err := svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT libvirt_domain_id FROM instances WHERE id = $1 AND project_id = $2",
		instanceID, projectID,
	).Scan(&libvirtDomainID)

	if err == pgx.ErrNoRows {
		common.SendError(c, common.NewNotFoundError("instance"))
		return
	}

	volumeID := req.VolumeAttachment.VolumeID

	// Atomically claim the volume — only succeeds if status is 'available'.
	// This eliminates the check-then-act race: two concurrent requests cannot
	// both pass the availability guard because exactly one UPDATE will match
	// the WHERE clause.
	now := time.Now()
	tag, err := svc.activeDB().Exec(c.Request.Context(),
		`UPDATE volumes
		 SET status = 'in-use', attached_to_instance_id = $1, updated_at = $2
		 WHERE id = $3 AND project_id = $4 AND status = 'available'`,
		instanceID, now, volumeID, projectID,
	)
	if err != nil {
		log.Error().Err(err).Str("operation", "atomic_claim_volume").Msg("database error")
		common.SendError(c, common.NewInternalServerError("failed to attach volume"))
		return
	}
	if tag.RowsAffected() == 0 {
		// Distinguish: not found vs. not available
		var volStatus string
		checkErr := svc.activeDB().QueryRow(c.Request.Context(),
			"SELECT status FROM volumes WHERE id = $1 AND project_id = $2",
			volumeID, projectID,
		).Scan(&volStatus)
		if checkErr != nil {
			common.SendError(c, common.NewNotFoundError("volume"))
			return
		}
		common.SendError(c, common.NewBadRequestError("volume is not available for attachment (current status: "+volStatus+")"))
		return
	}

	// Auto-assign device if not provided
	device := req.VolumeAttachment.Device
	if device == "" {
		device, err = svc.getNextAvailableDevice(c.Request.Context(), instanceID)
		if err != nil {
			log.Error().Err(err).Str("operation", "get_next_device").Msg("device assignment error")
			// Roll back the atomic status change so the volume is available again.
			svc.activeDB().Exec(c.Request.Context(),
				"UPDATE volumes SET status = 'available', attached_to_instance_id = NULL WHERE id = $1",
				volumeID,
			)
			common.SendError(c, common.NewInternalServerError("failed to assign device"))
			return
		}
	}

	// Create attachment record
	attachmentID := uuid.New().String()

	_, err = svc.activeDB().Exec(c.Request.Context(), `
		INSERT INTO volume_attachments (id, volume_id, instance_id, device, attached_at)
		VALUES ($1, $2, $3, $4, $5)
	`, attachmentID, volumeID, instanceID, device, now)

	if err != nil {
		log.Error().Err(err).Str("operation", "create_volume_attachment").Msg("database error")
		// Roll back the atomic status change so the volume is available again.
		svc.activeDB().Exec(c.Request.Context(),
			"UPDATE volumes SET status = 'available', attached_to_instance_id = NULL WHERE id = $1",
			volumeID,
		)
		common.SendError(c, common.NewInternalServerError("failed to create volume attachment"))
		return
	}

	// Attach volume to VM if hypervisor is available
	if svc.vmManager != nil && libvirtDomainID != "" {
		svc.wg.Add(1)
		go func() {
			defer svc.wg.Done()
			ctx, cancel := context.WithTimeout(svc.ctx, 10*time.Second)
			defer cancel()

			// Get volume details
			var sizeGB int
			var rbdPool, rbdImage string
			err := svc.activeDB().QueryRow(ctx,
				"SELECT size_gb, rbd_pool, rbd_image FROM volumes WHERE id = $1",
				req.VolumeAttachment.VolumeID,
			).Scan(&sizeGB, &rbdPool, &rbdImage)

			if err != nil {
				return
			}

			// Attach disk to VM
			diskXML := hypervisor.GenerateDiskXML(hypervisor.DiskSpec{
				Device:   device,
				Type:     "network", // or "file" for local
				Source:   fmt.Sprintf("%s/%s", rbdPool, rbdImage),
				Protocol: "rbd",
			})

			if err := svc.vmManager.AttachDevice(ctx, libvirtDomainID, diskXML); err != nil {
				// Update attachment status to error (don't delete, admin can retry)
				svc.activeDB().Exec(ctx,
					"UPDATE volume_attachments SET device = $1 WHERE id = $2",
					device+"(error)", attachmentID)
			}
		}()
	}

	c.JSON(http.StatusOK, gin.H{
		"volumeAttachment": gin.H{
			"id":        attachmentID,
			"volumeId":  req.VolumeAttachment.VolumeID,
			"serverId":  instanceID,
			"device":    device,
			"attachedAt": now.Format(time.RFC3339),
		},
	})
}

// DetachVolume detaches a volume from an instance
func (svc *Service) DetachVolume(c *gin.Context) {
	instanceID := c.Param("id")
	volumeID := c.Param("volume_id")
	projectID := c.GetString("project_id")

	// Verify instance exists and get libvirt domain ID
	var libvirtDomainID string
	err := svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT libvirt_domain_id FROM instances WHERE id = $1 AND project_id = $2",
		instanceID, projectID,
	).Scan(&libvirtDomainID)

	if err == pgx.ErrNoRows {
		common.SendError(c, common.NewNotFoundError("instance"))
		return
	}

	// Get attachment details
	var attachmentID, device string
	err = svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT id, device FROM volume_attachments WHERE volume_id = $1 AND instance_id = $2",
		volumeID, instanceID,
	).Scan(&attachmentID, &device)

	if err == pgx.ErrNoRows {
		common.SendError(c, common.NewNotFoundError("volume attachment"))
		return
	}

	// Detach from hypervisor first
	if svc.vmManager != nil && libvirtDomainID != "" {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
		defer cancel()

		// Get volume details for disk XML
		var rbdPool, rbdImage string
		err := svc.activeDB().QueryRow(ctx,
			"SELECT rbd_pool, rbd_image FROM volumes WHERE id = $1",
			volumeID,
		).Scan(&rbdPool, &rbdImage)

		if err == nil {
			diskXML := hypervisor.GenerateDiskXML(hypervisor.DiskSpec{
				Device:   device,
				Type:     "network",
				Source:   fmt.Sprintf("%s/%s", rbdPool, rbdImage),
				Protocol: "rbd",
			})

			if err := svc.vmManager.DetachDevice(ctx, libvirtDomainID, diskXML); err != nil {
				log.Error().Err(err).Str("operation", "detach_disk_libvirt").Msg("libvirt error")
				common.SendError(c, common.NewInternalServerError("failed to detach disk"))
				return
			}
		}
	}

	// Delete attachment record
	_, err = svc.activeDB().Exec(c.Request.Context(),
		"DELETE FROM volume_attachments WHERE id = $1",
		attachmentID,
	)

	if err != nil {
		log.Error().Err(err).Str("operation", "delete_volume_attachment").Msg("database error")
		common.SendError(c, common.NewInternalServerError("failed to delete volume attachment"))
		return
	}

	// Update volume status
	now := time.Now()
	_, err = svc.activeDB().Exec(c.Request.Context(), `
		UPDATE volumes
		SET attached_to_instance_id = NULL, status = $1, updated_at = $2
		WHERE id = $3
	`, "available", now, volumeID)

	if err != nil {
		log.Error().Err(err).Str("operation", "update_volume_status_detach").Msg("database error")
		common.SendError(c, common.NewInternalServerError("failed to update volume status"))
		return
	}

	c.Status(http.StatusAccepted)
}

// ListVolumeAttachments lists all volumes attached to an instance
func (svc *Service) ListVolumeAttachments(c *gin.Context) {
	instanceID := c.Param("id")
	projectID := c.GetString("project_id")

	// Verify instance exists
	var exists bool
	err := svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT EXISTS(SELECT 1 FROM instances WHERE id = $1 AND project_id = $2)",
		instanceID, projectID,
	).Scan(&exists)

	if err != nil || !exists {
		common.SendError(c, common.NewNotFoundError("instance"))
		return
	}

	// List attachments
	rows, err := svc.activeDB().Query(c.Request.Context(), `
		SELECT id, volume_id, device, attached_at
		FROM volume_attachments
		WHERE instance_id = $1
		ORDER BY attached_at
	`, instanceID)

	if err != nil {
		log.Error().Err(err).Str("operation", "list_volume_attachments").Msg("database error")
		common.SendError(c, common.NewInternalServerError("failed to list volume attachments"))
		return
	}
	defer rows.Close()

	var attachments []gin.H
	for rows.Next() {
		var id, volumeID, device string
		var attachedAt time.Time

		if err := rows.Scan(&id, &volumeID, &device, &attachedAt); err != nil {
			continue
		}

		attachments = append(attachments, gin.H{
			"id":         id,
			"volumeId":   volumeID,
			"serverId":   instanceID,
			"device":     device,
			"attachedAt": attachedAt.Format(time.RFC3339),
		})
	}

	if attachments == nil {
		attachments = []gin.H{}
	}

	c.JSON(http.StatusOK, gin.H{"volumeAttachments": attachments})
}

// getNextAvailableDevice finds the next available device name (vdb, vdc, etc.)
func (svc *Service) getNextAvailableDevice(ctx context.Context, instanceID string) (string, error) {
	// Query existing devices
	rows, err := svc.activeDB().Query(ctx,
		"SELECT device FROM volume_attachments WHERE instance_id = $1",
		instanceID,
	)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	usedDevices := make(map[string]bool)
	for rows.Next() {
		var device string
		if err := rows.Scan(&device); err != nil {
			continue
		}
		usedDevices[device] = true
	}

	// Find first available device (vdb, vdc, ..., vdz)
	for i := 'b'; i <= 'z'; i++ {
		device := fmt.Sprintf("/dev/vd%c", i)
		if !usedDevices[device] {
			return device, nil
		}
	}

	return "", fmt.Errorf("no available device slots")
}
