package nova

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/cobaltcore-dev/o3k/internal/database"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// ListAggregates lists all host aggregates
func (svc *Service) ListAggregates(c *gin.Context) {
	rows, err := database.DB.Query(c.Request.Context(), `
		SELECT uuid, name, availability_zone, metadata, hosts, created_at, updated_at
		FROM host_aggregates
		ORDER BY created_at DESC
	`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	aggregates := []map[string]interface{}{}
	for rows.Next() {
		var (
			id               string
			name             string
			availabilityZone *string
			metadata         []byte
			hosts            []string
			createdAt        time.Time
			updatedAt        time.Time
		)

		err := rows.Scan(&id, &name, &availabilityZone, &metadata, &hosts, &createdAt, &updatedAt)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		var metadataMap map[string]interface{}
		if len(metadata) > 0 {
			json.Unmarshal(metadata, &metadataMap)
		}
		if metadataMap == nil {
			metadataMap = make(map[string]interface{})
		}

		agg := map[string]interface{}{
			"id":                id,
			"name":              name,
			"availability_zone": availabilityZone,
			"metadata":          metadataMap,
			"hosts":             hosts,
			"created_at":        createdAt.Format(time.RFC3339),
			"updated_at":        updatedAt.Format(time.RFC3339),
		}
		aggregates = append(aggregates, agg)
	}

	c.JSON(http.StatusOK, gin.H{"aggregates": aggregates})
}

// CreateAggregate creates a new host aggregate
func (svc *Service) CreateAggregate(c *gin.Context) {
	var req struct {
		Aggregate struct {
			Name             string `json:"name" binding:"required"`
			AvailabilityZone string `json:"availability_zone"`
		} `json:"aggregate" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	aggregateUUID := uuid.New().String()
	now := time.Now()

	var availabilityZone *string
	if req.Aggregate.AvailabilityZone != "" {
		availabilityZone = &req.Aggregate.AvailabilityZone
	}

	_, err := database.DB.Exec(c.Request.Context(), `
		INSERT INTO host_aggregates (uuid, name, availability_zone, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5)
	`, aggregateUUID, req.Aggregate.Name, availabilityZone, now, now)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"aggregate": map[string]interface{}{
			"id":                aggregateUUID,
			"name":              req.Aggregate.Name,
			"availability_zone": availabilityZone,
			"metadata":          map[string]interface{}{},
			"hosts":             []string{},
			"created_at":        now.Format(time.RFC3339),
			"updated_at":        now.Format(time.RFC3339),
		},
	})
}

// GetAggregate retrieves a specific host aggregate
func (svc *Service) GetAggregate(c *gin.Context) {
	aggregateID := c.Param("id")

	var (
		name             string
		availabilityZone *string
		metadata         []byte
		hosts            []string
		createdAt        time.Time
		updatedAt        time.Time
	)

	err := database.DB.QueryRow(c.Request.Context(), `
		SELECT name, availability_zone, metadata, hosts, created_at, updated_at
		FROM host_aggregates
		WHERE uuid = $1
	`, aggregateID).Scan(&name, &availabilityZone, &metadata, &hosts, &createdAt, &updatedAt)

	if err == pgx.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "aggregate not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var metadataMap map[string]interface{}
	if len(metadata) > 0 {
		_ = database.DB.QueryRow(c.Request.Context(), "SELECT $1::jsonb", metadata).Scan(&metadataMap)
	}
	if metadataMap == nil {
		metadataMap = make(map[string]interface{})
	}

	c.JSON(http.StatusOK, gin.H{
		"aggregate": map[string]interface{}{
			"id":                aggregateID,
			"name":              name,
			"availability_zone": availabilityZone,
			"metadata":          metadataMap,
			"hosts":             hosts,
			"created_at":        createdAt.Format(time.RFC3339),
			"updated_at":        updatedAt.Format(time.RFC3339),
		},
	})
}

// UpdateAggregate updates a host aggregate
func (svc *Service) UpdateAggregate(c *gin.Context) {
	aggregateID := c.Param("id")

	var req struct {
		Aggregate struct {
			Name             *string `json:"name"`
			AvailabilityZone *string `json:"availability_zone"`
		} `json:"aggregate" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Check if aggregate exists
	var exists bool
	err := database.DB.QueryRow(c.Request.Context(),
		"SELECT EXISTS(SELECT 1 FROM host_aggregates WHERE uuid = $1)",
		aggregateID,
	).Scan(&exists)

	if err != nil || !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "aggregate not found"})
		return
	}

	// Build update query
	updates := []string{}
	args := []interface{}{}
	argPos := 1

	if req.Aggregate.Name != nil {
		updates = append(updates, fmt.Sprintf("name = $%d", argPos))
		args = append(args, *req.Aggregate.Name)
		argPos++
	}

	if req.Aggregate.AvailabilityZone != nil {
		updates = append(updates, fmt.Sprintf("availability_zone = $%d", argPos))
		args = append(args, *req.Aggregate.AvailabilityZone)
		argPos++
	}

	if len(updates) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no fields to update"})
		return
	}

	args = append(args, time.Now(), aggregateID)
	query := fmt.Sprintf("UPDATE host_aggregates SET %s, updated_at = $%d WHERE uuid = $%d",
		strings.Join(updates, ", "), argPos, argPos+1)

	_, err = database.DB.Exec(c.Request.Context(), query, args...)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Return updated aggregate
	svc.GetAggregate(c)
}

// DeleteAggregate deletes a host aggregate
func (svc *Service) DeleteAggregate(c *gin.Context) {
	aggregateID := c.Param("id")

	result, err := database.DB.Exec(c.Request.Context(),
		"DELETE FROM host_aggregates WHERE uuid = $1",
		aggregateID,
	)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if result.RowsAffected() == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "aggregate not found"})
		return
	}

	c.Status(http.StatusOK)
}

// AggregateAction handles aggregate actions (add_host, remove_host, set_metadata)
func (svc *Service) AggregateAction(c *gin.Context) {
	aggregateID := c.Param("id")

	var req map[string]interface{}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	if addHost, ok := req["add_host"]; ok {
		svc.AddHostToAggregate(c, aggregateID, addHost)
		return
	}

	if removeHost, ok := req["remove_host"]; ok {
		svc.RemoveHostFromAggregate(c, aggregateID, removeHost)
		return
	}

	if setMetadata, ok := req["set_metadata"]; ok {
		svc.SetAggregateMetadata(c, aggregateID, setMetadata)
		return
	}

	c.JSON(http.StatusBadRequest, gin.H{"error": "unknown action"})
}

// AddHostToAggregate adds a host to an aggregate
func (svc *Service) AddHostToAggregate(c *gin.Context, aggregateID string, actionData interface{}) {
	actionMap, ok := actionData.(map[string]interface{})
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid add_host data"})
		return
	}

	hostName, ok := actionMap["host"].(string)
	if !ok || hostName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "host name required"})
		return
	}

	// Check if aggregate exists and get current hosts
	var hosts []string
	err := database.DB.QueryRow(c.Request.Context(),
		"SELECT hosts FROM host_aggregates WHERE uuid = $1",
		aggregateID,
	).Scan(&hosts)

	if err == pgx.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "aggregate not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Add host if not already present
	for _, h := range hosts {
		if h == hostName {
			c.JSON(http.StatusConflict, gin.H{"error": "host already in aggregate"})
			return
		}
	}

	hosts = append(hosts, hostName)

	_, err = database.DB.Exec(c.Request.Context(), `
		UPDATE host_aggregates
		SET hosts = $1, updated_at = $2
		WHERE uuid = $3
	`, hosts, time.Now(), aggregateID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	svc.GetAggregate(c)
}

// RemoveHostFromAggregate removes a host from an aggregate
func (svc *Service) RemoveHostFromAggregate(c *gin.Context, aggregateID string, actionData interface{}) {
	actionMap, ok := actionData.(map[string]interface{})
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid remove_host data"})
		return
	}

	hostName, ok := actionMap["host"].(string)
	if !ok || hostName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "host name required"})
		return
	}

	// Check if aggregate exists and get current hosts
	var hosts []string
	err := database.DB.QueryRow(c.Request.Context(),
		"SELECT hosts FROM host_aggregates WHERE uuid = $1",
		aggregateID,
	).Scan(&hosts)

	if err == pgx.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "aggregate not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Remove host
	newHosts := []string{}
	found := false
	for _, h := range hosts {
		if h != hostName {
			newHosts = append(newHosts, h)
		} else {
			found = true
		}
	}

	if !found {
		c.JSON(http.StatusNotFound, gin.H{"error": "host not in aggregate"})
		return
	}

	_, err = database.DB.Exec(c.Request.Context(), `
		UPDATE host_aggregates
		SET hosts = $1, updated_at = $2
		WHERE uuid = $3
	`, newHosts, time.Now(), aggregateID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	svc.GetAggregate(c)
}

// SetAggregateMetadata sets metadata for an aggregate
func (svc *Service) SetAggregateMetadata(c *gin.Context, aggregateID string, actionData interface{}) {
	actionMap, ok := actionData.(map[string]interface{})
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid set_metadata data"})
		return
	}

	metadata, ok := actionMap["metadata"].(map[string]interface{})
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "metadata required"})
		return
	}

	// Check if aggregate exists
	var exists bool
	err := database.DB.QueryRow(c.Request.Context(),
		"SELECT EXISTS(SELECT 1 FROM host_aggregates WHERE uuid = $1)",
		aggregateID,
	).Scan(&exists)

	if err != nil || !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "aggregate not found"})
		return
	}

	// Update metadata (merge with existing)
	_, err = database.DB.Exec(c.Request.Context(), `
		UPDATE host_aggregates
		SET metadata = metadata || $1::jsonb, updated_at = $2
		WHERE uuid = $3
	`, metadata, time.Now(), aggregateID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	svc.GetAggregate(c)
}
