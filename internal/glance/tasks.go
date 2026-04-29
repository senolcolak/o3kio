package glance

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// CreateTask handles POST /v2/tasks
func (svc *Service) CreateTask(c *gin.Context) {
	var req struct {
		Type  string                 `json:"type" binding:"required"`
		Input map[string]interface{} `json:"input"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}

	taskID := uuid.New().String()
	now := time.Now()
	owner := c.GetString("user_id")

	_, err := svc.activeDB().Exec(c.Request.Context(), `
		INSERT INTO image_tasks (id, type, status, input, owner, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, taskID, req.Type, "pending", req.Input, owner, now, now)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"id":         taskID,
		"type":       req.Type,
		"status":     "pending",
		"input":      req.Input,
		"owner":      owner,
		"created_at": now.Format(time.RFC3339),
		"updated_at": now.Format(time.RFC3339),
		"self":       "/v2/tasks/" + taskID,
		"schema":     "/v2/schemas/task",
	})
}

// ListTasks handles GET /v2/tasks
func (svc *Service) ListTasks(c *gin.Context) {
	rows, err := svc.activeDB().Query(c.Request.Context(), `
		SELECT id, type, status, input, result, owner, created_at, updated_at
		FROM image_tasks
		ORDER BY created_at DESC
		LIMIT 100
	`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}
	defer rows.Close()

	tasks := []gin.H{}
	for rows.Next() {
		var id, taskType, status, owner string
		var input, result map[string]interface{}
		var createdAt, updatedAt time.Time

		if err := rows.Scan(&id, &taskType, &status, &input, &result, &owner, &createdAt, &updatedAt); err != nil {
			continue
		}

		task := gin.H{
			"id":         id,
			"type":       taskType,
			"status":     status,
			"owner":      owner,
			"created_at": createdAt.Format(time.RFC3339),
			"updated_at": updatedAt.Format(time.RFC3339),
			"self":       "/v2/tasks/" + id,
			"schema":     "/v2/schemas/task",
		}

		if input != nil {
			task["input"] = input
		}
		if result != nil {
			task["result"] = result
		}

		tasks = append(tasks, task)
	}

	c.JSON(http.StatusOK, gin.H{
		"tasks":  tasks,
		"schema": "/v2/schemas/tasks",
	})
}

// GetTask handles GET /v2/tasks/:id
func (svc *Service) GetTask(c *gin.Context) {
	taskID := c.Param("id")

	var taskType, status, owner, message string
	var inputJSON, resultJSON []byte
	var createdAt, updatedAt time.Time

	err := svc.activeDB().QueryRow(c.Request.Context(), `
		SELECT type, status, input, result, COALESCE(owner, ''), COALESCE(message, ''), created_at, updated_at
		FROM image_tasks
		WHERE id = $1
	`, taskID).Scan(&taskType, &status, &inputJSON, &resultJSON, &owner, &message, &createdAt, &updatedAt)

	if err != nil {
		// Log the actual error for debugging
		c.JSON(http.StatusNotFound, gin.H{"message": "Task not found", "debug_error": err.Error()})
		return
	}

	task := gin.H{
		"id":         taskID,
		"type":       taskType,
		"status":     status,
		"owner":      owner,
		"message":    message,
		"created_at": createdAt.Format(time.RFC3339),
		"updated_at": updatedAt.Format(time.RFC3339),
		"self":       "/v2/tasks/" + taskID,
		"schema":     "/v2/schemas/task",
	}

	// Parse input JSON if present
	if len(inputJSON) > 0 {
		var input map[string]interface{}
		if err := json.Unmarshal(inputJSON, &input); err == nil {
			task["input"] = input
		}
	}

	// Parse result JSON if present
	if len(resultJSON) > 0 {
		var result map[string]interface{}
		if err := json.Unmarshal(resultJSON, &result); err == nil {
			task["result"] = result
		}
	}

	c.JSON(http.StatusOK, task)
}

// ListStores handles GET /v2/stores
func (svc *Service) ListStores(c *gin.Context) {
	// Return configured stores
	// In stub mode, return a simple list
	stores := []gin.H{
		{
			"id":          "local",
			"description": "Local filesystem store",
			"default":     true,
		},
	}

	// If RBD or S3 configured, add them
	if svc.storageMode == "rbd" || svc.storageMode == "local,rbd" {
		stores = append(stores, gin.H{
			"id":          "rbd",
			"description": "Ceph RBD store",
		})
	}

	if svc.storageMode == "s3" || svc.storageMode == "local,s3" {
		stores = append(stores, gin.H{
			"id":          "s3",
			"description": "S3-compatible object store",
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"stores": stores,
	})
}

// GetStoresInfo handles GET /v2/stores/info
func (svc *Service) GetStoresInfo(c *gin.Context) {
	// Return detailed store information
	stores := []gin.H{
		{
			"id": "local",
			"properties": gin.H{
				"chunk_size":    65536,
				"store_type":    "file",
				"filesystem_store_datadir": "/var/lib/o3k/images",
			},
		},
	}

	c.JSON(http.StatusOK, gin.H{
		"stores": stores,
	})
}
