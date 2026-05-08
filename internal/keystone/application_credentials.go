package keystone

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"time"

	"github.com/cobaltcore-dev/o3k/internal/common"
	"github.com/cobaltcore-dev/o3k/internal/database"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"golang.org/x/crypto/bcrypt"
)

// ListApplicationCredentials returns application credentials for a user
func (svc *Service) ListApplicationCredentials(c *gin.Context) {
	userID := c.Param("id")

	callerID := c.GetString("user_id")
	if callerID != userID {
		roles, _ := c.Get("roles")
		roleList, _ := roles.([]string)
		isAdmin := false
		for _, r := range roleList {
			if r == "admin" {
				isAdmin = true
				break
			}
		}
		if !isAdmin {
			common.SendError(c, common.NewForbiddenError("access denied"))
			return
		}
	}

	rows, err := svc.activeDB().Query(c.Request.Context(), `
		SELECT id, user_id, project_id, name, description, expires_at, unrestricted, created_at, access_rules
		FROM application_credentials
		WHERE user_id = $1
		ORDER BY created_at DESC
	`, userID)
	if err != nil {
		log.Error().Err(err).Str("operation", "list_application_credentials").Str("user_id", userID).Msg("Failed to query application credentials")
		common.SendError(c, common.NewInternalServerError("failed to query application credentials"))
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
		var accessRulesJSON []byte

		if err := rows.Scan(&id, &userIDVal, &projectID, &name, &description, &expiresAt, &unrestricted, &createdAt, &accessRulesJSON); err != nil {
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

		// Parse and include access_rules if present
		if accessRulesJSON != nil {
			var accessRules []AccessRule
			if err := json.Unmarshal(accessRulesJSON, &accessRules); err == nil && len(accessRules) > 0 {
				credential["access_rules"] = accessRules
			}
		}

		// Get roles
		roleRows, err := svc.activeDB().Query(c.Request.Context(), `
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
			if roleRowsErr := roleRows.Err(); roleRowsErr != nil {
				log.Warn().Err(roleRowsErr).Str("operation", "list_application_credentials").Str("credential_id", id).Msg("error iterating role rows")
			}
			roleRows.Close()
			credential["roles"] = roles
		}

		credentials = append(credentials, credential)
	}

	if err := rows.Err(); err != nil {
		log.Error().Err(err).Str("operation", "list_application_credentials").Msg("row iteration error")
		common.SendError(c, common.NewInternalServerError("failed to read application credentials"))
		return
	}

	c.JSON(200, gin.H{"application_credentials": credentials})
}

// CreateApplicationCredential creates a new application credential
func (svc *Service) CreateApplicationCredential(c *gin.Context) {
	userID := c.Param("id")

	callerID := c.GetString("user_id")
	if callerID != userID {
		roles, _ := c.Get("roles")
		roleList, _ := roles.([]string)
		isAdmin := false
		for _, r := range roleList {
			if r == "admin" {
				isAdmin = true
				break
			}
		}
		if !isAdmin {
			common.SendError(c, common.NewForbiddenError("access denied"))
			return
		}
	}

	var req struct {
		ApplicationCredential struct {
			Name         string                   `json:"name" binding:"required"`
			Description  string                   `json:"description"`
			ProjectID    string                   `json:"project_id"`
			ExpiresAt    string                   `json:"expires_at"`
			Unrestricted bool                     `json:"unrestricted"`
			Roles        []map[string]interface{} `json:"roles"`
			AccessRules  []AccessRule             `json:"access_rules,omitempty"`
		} `json:"application_credential"`
	}

	// H1: reject creation of new app credentials when caller authenticated via
	// an app credential that does not have unrestricted=true.
	if c.GetString("auth_method") == "application_credential" && !c.GetBool("app_credential_unrestricted") {
		common.SendError(c, common.NewForbiddenError("application credentials cannot be created using a restricted application credential"))
		return
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		common.SendError(c, common.NewBadRequestError("invalid request body"))
		return
	}

	credID := uuid.New()
	now := time.Now()

	// Generate random secret
	secretBytes := make([]byte, 32)
	if _, err := rand.Read(secretBytes); err != nil {
		log.Error().Err(err).Msg("failed to generate random secret")
		common.SendError(c, common.NewInternalServerError("failed to generate credential secret"))
		return
	}
	secret := base64.URLEncoding.EncodeToString(secretBytes)

	// Hash the secret with bcrypt (cost 12 per spec)
	secretHash, err := bcrypt.GenerateFromPassword([]byte(secret), 12)
	if err != nil {
		log.Error().Err(err).Msg("failed to hash application credential secret")
		common.SendError(c, common.NewInternalServerError("failed to generate credential secret"))
		return
	}

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

	var accessRulesJSON interface{}
	if len(req.ApplicationCredential.AccessRules) > 0 {
		b, err := json.Marshal(req.ApplicationCredential.AccessRules)
		if err != nil {
			log.Error().Err(err).Msg("failed to marshal access_rules")
			common.SendError(c, common.NewInternalServerError("failed to process access rules"))
			return
		}
		accessRulesJSON = b
	}

	ctx := c.Request.Context()

	// Validate that caller holds each requested role
	callerProjectID := c.GetString("project_id")
	for _, role := range req.ApplicationCredential.Roles {
		roleID, _ := role["id"].(string)
		if roleID == "" {
			continue
		}
		var exists bool
		err := svc.activeDB().QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM role_assignments WHERE user_id=$1 AND project_id=$2 AND role_id=$3)`,
			callerID, callerProjectID, roleID,
		).Scan(&exists)
		if err != nil || !exists {
			common.SendError(c, common.NewForbiddenError("cannot assign role not held by caller"))
			return
		}
	}

	_, err = svc.activeDB().Exec(ctx, `
		INSERT INTO application_credentials (id, user_id, project_id, name, secret_hash, description, expires_at, unrestricted, legacy_auth, updated_at, access_rules, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, false, NOW(), $9, $10)
	`, credID, userID, projectID, req.ApplicationCredential.Name, string(secretHash), req.ApplicationCredential.Description, expiresAt, req.ApplicationCredential.Unrestricted, accessRulesJSON, now)

	if err != nil {
		log.Error().Err(err).Str("operation", "create_application_credential").Str("user_id", userID).Msg("Failed to create application credential")
		common.SendError(c, common.NewInternalServerError("failed to create application credential"))
		return
	}

	// Associate roles
	for _, role := range req.ApplicationCredential.Roles {
		roleID, ok := role["id"].(string)
		if !ok || roleID == "" {
			continue
		}
		_, _ = svc.activeDB().Exec(ctx, `
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
		if t, ok := expiresAt.(time.Time); ok {
			credential["expires_at"] = t.Format(time.RFC3339)
		}
	}

	// Add roles to response
	if len(req.ApplicationCredential.Roles) > 0 {
		credential["roles"] = req.ApplicationCredential.Roles
	}

	// Add access_rules to response
	if len(req.ApplicationCredential.AccessRules) > 0 {
		credential["access_rules"] = req.ApplicationCredential.AccessRules
	}

	c.JSON(201, gin.H{"application_credential": credential})
}

// GetApplicationCredential returns a specific application credential
func (svc *Service) GetApplicationCredential(c *gin.Context) {
	userID := c.Param("id")
	credID := c.Param("cred_id")

	callerID := c.GetString("user_id")
	if callerID != userID {
		roles, _ := c.Get("roles")
		roleList, _ := roles.([]string)
		isAdmin := false
		for _, r := range roleList {
			if r == "admin" {
				isAdmin = true
				break
			}
		}
		if !isAdmin {
			common.SendError(c, common.NewForbiddenError("access denied"))
			return
		}
	}

	var id, userIDVal, name string
	var projectID, description *string
	var expiresAt *time.Time
	var unrestricted bool
	var accessRulesJSON []byte

	err := svc.activeDB().QueryRow(c.Request.Context(), `
		SELECT id, user_id, project_id, name, description, expires_at, unrestricted, access_rules
		FROM application_credentials
		WHERE id = $1 AND user_id = $2
	`, credID, userID).Scan(&id, &userIDVal, &projectID, &name, &description, &expiresAt, &unrestricted, &accessRulesJSON)

	if errors.Is(err, database.ErrNoRows) {
		common.SendError(c, common.NewNotFoundError("application credential"))
		return
	}
	if err != nil {
		log.Error().Err(err).Str("operation", "get_application_credential").Str("cred_id", credID).Msg("Failed to query application credential")
		common.SendError(c, common.NewInternalServerError("failed to query application credential"))
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

	// Parse and include access_rules if present
	if accessRulesJSON != nil {
		var accessRules []AccessRule
		if err := json.Unmarshal(accessRulesJSON, &accessRules); err == nil && len(accessRules) > 0 {
			credential["access_rules"] = accessRules
		}
	}

	// Get roles
	roleRows, err := svc.activeDB().Query(c.Request.Context(), `
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
		if roleRowsErr := roleRows.Err(); roleRowsErr != nil {
			log.Warn().Err(roleRowsErr).Str("operation", "get_application_credential").Str("credential_id", id).Msg("error iterating role rows")
		}
		roleRows.Close()
		credential["roles"] = roles
	}

	c.JSON(200, gin.H{"application_credential": credential})
}

// DeleteApplicationCredential deletes an application credential
func (svc *Service) DeleteApplicationCredential(c *gin.Context) {
	userID := c.Param("id")
	credID := c.Param("cred_id")

	callerID := c.GetString("user_id")
	if callerID != userID {
		roles, _ := c.Get("roles")
		roleList, _ := roles.([]string)
		isAdmin := false
		for _, r := range roleList {
			if r == "admin" {
				isAdmin = true
				break
			}
		}
		if !isAdmin {
			common.SendError(c, common.NewForbiddenError("access denied"))
			return
		}
	}

	result, err := svc.activeDB().Exec(c.Request.Context(),
		"DELETE FROM application_credentials WHERE id = $1 AND user_id = $2",
		credID, userID)

	if err != nil {
		log.Error().Err(err).Str("operation", "delete_application_credential").Str("cred_id", credID).Msg("Failed to delete application credential")
		common.SendError(c, common.NewInternalServerError("failed to delete application credential"))
		return
	}

	if result.RowsAffected() == 0 {
		common.SendError(c, common.NewNotFoundError("application credential"))
		return
	}

	c.Status(204)
}

// GetApplicationCredentialByID returns an application credential by ID only
func (svc *Service) GetApplicationCredentialByID(c *gin.Context) {
	credID := c.Param("id")
	callerID := c.GetString("user_id")
	isAdmin := c.GetBool("is_admin")

	var id, userID, name string
	var projectID, description *string
	var expiresAt *time.Time
	var unrestricted bool
	var accessRulesJSON []byte

	err := svc.activeDB().QueryRow(c.Request.Context(), `
		SELECT id, user_id, project_id, name, description, expires_at, unrestricted, access_rules
		FROM application_credentials
		WHERE id = $1
	`, credID).Scan(&id, &userID, &projectID, &name, &description, &expiresAt, &unrestricted, &accessRulesJSON)

	if errors.Is(err, database.ErrNoRows) {
		common.SendError(c, common.NewNotFoundError("application credential"))
		return
	}
	if err != nil {
		log.Error().Err(err).Str("operation", "get_application_credential_by_id").Str("cred_id", credID).Msg("Failed to query application credential")
		common.SendError(c, common.NewInternalServerError("failed to query application credential"))
		return
	}

	// Non-admin users can only view their own application credentials
	if callerID != userID && !isAdmin {
		common.SendError(c, common.NewForbiddenError("insufficient privileges"))
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

	// Parse and include access_rules if present
	if accessRulesJSON != nil {
		var accessRules []AccessRule
		if err := json.Unmarshal(accessRulesJSON, &accessRules); err == nil && len(accessRules) > 0 {
			credential["access_rules"] = accessRules
		}
	}

	// Get roles
	roleRows, err := svc.activeDB().Query(c.Request.Context(), `
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
		if roleRowsErr := roleRows.Err(); roleRowsErr != nil {
			log.Warn().Err(roleRowsErr).Str("operation", "get_application_credential_by_id").Str("credential_id", id).Msg("error iterating role rows")
		}
		roleRows.Close()
		credential["roles"] = roles
	}

	c.JSON(200, gin.H{"application_credential": credential})
}
