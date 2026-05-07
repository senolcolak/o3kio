package glance

import (
	"net/http"
	"time"

	"github.com/cobaltcore-dev/o3k/internal/common"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

// ListMetadefNamespaces handles GET /v2/metadefs/namespaces
func (svc *Service) ListMetadefNamespaces(c *gin.Context) {
	rows, err := svc.activeDB().Query(c.Request.Context(), `
		SELECT namespace, display_name, description, visibility, protected, owner, created_at, updated_at
		FROM metadef_namespaces
		ORDER BY namespace ASC
	`)

	if err != nil {
		log.Error().Err(err).Str("operation", "list_metadef_namespaces").Msg("failed to query metadef namespaces")
		common.SendError(c, common.NewInternalServerError("operation failed"))
		return
	}
	defer rows.Close()

	namespaces := []map[string]interface{}{}
	for rows.Next() {
		var namespace, visibility string
		var displayName, description, owner *string
		var protected bool
		var createdAt, updatedAt time.Time

		err := rows.Scan(&namespace, &displayName, &description, &visibility, &protected, &owner, &createdAt, &updatedAt)
		if err != nil {
			continue
		}

		ns := map[string]interface{}{
			"namespace":  namespace,
			"visibility": visibility,
			"protected":  protected,
			"created_at": createdAt.Format(time.RFC3339),
			"updated_at": updatedAt.Format(time.RFC3339),
		}

		if displayName != nil {
			ns["display_name"] = *displayName
		}
		if description != nil {
			ns["description"] = *description
		}
		if owner != nil {
			ns["owner"] = *owner
		}

		namespaces = append(namespaces, ns)
	}
	if err := rows.Err(); err != nil {
		log.Error().Err(err).Str("operation", "list_metadef_namespaces").Msg("rows iteration error")
		common.SendError(c, common.NewInternalServerError("operation failed"))
		return
	}

	c.JSON(http.StatusOK, gin.H{"namespaces": namespaces})
}

// CreateMetadefNamespace handles POST /v2/metadefs/namespaces
func (svc *Service) CreateMetadefNamespace(c *gin.Context) {
	var req map[string]interface{}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.SendError(c, common.NewBadRequestError("invalid request body"))
		return
	}

	namespace, ok := req["namespace"].(string)
	if !ok || namespace == "" {
		common.SendError(c, common.NewBadRequestError("namespace is required"))
		return
	}

	displayName, _ := req["display_name"].(string)
	description, _ := req["description"].(string)
	visibility, _ := req["visibility"].(string)
	if visibility == "" {
		visibility = "public"
	}
	owner, _ := req["owner"].(string)
	protected, _ := req["protected"].(bool)

	_, err := svc.activeDB().Exec(c.Request.Context(), `
		INSERT INTO metadef_namespaces (namespace, display_name, description, visibility, protected, owner, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, namespace, displayName, description, visibility, protected, owner, time.Now(), time.Now())

	if err != nil {
		common.SendError(c, common.NewConflictError("Namespace already exists"))
		return
	}

	// Handle resource type associations if provided
	if rtAssocs, ok := req["resource_type_associations"].([]interface{}); ok {
		for _, assoc := range rtAssocs {
			if assocMap, ok := assoc.(map[string]interface{}); ok {
				rtName, _ := assocMap["name"].(string)
				rtPrefix, _ := assocMap["prefix"].(string)

				svc.activeDB().Exec(c.Request.Context(), `
					INSERT INTO metadef_resource_types (namespace, name, prefix, created_at)
					VALUES ($1, $2, $3, $4)
				`, namespace, rtName, rtPrefix, time.Now())
			}
		}
	}

	result := map[string]interface{}{
		"namespace":  namespace,
		"visibility": visibility,
		"protected":  protected,
		"created_at": time.Now().Format(time.RFC3339),
		"updated_at": time.Now().Format(time.RFC3339),
	}

	if displayName != "" {
		result["display_name"] = displayName
	}
	if description != "" {
		result["description"] = description
	}
	if owner != "" {
		result["owner"] = owner
	}

	c.JSON(http.StatusCreated, result)
}

// GetMetadefNamespace handles GET /v2/metadefs/namespaces/:namespace
func (svc *Service) GetMetadefNamespace(c *gin.Context) {
	namespace := c.Param("namespace")

	var ns, visibility string
	var displayName, description, owner *string
	var protected bool
	var createdAt, updatedAt time.Time

	err := svc.activeDB().QueryRow(c.Request.Context(), `
		SELECT namespace, display_name, description, visibility, protected, owner, created_at, updated_at
		FROM metadef_namespaces
		WHERE namespace = $1
	`, namespace).Scan(&ns, &displayName, &description, &visibility, &protected, &owner, &createdAt, &updatedAt)

	if err != nil {
		common.SendError(c, common.NewNotFoundError("Namespace"))
		return
	}

	result := map[string]interface{}{
		"namespace":  ns,
		"visibility": visibility,
		"protected":  protected,
		"created_at": createdAt.Format(time.RFC3339),
		"updated_at": updatedAt.Format(time.RFC3339),
	}

	if displayName != nil {
		result["display_name"] = *displayName
	}
	if description != nil {
		result["description"] = *description
	}
	if owner != nil {
		result["owner"] = *owner
	}

	// Get resource type associations
	rows, err := svc.activeDB().Query(c.Request.Context(), `
		SELECT name, prefix FROM metadef_resource_types WHERE namespace = $1
	`, namespace)

	if err == nil {
		defer rows.Close()
		var rtAssocs []map[string]interface{}
		for rows.Next() {
			var name string
			var prefix *string
			rows.Scan(&name, &prefix)

			rt := map[string]interface{}{
				"name": name,
			}
			if prefix != nil {
				rt["prefix"] = *prefix
			}

			rtAssocs = append(rtAssocs, rt)
		}
		// rows.Err() non-critical for resource type associations

		if len(rtAssocs) > 0 {
			result["resource_type_associations"] = rtAssocs
		}
	}

	c.JSON(http.StatusOK, result)
}

// UpdateMetadefNamespace handles PUT /v2/metadefs/namespaces/:namespace
func (svc *Service) UpdateMetadefNamespace(c *gin.Context) {
	namespace := c.Param("namespace")

	var req map[string]interface{}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.SendError(c, common.NewBadRequestError("invalid request body"))
		return
	}

	// Verify namespace exists
	var exists bool
	err := svc.activeDB().QueryRow(c.Request.Context(), `
		SELECT EXISTS(SELECT 1 FROM metadef_namespaces WHERE namespace = $1)
	`, namespace).Scan(&exists)

	if err != nil || !exists {
		common.SendError(c, common.NewNotFoundError("Namespace"))
		return
	}

	displayName, _ := req["display_name"].(string)
	description, _ := req["description"].(string)
	visibility, _ := req["visibility"].(string)
	protected, _ := req["protected"].(bool)

	_, err = svc.activeDB().Exec(c.Request.Context(), `
		UPDATE metadef_namespaces
		SET display_name = $1, description = $2, visibility = $3, protected = $4, updated_at = $5
		WHERE namespace = $6
	`, displayName, description, visibility, protected, time.Now(), namespace)

	if err != nil {
		log.Error().Err(err).Str("operation", "update_metadef_namespace").Msg("failed to update metadef namespace")
		common.SendError(c, common.NewInternalServerError("operation failed"))
		return
	}

	result := map[string]interface{}{
		"namespace":  namespace,
		"visibility": visibility,
		"protected":  protected,
		"updated_at": time.Now().Format(time.RFC3339),
	}

	if displayName != "" {
		result["display_name"] = displayName
	}
	if description != "" {
		result["description"] = description
	}

	c.JSON(http.StatusOK, result)
}

// DeleteMetadefNamespace handles DELETE /v2/metadefs/namespaces/:namespace
func (svc *Service) DeleteMetadefNamespace(c *gin.Context) {
	namespace := c.Param("namespace")

	result, err := svc.activeDB().Exec(c.Request.Context(),
		"DELETE FROM metadef_namespaces WHERE namespace = $1",
		namespace,
	)

	if err != nil {
		log.Error().Err(err).Str("operation", "delete_metadef_namespace").Msg("failed to delete metadef namespace")
		common.SendError(c, common.NewInternalServerError("operation failed"))
		return
	}

	if result.RowsAffected() == 0 {
		common.SendError(c, common.NewNotFoundError("Namespace"))
		return
	}

	c.Status(http.StatusNoContent)
}

// ListMetadefResourceTypes handles GET /v2/metadefs/resource_types
func (svc *Service) ListMetadefResourceTypes(c *gin.Context) {
	// Return predefined resource types
	resourceTypes := []map[string]interface{}{
		{
			"name":       "OS::Glance::Image",
			"created_at": time.Now().Format(time.RFC3339),
			"updated_at": time.Now().Format(time.RFC3339),
		},
		{
			"name":       "OS::Cinder::Volume",
			"created_at": time.Now().Format(time.RFC3339),
			"updated_at": time.Now().Format(time.RFC3339),
		},
		{
			"name":       "OS::Nova::Flavor",
			"created_at": time.Now().Format(time.RFC3339),
			"updated_at": time.Now().Format(time.RFC3339),
		},
		{
			"name":       "OS::Nova::Instance",
			"created_at": time.Now().Format(time.RFC3339),
			"updated_at": time.Now().Format(time.RFC3339),
		},
	}

	c.JSON(http.StatusOK, gin.H{"resource_types": resourceTypes})
}
