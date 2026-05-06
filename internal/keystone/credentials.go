package keystone

import (
	"fmt"
	"time"

	"github.com/cobaltcore-dev/o3k/internal/common"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog/log"
)

// ListCredentials returns credentials for the authenticated user
func (svc *Service) ListCredentials(c *gin.Context) {
	userID := c.GetString("user_id")

	rows, err := svc.activeDB().Query(c.Request.Context(), `
		SELECT id, user_id, project_id, type, blob, created_at
		FROM credentials
		WHERE user_id = $1
		ORDER BY created_at DESC
	`, userID)
	if err != nil {
		log.Error().Err(err).Str("operation", "list_credentials").Msg("Failed to query credentials")
		common.SendError(c, common.NewInternalServerError("failed to query credentials"))
		return
	}
	defer rows.Close()

	credentials := []map[string]interface{}{}
	for rows.Next() {
		var id, userID, credType, blob string
		var projectID *string
		var createdAt time.Time

		if err := rows.Scan(&id, &userID, &projectID, &credType, &blob, &createdAt); err != nil {
			continue
		}

		credential := map[string]interface{}{
			"id":      id,
			"user_id": userID,
			"type":    credType,
			"blob":    blob,
		}

		if projectID != nil {
			credential["project_id"] = *projectID
		}

		credentials = append(credentials, credential)
	}

	c.JSON(200, gin.H{"credentials": credentials})
}

// CreateCredential creates a new credential
func (svc *Service) CreateCredential(c *gin.Context) {
	authUserID := c.GetString("user_id")

	var req struct {
		Credential struct {
			UserID    string `json:"user_id"`
			ProjectID string `json:"project_id"`
			Type      string `json:"type" binding:"required"`
			Blob      string `json:"blob" binding:"required"`
		} `json:"credential"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		common.SendError(c, common.NewBadRequestError("invalid request body"))
		return
	}

	// Use authenticated user's ID (ignore request body user_id for non-admin)
	userID := authUserID
	if req.Credential.UserID != "" && req.Credential.UserID != authUserID {
		// Only admin can create credentials for other users
		roles, _ := c.Get("roles")
		isAdmin := false
		if roleSlice, ok := roles.([]string); ok {
			for _, r := range roleSlice {
				if r == "admin" {
					isAdmin = true
					break
				}
			}
		}
		if !isAdmin {
			common.SendError(c, common.NewForbiddenError("cannot create credentials for another user"))
			return
		}
		userID = req.Credential.UserID
	}

	credID := uuid.New()
	now := time.Now()

	var projectID interface{}
	if req.Credential.ProjectID != "" {
		projectID = req.Credential.ProjectID
	}

	_, err := svc.activeDB().Exec(c.Request.Context(), `
		INSERT INTO credentials (id, user_id, project_id, type, blob, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, credID, userID, projectID, req.Credential.Type, req.Credential.Blob, now, now)

	if err != nil {
		log.Error().Err(err).Str("operation", "create_credential").Msg("Failed to create credential")
		common.SendError(c, common.NewInternalServerError("failed to create credential"))
		return
	}

	credential := map[string]interface{}{
		"id":      credID.String(),
		"user_id": userID,
		"type":    req.Credential.Type,
		"blob":    req.Credential.Blob,
	}

	if req.Credential.ProjectID != "" {
		credential["project_id"] = req.Credential.ProjectID
	}

	c.JSON(201, gin.H{"credential": credential})
}

// GetCredential returns a specific credential by ID
func (svc *Service) GetCredential(c *gin.Context) {
	credID := c.Param("id")
	authUserID := c.GetString("user_id")

	var id, userID, credType, blob string
	var projectID *string

	err := svc.activeDB().QueryRow(c.Request.Context(), `
		SELECT id, user_id, project_id, type, blob
		FROM credentials
		WHERE id = $1 AND user_id = $2
	`, credID, authUserID).Scan(&id, &userID, &projectID, &credType, &blob)

	if err == pgx.ErrNoRows {
		common.SendError(c, common.NewNotFoundError("credential"))
		return
	}
	if err != nil {
		log.Error().Err(err).Str("operation", "get_credential").Str("cred_id", credID).Msg("Failed to query credential")
		common.SendError(c, common.NewInternalServerError("failed to query credential"))
		return
	}

	credential := map[string]interface{}{
		"id":      id,
		"user_id": userID,
		"type":    credType,
		"blob":    blob,
	}

	if projectID != nil {
		credential["project_id"] = *projectID
	}

	c.JSON(200, gin.H{"credential": credential})
}

// UpdateCredential updates a credential
func (svc *Service) UpdateCredential(c *gin.Context) {
	credID := c.Param("id")
	authUserID := c.GetString("user_id")

	var req struct {
		Credential map[string]interface{} `json:"credential"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		common.SendError(c, common.NewBadRequestError("invalid request body"))
		return
	}

	// Build dynamic update query
	updates := []string{}
	args := []interface{}{}
	argCount := 1

	if blob, ok := req.Credential["blob"].(string); ok {
		updates = append(updates, fmt.Sprintf("blob = $%d", argCount))
		args = append(args, blob)
		argCount++
	}

	if len(updates) == 0 {
		common.SendError(c, common.NewBadRequestError("no fields to update"))
		return
	}

	updates = append(updates, fmt.Sprintf("updated_at = $%d", argCount))
	args = append(args, time.Now())
	argCount++

	// Add credential ID and user_id as final parameters
	args = append(args, credID, authUserID)

	query := "UPDATE credentials SET " + updates[0]
	for i := 1; i < len(updates); i++ {
		query += ", " + updates[i]
	}
	query += fmt.Sprintf(" WHERE id = $%d AND user_id = $%d", argCount, argCount+1)

	result, err := svc.activeDB().Exec(c.Request.Context(), query, args...)
	if err != nil {
		log.Error().Err(err).Str("operation", "update_credential").Str("cred_id", credID).Msg("Failed to update credential")
		common.SendError(c, common.NewInternalServerError("failed to update credential"))
		return
	}

	if result.RowsAffected() == 0 {
		common.SendError(c, common.NewNotFoundError("credential"))
		return
	}

	// Return updated credential
	svc.GetCredential(c)
}

// DeleteCredential deletes a credential
func (svc *Service) DeleteCredential(c *gin.Context) {
	credID := c.Param("id")
	authUserID := c.GetString("user_id")

	result, err := svc.activeDB().Exec(c.Request.Context(),
		"DELETE FROM credentials WHERE id = $1 AND user_id = $2",
		credID, authUserID)

	if err != nil {
		log.Error().Err(err).Str("operation", "delete_credential").Str("cred_id", credID).Msg("Failed to delete credential")
		common.SendError(c, common.NewInternalServerError("failed to delete credential"))
		return
	}

	if result.RowsAffected() == 0 {
		common.SendError(c, common.NewNotFoundError("credential"))
		return
	}

	c.Status(204)
}
