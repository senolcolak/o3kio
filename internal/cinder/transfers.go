package cinder

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"time"

	"github.com/cobaltcore-dev/o3k/internal/common"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog/log"
)

// CreateVolumeTransfer handles POST /v3/:project_id/volume-transfers
func (svc *Service) CreateVolumeTransfer(c *gin.Context) {
	projectID := c.GetString("project_id")

	var req struct {
		Transfer struct {
			VolumeID string  `json:"volume_id" binding:"required"`
			Name     *string `json:"name"`
		} `json:"transfer" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		common.SendError(c, common.NewBadRequestError("invalid request body"))
		return
	}

	// Verify volume exists and belongs to project
	var volumeStatus string
	err := svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT status FROM volumes WHERE id = $1 AND project_id = $2",
		req.Transfer.VolumeID, projectID,
	).Scan(&volumeStatus)

	if errors.Is(err, pgx.ErrNoRows) {
		common.SendError(c, common.NewNotFoundError("volume"))
		return
	}
	if err != nil {
		log.Error().Err(err).Str("operation", "create_transfer").Msg("failed to query volume")
		common.SendError(c, common.NewInternalServerError("failed to create transfer"))
		return
	}

	// Volume must be available
	if volumeStatus != "available" {
		common.SendError(c, common.NewBadRequestError("volume must be available for transfer"))
		return
	}

	// Generate auth key
	authKeyBytes := make([]byte, 16)
	rand.Read(authKeyBytes)
	authKey := hex.EncodeToString(authKeyBytes)

	transferID := uuid.New().String()
	now := time.Now()

	_, err = svc.activeDB().Exec(c.Request.Context(), `
		INSERT INTO volume_transfers (id, volume_id, name, source_project_id, auth_key, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, transferID, req.Transfer.VolumeID, req.Transfer.Name, projectID, authKey, now, now)

	if err != nil {
		log.Error().Err(err).Str("operation", "create_transfer").Msg("failed to insert transfer")
		common.SendError(c, common.NewInternalServerError("failed to create transfer"))
		return
	}

	c.JSON(http.StatusAccepted, gin.H{
		"transfer": gin.H{
			"id":         transferID,
			"volume_id":  req.Transfer.VolumeID,
			"name":       req.Transfer.Name,
			"auth_key":   authKey,
			"created_at": now.Format(time.RFC3339),
		},
	})
}

// ListVolumeTransfers handles GET /v3/:project_id/volume-transfers
func (svc *Service) ListVolumeTransfers(c *gin.Context) {
	projectID := c.GetString("project_id")

	rows, err := svc.activeDB().Query(c.Request.Context(), `
		SELECT id, volume_id, name, created_at
		FROM volume_transfers
		WHERE source_project_id = $1 AND accepted = false
		ORDER BY created_at DESC
	`, projectID)
	if err != nil {
		log.Error().Err(err).Str("operation", "list_transfers").Msg("failed to query transfers")
		common.SendError(c, common.NewInternalServerError("failed to list transfers"))
		return
	}
	defer rows.Close()

	transfers := []gin.H{}
	for rows.Next() {
		var id, volumeID string
		var name *string
		var createdAt time.Time

		if err := rows.Scan(&id, &volumeID, &name, &createdAt); err != nil {
			log.Warn().Err(err).Msg("failed to scan transfer row")
			continue
		}

		transfers = append(transfers, gin.H{
			"id":         id,
			"volume_id":  volumeID,
			"name":       name,
			"created_at": createdAt.Format(time.RFC3339),
		})
	}
	if err := rows.Err(); err != nil {
		log.Error().Err(err).Str("operation", "list_transfers").Msg("rows iteration error")
		common.SendError(c, common.NewInternalServerError("failed to list transfers"))
		return
	}

	c.JSON(http.StatusOK, gin.H{"transfers": transfers})
}

// ListVolumeTransfersNoProject is an alias for routes without project_id in URL
func (svc *Service) ListVolumeTransfersNoProject(c *gin.Context) {
	svc.ListVolumeTransfers(c)
}

// GetVolumeTransfer handles GET /v3/:project_id/volume-transfers/:id
func (svc *Service) GetVolumeTransfer(c *gin.Context) {
	transferID := c.Param("id")
	projectID := c.GetString("project_id")

	var volumeID string
	var name *string
	var createdAt time.Time

	err := svc.activeDB().QueryRow(c.Request.Context(), `
		SELECT volume_id, name, created_at
		FROM volume_transfers
		WHERE id = $1 AND source_project_id = $2 AND accepted = false
	`, transferID, projectID).Scan(&volumeID, &name, &createdAt)

	if errors.Is(err, pgx.ErrNoRows) {
		common.SendError(c, common.NewNotFoundError("transfer"))
		return
	}
	if err != nil {
		log.Error().Err(err).Str("operation", "get_transfer").Msg("failed to query transfer")
		common.SendError(c, common.NewInternalServerError("failed to get transfer"))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"transfer": gin.H{
			"id":         transferID,
			"volume_id":  volumeID,
			"name":       name,
			"created_at": createdAt.Format(time.RFC3339),
		},
	})
}

// DeleteVolumeTransfer handles DELETE /v3/:project_id/volume-transfers/:id
func (svc *Service) DeleteVolumeTransfer(c *gin.Context) {
	transferID := c.Param("id")
	projectID := c.GetString("project_id")

	result, err := svc.activeDB().Exec(c.Request.Context(),
		"DELETE FROM volume_transfers WHERE id = $1 AND source_project_id = $2 AND accepted = false",
		transferID, projectID,
	)

	if err != nil {
		log.Error().Err(err).Str("operation", "delete_transfer").Msg("failed to delete transfer")
		common.SendError(c, common.NewInternalServerError("failed to delete transfer"))
		return
	}

	if result.RowsAffected() == 0 {
		common.SendError(c, common.NewNotFoundError("transfer"))
		return
	}

	c.Status(http.StatusAccepted)
}

// AcceptVolumeTransfer handles POST /v3/:project_id/volume-transfers/:id/accept
func (svc *Service) AcceptVolumeTransfer(c *gin.Context) {
	transferID := c.Param("id")
	projectID := c.GetString("project_id")

	var req struct {
		Accept struct {
			AuthKey string `json:"auth_key" binding:"required"`
		} `json:"accept" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		common.SendError(c, common.NewBadRequestError("invalid request body"))
		return
	}

	// Get transfer details and verify auth key
	var volumeID, storedAuthKey, sourceProjectID string
	var name *string

	err := svc.activeDB().QueryRow(c.Request.Context(), `
		SELECT volume_id, name, auth_key, source_project_id
		FROM volume_transfers
		WHERE id = $1 AND accepted = false
	`, transferID).Scan(&volumeID, &name, &storedAuthKey, &sourceProjectID)

	if errors.Is(err, pgx.ErrNoRows) {
		common.SendError(c, common.NewNotFoundError("transfer"))
		return
	}
	if err != nil {
		log.Error().Err(err).Str("operation", "accept_transfer").Msg("failed to query transfer")
		common.SendError(c, common.NewInternalServerError("failed to accept transfer"))
		return
	}

	// Verify auth key
	if req.Accept.AuthKey != storedAuthKey {
		common.SendError(c, common.NewBadRequestError("invalid auth key"))
		return
	}

	// Transfer volume ownership atomically
	tx, err := svc.activeDB().BeginTx(c.Request.Context(), pgx.TxOptions{})
	if err != nil {
		log.Error().Err(err).Str("operation", "accept_transfer").Msg("failed to begin transaction")
		common.SendError(c, common.NewInternalServerError("failed to accept transfer"))
		return
	}
	defer func() { _ = tx.Rollback(c.Request.Context()) }()

	_, err = tx.Exec(c.Request.Context(),
		"UPDATE volumes SET project_id = $1 WHERE id = $2",
		projectID, volumeID,
	)
	if err != nil {
		log.Error().Err(err).Str("operation", "accept_transfer").Msg("failed to transfer volume ownership")
		common.SendError(c, common.NewInternalServerError("failed to accept transfer"))
		return
	}

	_, err = tx.Exec(c.Request.Context(),
		"UPDATE volume_transfers SET accepted = true, destination_project_id = $1 WHERE id = $2",
		projectID, transferID,
	)
	if err != nil {
		log.Error().Err(err).Str("operation", "accept_transfer").Msg("failed to mark transfer as accepted")
		common.SendError(c, common.NewInternalServerError("failed to accept transfer"))
		return
	}

	if err := tx.Commit(c.Request.Context()); err != nil {
		log.Error().Err(err).Str("operation", "accept_transfer").Msg("failed to commit transfer")
		common.SendError(c, common.NewInternalServerError("failed to accept transfer"))
		return
	}

	c.JSON(http.StatusAccepted, gin.H{
		"transfer": gin.H{
			"id":        transferID,
			"volume_id": volumeID,
			"name":      name,
		},
	})
}
