package keystone

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"time"

	"github.com/cobaltcore-dev/o3k/internal/database"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// ListApplicationCredentials returns application credentials for a user
func (svc *Service) ListApplicationCredentials(c *gin.Context) {
	userID := c.Param("id")

	rows, err := database.DB.Query(c.Request.Context(), `
		SELECT id, user_id, project_id, name, description, expires_at, unrestricted, created_at
		FROM application_credentials
		WHERE user_id = $1
		ORDER BY created_at DESC
	`, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to query application credentials"})
		return
	}
	defer rows.Close()

	credentials := []map[string]interface{}{}
	for rows.Next() {
		var id, userIDVal, name string
		var projectID, description *string
		var expiresAt *time.Time
		var unrestricted bool
		var createdAt time.Time

		if err := rows.Scan(&id, &userIDVal, &projectID, &name, &description, &expiresAt, &unrestricted, &createdAt); err != nil {
			continue
		}

		credential := map[string]interface{}{
			"id":           id,
			"user_id":      userIDVal,
			"name":         name,
			"unrestricted": unrestricted,
		}

		if projectID != nil {
			credential["project_id"] = *projectID
		}
		if description != nil {
			credential["description"] = *description
		}
		if expiresAt != nil {
			credential["expires_at"] = expiresAt.Format(time.RFC3339)
		}

		// Get roles
		roleRows, err := database.DB.Query(c.Request.Context(), `
			SELECT r.id, r.name
			FROM application_credential_roles acr
			JOIN roles r ON acr.role_id = r.id
			WHERE acr.application_credential_id = $1
		`, id)
		if err == nil {
			roles := []map[string]interface{}{}
			for roleRows.Next() {
				var roleID, roleName string
				if err := roleRows.Scan(&roleID, &roleName); err == nil {
					roles = append(roles, map[string]interface{}{
						"id":   roleID,
						"name": roleName,
					})
				}
			}
			roleRows.Close()
			credential["roles"] = roles
		}

		credentials = append(credentials, credential)
	}

	c.JSON(http.StatusOK, gin.H{"application_credentials": credentials})
}

// CreateApplicationCredential creates a new application credential
func (svc *Service) CreateApplicationCredential(c *gin.Context) {
	userID := c.Param("id")

	var req struct {
		ApplicationCredential struct {
			Name          string                   `json:"name" binding:"required"`
			Description   string                   `json:"description"`
			ProjectID     string                   `json:"project_id"`
			ExpiresAt     string                   `json:"expires_at"`
			Unrestricted  bool                     `json:"unrestricted"`
			Roles         []map[string]interface{} `json:"roles"`
		} `json:"application_credential"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	credID := uuid.New()
	now := time.Now()

	// Generate random secret
	secretBytes := make([]byte, 32)
	rand.Read(secretBytes)
	secret := base64.URLEncoding.EncodeToString(secretBytes)

	var projectID interface{}
	if req.ApplicationCredential.ProjectID != "" {
		projectID = req.ApplicationCredential.ProjectID
	}

	var expiresAt interface{}
	if req.ApplicationCredential.ExpiresAt != "" {
		if t, err := time.Parse(time.RFC3339, req.ApplicationCredential.ExpiresAt); err == nil {
			expiresAt = t
		}
	}

	_, err := database.DB.Exec(c.Request.Context(), `
		INSERT INTO application_credentials (id, user_id, project_id, name, secret_hash, description, expires_at, unrestricted, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, credID, userID, projectID, req.ApplicationCredential.Name, secret, req.ApplicationCredential.Description, expiresAt, req.ApplicationCredential.Unrestricted, now)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Associate roles
	for _, role := range req.ApplicationCredential.Roles {
		roleID := role["id"].(string)
		database.DB.Exec(c.Request.Context(), `
			INSERT INTO application_credential_roles (application_credential_id, role_id)
			VALUES ($1, $2)
		`, credID, roleID)
	}

	credential := map[string]interface{}{
		"id":           credID.String(),
		"user_id":      userID,
		"name":         req.ApplicationCredential.Name,
		"secret":       secret,
		"unrestricted": req.ApplicationCredential.Unrestricted,
	}

	if req.ApplicationCredential.ProjectID != "" {
		credential["project_id"] = req.ApplicationCredential.ProjectID
	}
	if req.ApplicationCredential.Description != "" {
		credential["description"] = req.ApplicationCredential.Description
	}
	if expiresAt != nil {
		credential["expires_at"] = expiresAt.(time.Time).Format(time.RFC3339)
	}

	// Add roles to response
	if len(req.ApplicationCredential.Roles) > 0 {
		credential["roles"] = req.ApplicationCredential.Roles
	}

	c.JSON(http.StatusCreated, gin.H{"application_credential": credential})
}

// GetApplicationCredential returns a specific application credential
func (svc *Service) GetApplicationCredential(c *gin.Context) {
	userID := c.Param("id")
	credID := c.Param("cred_id")

	var id, userIDVal, name string
	var projectID, description *string
	var expiresAt *time.Time
	var unrestricted bool

	err := database.DB.QueryRow(c.Request.Context(), `
		SELECT id, user_id, project_id, name, description, expires_at, unrestricted
		FROM application_credentials
		WHERE id = $1 AND user_id = $2
	`, credID, userID).Scan(&id, &userIDVal, &projectID, &name, &description, &expiresAt, &unrestricted)

	if err == pgx.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Application credential not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to query application credential"})
		return
	}

	credential := map[string]interface{}{
		"id":           id,
		"user_id":      userIDVal,
		"name":         name,
		"unrestricted": unrestricted,
	}

	if projectID != nil {
		credential["project_id"] = *projectID
	}
	if description != nil {
		credential["description"] = *description
	}
	if expiresAt != nil {
		credential["expires_at"] = expiresAt.Format(time.RFC3339)
	}

	// Get roles
	roleRows, err := database.DB.Query(c.Request.Context(), `
		SELECT r.id, r.name
		FROM application_credential_roles acr
		JOIN roles r ON acr.role_id = r.id
		WHERE acr.application_credential_id = $1
	`, id)
	if err == nil {
		roles := []map[string]interface{}{}
		for roleRows.Next() {
			var roleID, roleName string
			if err := roleRows.Scan(&roleID, &roleName); err == nil {
				roles = append(roles, map[string]interface{}{
					"id":   roleID,
					"name": roleName,
				})
			}
		}
		roleRows.Close()
		credential["roles"] = roles
	}

	c.JSON(http.StatusOK, gin.H{"application_credential": credential})
}

// DeleteApplicationCredential deletes an application credential
func (svc *Service) DeleteApplicationCredential(c *gin.Context) {
	userID := c.Param("id")
	credID := c.Param("cred_id")

	result, err := database.DB.Exec(c.Request.Context(),
		"DELETE FROM application_credentials WHERE id = $1 AND user_id = $2",
		credID, userID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete application credential"})
		return
	}

	if result.RowsAffected() == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Application credential not found"})
		return
	}

	c.Status(http.StatusNoContent)
}

// GetApplicationCredentialByID returns an application credential by ID only
func (svc *Service) GetApplicationCredentialByID(c *gin.Context) {
	credID := c.Param("id")

	var id, userID, name string
	var projectID, description *string
	var expiresAt *time.Time
	var unrestricted bool

	err := database.DB.QueryRow(c.Request.Context(), `
		SELECT id, user_id, project_id, name, description, expires_at, unrestricted
		FROM application_credentials
		WHERE id = $1
	`, credID).Scan(&id, &userID, &projectID, &name, &description, &expiresAt, &unrestricted)

	if err == pgx.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Application credential not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to query application credential"})
		return
	}

	credential := map[string]interface{}{
		"id":           id,
		"user_id":      userID,
		"name":         name,
		"unrestricted": unrestricted,
	}

	if projectID != nil {
		credential["project_id"] = *projectID
	}
	if description != nil {
		credential["description"] = *description
	}
	if expiresAt != nil {
		credential["expires_at"] = expiresAt.Format(time.RFC3339)
	}

	// Get roles
	roleRows, err := database.DB.Query(c.Request.Context(), `
		SELECT r.id, r.name
		FROM application_credential_roles acr
		JOIN roles r ON acr.role_id = r.id
		WHERE acr.application_credential_id = $1
	`, id)
	if err == nil {
		roles := []map[string]interface{}{}
		for roleRows.Next() {
			var roleID, roleName string
			if err := roleRows.Scan(&roleID, &roleName); err == nil {
				roles = append(roles, map[string]interface{}{
					"id":   roleID,
					"name": roleName,
				})
			}
		}
		roleRows.Close()
		credential["roles"] = roles
	}

	c.JSON(http.StatusOK, gin.H{"application_credential": credential})
}
