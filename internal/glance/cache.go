package glance

import (
	"net/http"
	"time"

	"github.com/cobaltcore-dev/o3k/internal/common"
	"github.com/cobaltcore-dev/o3k/internal/database"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

// ListCachedImages lists images in the cache
func (svc *Service) ListCachedImages(c *gin.Context) {
	// Query images that have been marked as cached
	rows, err := database.DB.Query(c.Request.Context(), `
		SELECT id, name, size_bytes, cached_at
		FROM images
		WHERE cached_at IS NOT NULL
		ORDER BY cached_at DESC
	`)
	if err != nil {
		log.Error().Err(err).Str("operation", "list_cached_images").Msg("failed to query cached images")
		common.SendError(c, common.NewInternalServerError("operation failed"))
		return
	}
	defer rows.Close()

	cachedImages := []map[string]interface{}{}
	for rows.Next() {
		var (
			id       string
			name     *string
			size     *int64
			cachedAt time.Time
		)

		err := rows.Scan(&id, &name, &size, &cachedAt)
		if err != nil {
			log.Error().Err(err).Str("operation", "list_cached_images_scan").Msg("failed to scan cached image row")
			common.SendError(c, common.NewInternalServerError("operation failed"))
			return
		}

		image := map[string]interface{}{
			"image_id":      id,
			"last_accessed": cachedAt.Format(time.RFC3339),
			"last_modified": cachedAt.Format(time.RFC3339),
			"size":          size,
		}
		if name != nil {
			image["name"] = *name
		}
		cachedImages = append(cachedImages, image)
	}

	c.JSON(http.StatusOK, gin.H{"cached_images": cachedImages})
}

// PrefetchImage prefetches an image into the cache
func (svc *Service) PrefetchImage(c *gin.Context) {
	imageID := c.Param("id")

	// Verify image exists
	var exists bool
	err := database.DB.QueryRow(c.Request.Context(),
		"SELECT EXISTS(SELECT 1 FROM images WHERE id = $1)",
		imageID,
	).Scan(&exists)

	if err != nil || !exists {
		common.SendError(c, common.NewNotFoundError("image"))
		return
	}

	// Mark image as cached
	_, err = database.DB.Exec(c.Request.Context(), `
		UPDATE images
		SET cached_at = $1
		WHERE id = $2
	`, time.Now(), imageID)

	if err != nil {
		log.Error().Err(err).Str("operation", "prefetch_image").Msg("failed to mark image as cached")
		common.SendError(c, common.NewInternalServerError("operation failed"))
		return
	}

	c.Status(http.StatusAccepted)
}

// DeleteCachedImage removes an image from the cache
func (svc *Service) DeleteCachedImage(c *gin.Context) {
	imageID := c.Param("id")

	result, err := database.DB.Exec(c.Request.Context(), `
		UPDATE images
		SET cached_at = NULL
		WHERE id = $1 AND cached_at IS NOT NULL
	`, imageID)

	if err != nil {
		log.Error().Err(err).Str("operation", "delete_cached_image").Msg("failed to remove image from cache")
		common.SendError(c, common.NewInternalServerError("operation failed"))
		return
	}

	if result.RowsAffected() == 0 {
		common.SendError(c, common.NewNotFoundError("cached image"))
		return
	}

	c.Status(http.StatusNoContent)
}

// ClearCache clears all cached images
func (svc *Service) ClearCache(c *gin.Context) {
	_, err := database.DB.Exec(c.Request.Context(), `
		UPDATE images
		SET cached_at = NULL
		WHERE cached_at IS NOT NULL
	`)

	if err != nil {
		log.Error().Err(err).Str("operation", "clear_cache").Msg("failed to clear image cache")
		common.SendError(c, common.NewInternalServerError("operation failed"))
		return
	}

	c.Status(http.StatusNoContent)
}
