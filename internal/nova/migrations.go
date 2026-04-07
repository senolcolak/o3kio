package nova

import (
	"net/http"
	"time"

	"github.com/cobaltcore-dev/o3k/internal/common"
	"github.com/cobaltcore-dev/o3k/internal/database"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

// ListMigrations handles GET /v2.1/:project_id/os-migrations
func (svc *Service) ListMigrations(c *gin.Context) {
	// List all migrations (admin endpoint in real OpenStack)
	rows, err := database.DB.Query(c.Request.Context(), `
		SELECT id, server_uuid, source_node, dest_node, old_flavor_id, new_flavor_id,
		       status, migration_type, created_at, updated_at
		FROM server_migrations
		ORDER BY created_at DESC
	`)

	if err != nil {
		log.Error().Err(err).Str("operation", "list_migrations").Msg("database error")
		common.SendError(c, common.NewInternalServerError("failed to list migrations"))
		return
	}
	defer rows.Close()

	migrations := []map[string]interface{}{}
	for rows.Next() {
		var id, serverUUID uuid.UUID
		var oldFlavorID, newFlavorID *uuid.UUID
		var sourceNode, destNode, status, migrationType string
		var createdAt, updatedAt time.Time

		err := rows.Scan(&id, &serverUUID, &sourceNode, &destNode, &oldFlavorID, &newFlavorID,
			&status, &migrationType, &createdAt, &updatedAt)
		if err != nil {
			log.Warn().Err(err).Msg("failed to scan migration row")
			continue
		}

		migration := map[string]interface{}{
			"id":             id.String(),
			"server_uuid":    serverUUID.String(),
			"source_node":    sourceNode,
			"dest_node":      destNode,
			"status":         status,
			"migration_type": migrationType,
			"created_at":     createdAt.Format(time.RFC3339),
			"updated_at":     updatedAt.Format(time.RFC3339),
		}

		if oldFlavorID != nil {
			migration["old_instance_type_id"] = oldFlavorID.String()
		}
		if newFlavorID != nil {
			migration["new_instance_type_id"] = newFlavorID.String()
		}

		migrations = append(migrations, migration)
	}

	c.JSON(http.StatusOK, gin.H{"migrations": migrations})
}

// ListServerMigrations handles GET /v2.1/:project_id/servers/:id/migrations
func (svc *Service) ListServerMigrations(c *gin.Context) {
	serverID := c.Param("id")

	rows, err := database.DB.Query(c.Request.Context(), `
		SELECT id, server_uuid, source_node, dest_node, old_flavor_id, new_flavor_id,
		       status, migration_type, created_at, updated_at
		FROM server_migrations
		WHERE server_uuid = $1
		ORDER BY created_at DESC
	`, serverID)

	if err != nil {
		log.Error().Err(err).Str("operation", "list_server_migrations").Msg("database error")
		common.SendError(c, common.NewInternalServerError("failed to list server migrations"))
		return
	}
	defer rows.Close()

	migrations := []map[string]interface{}{}
	for rows.Next() {
		var id, serverUUID uuid.UUID
		var oldFlavorID, newFlavorID *uuid.UUID
		var sourceNode, destNode, status, migrationType string
		var createdAt, updatedAt time.Time

		err := rows.Scan(&id, &serverUUID, &sourceNode, &destNode, &oldFlavorID, &newFlavorID,
			&status, &migrationType, &createdAt, &updatedAt)
		if err != nil {
			log.Warn().Err(err).Msg("failed to scan migration row")
			continue
		}

		migration := map[string]interface{}{
			"id":             id.String(),
			"server_uuid":    serverUUID.String(),
			"source_node":    sourceNode,
			"dest_node":      destNode,
			"status":         status,
			"migration_type": migrationType,
			"created_at":     createdAt.Format(time.RFC3339),
			"updated_at":     updatedAt.Format(time.RFC3339),
		}

		if oldFlavorID != nil {
			migration["old_instance_type_id"] = oldFlavorID.String()
		}
		if newFlavorID != nil {
			migration["new_instance_type_id"] = newFlavorID.String()
		}

		migrations = append(migrations, migration)
	}

	c.JSON(http.StatusOK, gin.H{"migrations": migrations})
}

// GetServerMigration handles GET /v2.1/:project_id/servers/:id/migrations/:migration_id
func (svc *Service) GetServerMigration(c *gin.Context) {
	serverID := c.Param("id")
	migrationID := c.Param("migration_id")

	var id, serverUUID uuid.UUID
	var oldFlavorID, newFlavorID *uuid.UUID
	var sourceNode, destNode, status, migrationType string
	var createdAt, updatedAt time.Time

	err := database.DB.QueryRow(c.Request.Context(), `
		SELECT id, server_uuid, source_node, dest_node, old_flavor_id, new_flavor_id,
		       status, migration_type, created_at, updated_at
		FROM server_migrations
		WHERE id = $1 AND server_uuid = $2
	`, migrationID, serverID).Scan(&id, &serverUUID, &sourceNode, &destNode, &oldFlavorID, &newFlavorID,
		&status, &migrationType, &createdAt, &updatedAt)

	if err != nil {
		common.SendError(c, common.NewNotFoundError("migration"))
		return
	}

	migration := map[string]interface{}{
		"id":             id.String(),
		"server_uuid":    serverUUID.String(),
		"source_node":    sourceNode,
		"dest_node":      destNode,
		"status":         status,
		"migration_type": migrationType,
		"created_at":     createdAt.Format(time.RFC3339),
		"updated_at":     updatedAt.Format(time.RFC3339),
	}

	if oldFlavorID != nil {
		migration["old_instance_type_id"] = oldFlavorID.String()
	}
	if newFlavorID != nil {
		migration["new_instance_type_id"] = newFlavorID.String()
	}

	c.JSON(http.StatusOK, gin.H{"migration": migration})
}

// DeleteServerMigration handles DELETE /v2.1/:project_id/servers/:id/migrations/:migration_id
func (svc *Service) DeleteServerMigration(c *gin.Context) {
	serverID := c.Param("id")
	migrationID := c.Param("migration_id")

	// Delete (cancel) migration
	result, err := database.DB.Exec(c.Request.Context(),
		"DELETE FROM server_migrations WHERE id = $1 AND server_uuid = $2",
		migrationID, serverID,
	)

	if err != nil {
		log.Error().Err(err).Str("operation", "delete_server_migration").Msg("database error")
		common.SendError(c, common.NewInternalServerError("failed to delete migration"))
		return
	}

	if result.RowsAffected() == 0 {
		common.SendError(c, common.NewNotFoundError("migration"))
		return
	}

	c.Status(http.StatusNoContent)
}

// ServerMigrationAction handles POST /v2.1/:project_id/servers/:id/migrations/:migration_id/action
func (svc *Service) ServerMigrationAction(c *gin.Context) {
	serverID := c.Param("id")
	migrationID := c.Param("migration_id")

	var req map[string]interface{}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.SendError(c, common.NewBadRequestError("invalid request body"))
		return
	}

	// Handle force_complete action
	if _, ok := req["force_complete"]; ok {
		// Update migration status to completed
		result, err := database.DB.Exec(c.Request.Context(), `
			UPDATE server_migrations
			SET status = $1, updated_at = $2
			WHERE id = $3 AND server_uuid = $4
		`, "completed", time.Now(), migrationID, serverID)

		if err != nil {
			log.Error().Err(err).Str("operation", "force_complete_migration").Msg("database error")
			common.SendError(c, common.NewInternalServerError("failed to complete migration"))
			return
		}

		if result.RowsAffected() == 0 {
			common.SendError(c, common.NewNotFoundError("migration"))
			return
		}

		c.Status(http.StatusAccepted)
		return
	}

	common.SendError(c, common.NewBadRequestError("Unknown action"))
}
