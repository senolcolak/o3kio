package neutron

import (
	"net/http"
	"time"

	"github.com/cobaltcore-dev/o3k/internal/database"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// ListAddressScopes lists all address scopes
func (svc *Service) ListAddressScopes(c *gin.Context) {
	projectID := c.GetString("project_id")

	rows, err := database.DB.Query(c.Request.Context(), `
		SELECT id, project_id, name, ip_version, shared, created_at, updated_at
		FROM address_scopes
		WHERE project_id = $1
		ORDER BY created_at DESC
	`, projectID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	scopes := []map[string]interface{}{}
	for rows.Next() {
		var (
			id        string
			projID    string
			name      string
			ipVersion int
			shared    bool
			createdAt time.Time
			updatedAt time.Time
		)

		err := rows.Scan(&id, &projID, &name, &ipVersion, &shared, &createdAt, &updatedAt)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		scope := map[string]interface{}{
			"id":          id,
			"tenant_id":   projID,
			"project_id":  projID,
			"name":        name,
			"ip_version":  ipVersion,
			"shared":      shared,
			"created_at":  createdAt.Format(time.RFC3339),
			"updated_at":  updatedAt.Format(time.RFC3339),
		}
		scopes = append(scopes, scope)
	}

	c.JSON(http.StatusOK, gin.H{"address_scopes": scopes})
}

// CreateAddressScope creates a new address scope
func (svc *Service) CreateAddressScope(c *gin.Context) {
	projectID := c.GetString("project_id")

	var req struct {
		AddressScope struct {
			Name      string `json:"name" binding:"required"`
			IPVersion int    `json:"ip_version" binding:"required"`
			Shared    *bool  `json:"shared"`
		} `json:"address_scope" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	shared := false
	if req.AddressScope.Shared != nil {
		shared = *req.AddressScope.Shared
	}

	scopeID := uuid.New().String()
	now := time.Now()

	_, err := database.DB.Exec(c.Request.Context(), `
		INSERT INTO address_scopes (id, project_id, name, ip_version, shared, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, scopeID, projectID, req.AddressScope.Name, req.AddressScope.IPVersion, shared, now, now)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"address_scope": map[string]interface{}{
			"id":          scopeID,
			"tenant_id":   projectID,
			"project_id":  projectID,
			"name":        req.AddressScope.Name,
			"ip_version":  req.AddressScope.IPVersion,
			"shared":      shared,
			"created_at":  now.Format(time.RFC3339),
			"updated_at":  now.Format(time.RFC3339),
		},
	})
}

// GetAddressScope retrieves a specific address scope
func (svc *Service) GetAddressScope(c *gin.Context) {
	scopeID := c.Param("id")
	projectID := c.GetString("project_id")

	var (
		projID    string
		name      string
		ipVersion int
		shared    bool
		createdAt time.Time
		updatedAt time.Time
	)

	err := database.DB.QueryRow(c.Request.Context(), `
		SELECT project_id, name, ip_version, shared, created_at, updated_at
		FROM address_scopes
		WHERE id = $1 AND project_id = $2
	`, scopeID, projectID).Scan(&projID, &name, &ipVersion, &shared, &createdAt, &updatedAt)

	if err == pgx.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "address scope not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"address_scope": map[string]interface{}{
			"id":          scopeID,
			"tenant_id":   projID,
			"project_id":  projID,
			"name":        name,
			"ip_version":  ipVersion,
			"shared":      shared,
			"created_at":  createdAt.Format(time.RFC3339),
			"updated_at":  updatedAt.Format(time.RFC3339),
		},
	})
}

// UpdateAddressScope updates an address scope
func (svc *Service) UpdateAddressScope(c *gin.Context) {
	scopeID := c.Param("id")
	projectID := c.GetString("project_id")

	var req struct {
		AddressScope struct {
			Name   *string `json:"name"`
			Shared *bool   `json:"shared"`
		} `json:"address_scope" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Check if scope exists
	var exists bool
	err := database.DB.QueryRow(c.Request.Context(),
		"SELECT EXISTS(SELECT 1 FROM address_scopes WHERE id = $1 AND project_id = $2)",
		scopeID, projectID,
	).Scan(&exists)

	if err != nil || !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "address scope not found"})
		return
	}

	// Update scope
	_, err = database.DB.Exec(c.Request.Context(), `
		UPDATE address_scopes
		SET name = COALESCE($1, name),
		    shared = COALESCE($2, shared),
		    updated_at = $3
		WHERE id = $4 AND project_id = $5
	`, req.AddressScope.Name, req.AddressScope.Shared, time.Now(), scopeID, projectID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Fetch updated scope
	var (
		projID    string
		name      string
		ipVersion int
		shared    bool
		createdAt time.Time
		updatedAt time.Time
	)

	err = database.DB.QueryRow(c.Request.Context(), `
		SELECT project_id, name, ip_version, shared, created_at, updated_at
		FROM address_scopes
		WHERE id = $1 AND project_id = $2
	`, scopeID, projectID).Scan(&projID, &name, &ipVersion, &shared, &createdAt, &updatedAt)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"address_scope": map[string]interface{}{
			"id":          scopeID,
			"tenant_id":   projID,
			"project_id":  projID,
			"name":        name,
			"ip_version":  ipVersion,
			"shared":      shared,
			"created_at":  createdAt.Format(time.RFC3339),
			"updated_at":  updatedAt.Format(time.RFC3339),
		},
	})
}

// DeleteAddressScope deletes an address scope
func (svc *Service) DeleteAddressScope(c *gin.Context) {
	scopeID := c.Param("id")
	projectID := c.GetString("project_id")

	result, err := database.DB.Exec(c.Request.Context(),
		"DELETE FROM address_scopes WHERE id = $1 AND project_id = $2",
		scopeID, projectID,
	)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if result.RowsAffected() == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "address scope not found"})
		return
	}

	c.Status(http.StatusNoContent)
}
