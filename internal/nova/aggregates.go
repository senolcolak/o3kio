package nova

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/cobaltcore-dev/o3k/internal/common"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog/log"
)

// ListAggregates lists all host aggregates
func (svc *Service) ListAggregates(c *gin.Context) {
	rows, err := svc.activeDB().Query(c.Request.Context(), `
		SELECT uuid, name, availability_zone, metadata, hosts, created_at, updated_at
		FROM host_aggregates
		ORDER BY created_at DESC
	`)
	if err != nil {
		log.Error().Err(err).Str("operation", "list_aggregates").Msg("database error")
		common.SendError(c, common.NewInternalServerError("failed to list aggregates"))
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
			log.Error().Err(err).Str("operation", "scan_aggregate").Msg("database error")
			common.SendError(c, common.NewInternalServerError("failed to read aggregates"))
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
	if err := rows.Err(); err != nil {
		log.Error().Err(err).Str("operation", "list_aggregates").Msg("rows iteration error")
		common.SendError(c, common.NewInternalServerError("failed to list aggregates"))
		return
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
		common.SendError(c, common.NewBadRequestError("invalid request body"))
		return
	}

	aggregateUUID := uuid.New().String()
	now := time.Now()

	var availabilityZone *string
	if req.Aggregate.AvailabilityZone != "" {
		availabilityZone = &req.Aggregate.AvailabilityZone
	}

	_, err := svc.activeDB().Exec(c.Request.Context(), `
		INSERT INTO host_aggregates (uuid, name, availability_zone, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5)
	`, aggregateUUID, req.Aggregate.Name, availabilityZone, now, now)

	if err != nil {
		log.Error().Err(err).Str("operation", "create_aggregate").Msg("database error")
		common.SendError(c, common.NewInternalServerError("failed to create aggregate"))
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

	err := svc.activeDB().QueryRow(c.Request.Context(), `
		SELECT name, availability_zone, metadata, hosts, created_at, updated_at
		FROM host_aggregates
		WHERE uuid = $1
	`, aggregateID).Scan(&name, &availabilityZone, &metadata, &hosts, &createdAt, &updatedAt)

	if errors.Is(err, pgx.ErrNoRows) {
		common.SendError(c, common.NewNotFoundError("aggregate"))
		return
	}
	if err != nil {
		log.Error().Err(err).Str("operation", "get_aggregate").Msg("database error")
		common.SendError(c, common.NewInternalServerError("failed to get aggregate"))
		return
	}

	var metadataMap map[string]interface{}
	if len(metadata) > 0 {
		_ = svc.activeDB().QueryRow(c.Request.Context(), "SELECT $1::jsonb", metadata).Scan(&metadataMap)
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
		common.SendError(c, common.NewBadRequestError("invalid request body"))
		return
	}

	// Check if aggregate exists
	var exists bool
	err := svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT EXISTS(SELECT 1 FROM host_aggregates WHERE uuid = $1)",
		aggregateID,
	).Scan(&exists)

	if err != nil || !exists {
		common.SendError(c, common.NewNotFoundError("aggregate"))
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
		common.SendError(c, common.NewBadRequestError("no fields to update"))
		return
	}

	args = append(args, time.Now(), aggregateID)
	query := fmt.Sprintf("UPDATE host_aggregates SET %s, updated_at = $%d WHERE uuid = $%d",
		strings.Join(updates, ", "), argPos, argPos+1)

	_, err = svc.activeDB().Exec(c.Request.Context(), query, args...)

	if err != nil {
		log.Error().Err(err).Str("operation", "update_aggregate").Msg("database error")
		common.SendError(c, common.NewInternalServerError("failed to update aggregate"))
		return
	}

	// Return updated aggregate
	svc.GetAggregate(c)
}

// DeleteAggregate deletes a host aggregate
func (svc *Service) DeleteAggregate(c *gin.Context) {
	aggregateID := c.Param("id")

	result, err := svc.activeDB().Exec(c.Request.Context(),
		"DELETE FROM host_aggregates WHERE uuid = $1",
		aggregateID,
	)

	if err != nil {
		log.Error().Err(err).Str("operation", "delete_aggregate").Msg("database error")
		common.SendError(c, common.NewInternalServerError("failed to delete aggregate"))
		return
	}

	if result.RowsAffected() == 0 {
		common.SendError(c, common.NewNotFoundError("aggregate"))
		return
	}

	c.Status(http.StatusOK)
}

// AggregateAction handles aggregate actions (add_host, remove_host, set_metadata)
func (svc *Service) AggregateAction(c *gin.Context) {
	aggregateID := c.Param("id")

	var req map[string]interface{}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.SendError(c, common.NewBadRequestError("invalid request body"))
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

	common.SendError(c, common.NewBadRequestError("unknown action"))
}

// AddHostToAggregate adds a host to an aggregate
func (svc *Service) AddHostToAggregate(c *gin.Context, aggregateID string, actionData interface{}) {
	actionMap, ok := actionData.(map[string]interface{})
	if !ok {
		common.SendError(c, common.NewBadRequestError("invalid add_host data"))
		return
	}

	hostName, ok := actionMap["host"].(string)
	if !ok || hostName == "" {
		common.SendError(c, common.NewBadRequestError("host name required"))
		return
	}

	// Check if aggregate exists and get current hosts
	var hosts []string
	err := svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT hosts FROM host_aggregates WHERE uuid = $1",
		aggregateID,
	).Scan(&hosts)

	if errors.Is(err, pgx.ErrNoRows) {
		common.SendError(c, common.NewNotFoundError("aggregate"))
		return
	}
	if err != nil {
		log.Error().Err(err).Str("operation", "get_aggregate_hosts").Msg("database error")
		common.SendError(c, common.NewInternalServerError("failed to get aggregate"))
		return
	}

	// Add host if not already present
	for _, h := range hosts {
		if h == hostName {
			common.SendError(c, common.NewConflictError("host already in aggregate"))
			return
		}
	}

	hosts = append(hosts, hostName)

	_, err = svc.activeDB().Exec(c.Request.Context(), `
		UPDATE host_aggregates
		SET hosts = $1, updated_at = $2
		WHERE uuid = $3
	`, hosts, time.Now(), aggregateID)

	if err != nil {
		log.Error().Err(err).Str("operation", "add_host_to_aggregate").Msg("database error")
		common.SendError(c, common.NewInternalServerError("failed to add host to aggregate"))
		return
	}

	svc.GetAggregate(c)
}

// RemoveHostFromAggregate removes a host from an aggregate
func (svc *Service) RemoveHostFromAggregate(c *gin.Context, aggregateID string, actionData interface{}) {
	actionMap, ok := actionData.(map[string]interface{})
	if !ok {
		common.SendError(c, common.NewBadRequestError("invalid remove_host data"))
		return
	}

	hostName, ok := actionMap["host"].(string)
	if !ok || hostName == "" {
		common.SendError(c, common.NewBadRequestError("host name required"))
		return
	}

	// Check if aggregate exists and get current hosts
	var hosts []string
	err := svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT hosts FROM host_aggregates WHERE uuid = $1",
		aggregateID,
	).Scan(&hosts)

	if errors.Is(err, pgx.ErrNoRows) {
		common.SendError(c, common.NewNotFoundError("aggregate"))
		return
	}
	if err != nil {
		log.Error().Err(err).Str("operation", "get_aggregate_hosts_remove").Msg("database error")
		common.SendError(c, common.NewInternalServerError("failed to get aggregate"))
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
		common.SendError(c, common.NewNotFoundError("host in aggregate"))
		return
	}

	_, err = svc.activeDB().Exec(c.Request.Context(), `
		UPDATE host_aggregates
		SET hosts = $1, updated_at = $2
		WHERE uuid = $3
	`, newHosts, time.Now(), aggregateID)

	if err != nil {
		log.Error().Err(err).Str("operation", "remove_host_from_aggregate").Msg("database error")
		common.SendError(c, common.NewInternalServerError("failed to remove host from aggregate"))
		return
	}

	svc.GetAggregate(c)
}

// SetAggregateMetadata sets metadata for an aggregate
func (svc *Service) SetAggregateMetadata(c *gin.Context, aggregateID string, actionData interface{}) {
	actionMap, ok := actionData.(map[string]interface{})
	if !ok {
		common.SendError(c, common.NewBadRequestError("invalid set_metadata data"))
		return
	}

	metadata, ok := actionMap["metadata"].(map[string]interface{})
	if !ok {
		common.SendError(c, common.NewBadRequestError("metadata required"))
		return
	}

	// Check if aggregate exists
	var exists bool
	err := svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT EXISTS(SELECT 1 FROM host_aggregates WHERE uuid = $1)",
		aggregateID,
	).Scan(&exists)

	if err != nil || !exists {
		common.SendError(c, common.NewNotFoundError("aggregate"))
		return
	}

	// Update metadata (merge with existing)
	_, err = svc.activeDB().Exec(c.Request.Context(), `
		UPDATE host_aggregates
		SET metadata = metadata || $1::jsonb, updated_at = $2
		WHERE uuid = $3
	`, metadata, time.Now(), aggregateID)

	if err != nil {
		log.Error().Err(err).Str("operation", "set_aggregate_metadata").Msg("database error")
		common.SendError(c, common.NewInternalServerError("failed to set aggregate metadata"))
		return
	}

	svc.GetAggregate(c)
}
