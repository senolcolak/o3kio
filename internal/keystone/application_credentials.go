package keystone

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"time"

	"github.com/cobaltcore-dev/o3k/internal/database"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

// CreateApplicationCredential handles POST /v3/users/:id/application_credentials
func (svc *Service) CreateApplicationCredential(c *gin.Context) {
	userID := c.Param("id")

	var req struct {
		ApplicationCredential struct {
			Name         string  `json:"name" binding:"required"`
			Description  string  `json:"description"`
			ProjectID    *string `json:"project_id"`
			ExpiresAt    *string `json:"expires_at"`
			Unrestricted bool    `json:"unrestricted"`
		} `json:"application_credential" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Generate random secret
	secretBytes := make([]byte, 32)
	rand.Read(secretBytes)
	secret := hex.EncodeToString(secretBytes)

	// Hash the secret for storage
	hashedSecret, err := bcrypt.GenerateFromPassword([]byte(secret), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	credID := uuid.New().String()
	now := time.Now()

	var expiresAt *time.Time
	if req.ApplicationCredential.ExpiresAt != nil {
		t, err := time.Parse(time.RFC3339, *req.ApplicationCredential.ExpiresAt)
		if err == nil {
			expiresAt = &t
		}
	}

	_, err = database.DB.Exec(c.Request.Context(), `
		INSERT INTO application_credentials (id, name, user_id, project_id, secret_hash, description, unrestricted, expires_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, credID, req.ApplicationCredential.Name, userID, req.ApplicationCredential.ProjectID, string(hashedSecret), req.ApplicationCredential.Description, req.ApplicationCredential.Unrestricted, expiresAt, now)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	response := gin.H{
		"id":           credID,
		"name":         req.ApplicationCredential.Name,
		"description":  req.ApplicationCredential.Description,
		"user_id":      userID,
		"project_id":   req.ApplicationCredential.ProjectID,
		"unrestricted": req.ApplicationCredential.Unrestricted,
		"secret":       secret, // Only returned on create
		"created_at":   now.Format(time.RFC3339),
	}

	if expiresAt != nil {
		response["expires_at"] = expiresAt.Format(time.RFC3339)
	}

	c.JSON(http.StatusCreated, gin.H{
		"application_credential": response,
	})
}

// ListApplicationCredentials handles GET /v3/users/:id/application_credentials
func (svc *Service) ListApplicationCredentials(c *gin.Context) {
	userID := c.Param("id")

	rows, err := database.DB.Query(c.Request.Context(),
		`SELECT id, name, description, project_id, unrestricted, expires_at, created_at
		 FROM application_credentials
		 WHERE user_id = $1
		 ORDER BY created_at DESC`,
		userID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var credentials []gin.H
	for rows.Next() {
		var id, name, description string
		var projectID *string
		var unrestricted bool
		var expiresAt *time.Time
		var createdAt time.Time

		if err := rows.Scan(&id, &name, &description, &projectID, &unrestricted, &expiresAt, &createdAt); err != nil {
			continue
		}

		cred := gin.H{
			"id":           id,
			"name":         name,
			"description":  description,
			"user_id":      userID,
			"project_id":   projectID,
			"unrestricted": unrestricted,
			"created_at":   createdAt.Format(time.RFC3339),
		}

		if expiresAt != nil {
			cred["expires_at"] = expiresAt.Format(time.RFC3339)
		}

		credentials = append(credentials, cred)
	}

	if credentials == nil {
		credentials = []gin.H{}
	}

	c.JSON(http.StatusOK, gin.H{"application_credentials": credentials})
}

// GetApplicationCredential handles GET /v3/users/:id/application_credentials/:cred_id
func (svc *Service) GetApplicationCredential(c *gin.Context) {
	userID := c.Param("id")
	credID := c.Param("cred_id")

	var name, description string
	var projectID *string
	var unrestricted bool
	var expiresAt *time.Time
	var createdAt time.Time

	err := database.DB.QueryRow(c.Request.Context(),
		`SELECT name, description, project_id, unrestricted, expires_at, created_at
		 FROM application_credentials
		 WHERE id = $1 AND user_id = $2`,
		credID, userID,
	).Scan(&name, &description, &projectID, &unrestricted, &expiresAt, &createdAt)

	if err == pgx.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{
			"error": gin.H{
				"message": "Application credential not found",
				"code":    404,
				"title":   "Not Found",
			},
		})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	response := gin.H{
		"id":           credID,
		"name":         name,
		"description":  description,
		"user_id":      userID,
		"project_id":   projectID,
		"unrestricted": unrestricted,
		"created_at":   createdAt.Format(time.RFC3339),
	}

	if expiresAt != nil {
		response["expires_at"] = expiresAt.Format(time.RFC3339)
	}

	c.JSON(http.StatusOK, gin.H{
		"application_credential": response,
	})
}

// DeleteApplicationCredential handles DELETE /v3/users/:id/application_credentials/:cred_id
func (svc *Service) DeleteApplicationCredential(c *gin.Context) {
	userID := c.Param("id")
	credID := c.Param("cred_id")

	result, err := database.DB.Exec(c.Request.Context(),
		"DELETE FROM application_credentials WHERE id = $1 AND user_id = $2",
		credID, userID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	rowsAffected := result.RowsAffected()
	if rowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"error": gin.H{
				"message": "Application credential not found",
				"code":    404,
				"title":   "Not Found",
			},
		})
		return
	}

	c.Status(http.StatusNoContent)
}
