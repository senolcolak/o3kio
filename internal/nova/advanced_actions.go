package nova

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/cobaltcore-dev/o3k/internal/database"
)

// Suspend suspends a running instance (saves RAM to disk)
func (svc *Service) SuspendInstance(c *gin.Context) {
	instanceID := c.Param("id")
	projectID := c.GetString("project_id")

	// Get instance and libvirt domain ID
	var libvirtDomainID, status string
	var powerState int
	err := database.DB.QueryRow(c.Request.Context(),
		"SELECT libvirt_domain_id, status, power_state FROM instances WHERE id = $1 AND project_id = $2",
		instanceID, projectID,
	).Scan(&libvirtDomainID, &status, &powerState)

	if err == pgx.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "instance not found"})
		return
	}

	// Can only suspend an active instance
	if status != "ACTIVE" {
		c.JSON(http.StatusConflict, gin.H{"error": fmt.Sprintf("cannot suspend instance in %s state", status)})
		return
	}

	// Update status to SUSPENDED
	_, err = database.DB.Exec(c.Request.Context(), `
		UPDATE instances
		SET status = $1, power_state = $2, task_state = $3, updated_at = $4
		WHERE id = $5
	`, "SUSPENDED", 4, "", time.Now(), instanceID) // power_state 4 = SUSPENDED

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Suspend VM in libvirt (asynchronously)
	if svc.vmManager != nil && libvirtDomainID != "" {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			if err := svc.vmManager.SuspendVM(ctx, libvirtDomainID); err != nil {
				// On failure, revert to ERROR state
				database.DB.Exec(context.Background(),
					"UPDATE instances SET status = $1, task_state = $2 WHERE id = $3",
					"ERROR", "", instanceID)
			}
		}()
	}

	c.Status(http.StatusAccepted)
}

// Resume resumes a suspended instance
func (svc *Service) ResumeInstance(c *gin.Context) {
	instanceID := c.Param("id")
	projectID := c.GetString("project_id")

	// Get instance and libvirt domain ID
	var libvirtDomainID, status string
	err := database.DB.QueryRow(c.Request.Context(),
		"SELECT libvirt_domain_id, status FROM instances WHERE id = $1 AND project_id = $2",
		instanceID, projectID,
	).Scan(&libvirtDomainID, &status)

	if err == pgx.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "instance not found"})
		return
	}

	// Can only resume a suspended instance
	if status != "SUSPENDED" {
		c.JSON(http.StatusConflict, gin.H{"error": fmt.Sprintf("cannot resume instance in %s state", status)})
		return
	}

	// Update status to ACTIVE
	_, err = database.DB.Exec(c.Request.Context(), `
		UPDATE instances
		SET status = $1, power_state = $2, task_state = $3, updated_at = $4
		WHERE id = $5
	`, "ACTIVE", 1, "", time.Now(), instanceID) // power_state 1 = RUNNING

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Resume VM in libvirt (asynchronously)
	if svc.vmManager != nil && libvirtDomainID != "" {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			if err := svc.vmManager.ResumeVM(ctx, libvirtDomainID); err != nil {
				// On failure, revert to ERROR state
				database.DB.Exec(context.Background(),
					"UPDATE instances SET status = $1, task_state = $2 WHERE id = $3",
					"ERROR", "", instanceID)
			}
		}()
	}

	c.Status(http.StatusAccepted)
}

// Shelve shelves an instance (shutdown and save disk image)
func (svc *Service) ShelveInstance(c *gin.Context) {
	instanceID := c.Param("id")
	projectID := c.GetString("project_id")

	// Get instance
	var libvirtDomainID, status string
	err := database.DB.QueryRow(c.Request.Context(),
		"SELECT libvirt_domain_id, status FROM instances WHERE id = $1 AND project_id = $2",
		instanceID, projectID,
	).Scan(&libvirtDomainID, &status)

	if err == pgx.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "instance not found"})
		return
	}

	// Can only shelve an active or stopped instance
	if status != "ACTIVE" && status != "SHUTOFF" {
		c.JSON(http.StatusConflict, gin.H{"error": fmt.Sprintf("cannot shelve instance in %s state", status)})
		return
	}

	// Create snapshot before shelving
	snapshotName := fmt.Sprintf("shelved-%s-%d", instanceID[:8], time.Now().Unix())

	_, err = database.DB.Exec(c.Request.Context(), `
		INSERT INTO instance_snapshots (id, instance_id, snapshot_name, flavor_id, created_at)
		SELECT gen_random_uuid(), $1, $2, flavor_id, $3
		FROM instances WHERE id = $1
	`, instanceID, snapshotName, time.Now())

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to create snapshot: %v", err)})
		return
	}

	// Update status to SHELVED
	_, err = database.DB.Exec(c.Request.Context(), `
		UPDATE instances
		SET status = $1, power_state = $2, task_state = $3, updated_at = $4
		WHERE id = $5
	`, "SHELVED", 0, "", time.Now(), instanceID) // power_state 0 = NO STATE

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Stop and undefine VM in libvirt (asynchronously)
	if svc.vmManager != nil && libvirtDomainID != "" {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			// Stop VM
			_ = svc.vmManager.StopVM(ctx, libvirtDomainID)
			// In real mode, would also snapshot disk and delete instance
		}()
	}

	c.Status(http.StatusAccepted)
}

// Unshelve restores a shelved instance
func (svc *Service) UnshelveInstance(c *gin.Context) {
	instanceID := c.Param("id")
	projectID := c.GetString("project_id")

	// Get instance
	var status string
	err := database.DB.QueryRow(c.Request.Context(),
		"SELECT status FROM instances WHERE id = $1 AND project_id = $2",
		instanceID, projectID,
	).Scan(&status)

	if err == pgx.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "instance not found"})
		return
	}

	// Can only unshelve a shelved instance
	if status != "SHELVED" {
		c.JSON(http.StatusConflict, gin.H{"error": fmt.Sprintf("cannot unshelve instance in %s state", status)})
		return
	}

	// Update status to ACTIVE
	_, err = database.DB.Exec(c.Request.Context(), `
		UPDATE instances
		SET status = $1, power_state = $2, task_state = $3, updated_at = $4
		WHERE id = $5
	`, "ACTIVE", 1, "", time.Now(), instanceID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// In real mode, would restore from snapshot and start VM
	// For stub mode, just update DB

	c.Status(http.StatusAccepted)
}

// Resize changes the flavor of an instance
func (svc *Service) ResizeInstance(c *gin.Context) {
	instanceID := c.Param("id")
	projectID := c.GetString("project_id")

	var req struct {
		Resize struct {
			FlavorRef string `json:"flavorRef" binding:"required"`
		} `json:"resize"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	svc.resizeInstance(c, instanceID, projectID, req.Resize.FlavorRef)
}

// ResizeInstanceAction handles resize from the action API
func (svc *Service) ResizeInstanceAction(c *gin.Context, resizeData interface{}) {
	instanceID := c.Param("id")
	projectID := c.GetString("project_id")

	// Parse the already-parsed action body
	resizeMap, ok := resizeData.(map[string]interface{})
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid resize request"})
		return
	}

	flavorRef, ok := resizeMap["flavorRef"].(string)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "flavorRef is required"})
		return
	}

	svc.resizeInstance(c, instanceID, projectID, flavorRef)
}

// resizeInstance is the shared resize logic
func (svc *Service) resizeInstance(c *gin.Context, instanceID, projectID, flavorRef string) {

	// Get instance
	var status, currentFlavorID string
	err := database.DB.QueryRow(c.Request.Context(),
		"SELECT status, flavor_id FROM instances WHERE id = $1 AND project_id = $2",
		instanceID, projectID,
	).Scan(&status, &currentFlavorID)

	if err == pgx.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "instance not found"})
		return
	}

	// Can only resize a stopped instance (for simplicity)
	if status != "SHUTOFF" && status != "ACTIVE" {
		c.JSON(http.StatusConflict, gin.H{"error": fmt.Sprintf("cannot resize instance in %s state", status)})
		return
	}

	// Verify new flavor exists
	var newFlavorID string
	err = database.DB.QueryRow(c.Request.Context(),
		"SELECT id FROM flavors WHERE id::text = $1 OR name = $1 LIMIT 1",
		flavorRef,
	).Scan(&newFlavorID)

	if err == pgx.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "flavor not found"})
		return
	}

	// Don't allow resizing to same flavor
	if newFlavorID == currentFlavorID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "new flavor is same as current flavor"})
		return
	}

	// Update instance with new flavor
	_, err = database.DB.Exec(c.Request.Context(), `
		UPDATE instances
		SET flavor_id = $1, status = $2, task_state = $3, updated_at = $4
		WHERE id = $5
	`, newFlavorID, "VERIFY_RESIZE", "resize_prep", time.Now(), instanceID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// In real mode, would rebuild VM with new flavor
	// For stub mode, auto-confirm after 5 seconds
	go func() {
		time.Sleep(5 * time.Second)
		database.DB.Exec(context.Background(),
			"UPDATE instances SET status = $1, task_state = $2 WHERE id = $3",
			"ACTIVE", "", instanceID)
	}()

	c.Status(http.StatusAccepted)
}

// ConfirmResize confirms a resize operation
func (svc *Service) ConfirmResizeInstance(c *gin.Context) {
	instanceID := c.Param("id")
	projectID := c.GetString("project_id")

	// Update status to ACTIVE
	result, err := database.DB.Exec(c.Request.Context(), `
		UPDATE instances
		SET status = $1, task_state = $2, updated_at = $3
		WHERE id = $4 AND project_id = $5 AND status = $6
	`, "ACTIVE", "", time.Now(), instanceID, projectID, "VERIFY_RESIZE")

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	rowsAffected := result.RowsAffected()
	if rowsAffected == 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "instance not in resize-verify state"})
		return
	}

	c.Status(http.StatusNoContent)
}

// RevertResize reverts a resize operation
func (svc *Service) RevertResizeInstance(c *gin.Context) {
	instanceID := c.Param("id")
	projectID := c.GetString("project_id")

	// Get instance
	var status string
	err := database.DB.QueryRow(c.Request.Context(),
		"SELECT status FROM instances WHERE id = $1 AND project_id = $2",
		instanceID, projectID,
	).Scan(&status)

	if err == pgx.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "instance not found"})
		return
	}

	if status != "VERIFY_RESIZE" {
		c.JSON(http.StatusConflict, gin.H{"error": "instance not in resize-verify state"})
		return
	}

	// In real mode, would revert to old flavor
	// For stub mode, just set back to ACTIVE
	_, err = database.DB.Exec(c.Request.Context(), `
		UPDATE instances
		SET status = $1, task_state = $2, updated_at = $3
		WHERE id = $4
	`, "ACTIVE", "", time.Now(), instanceID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.Status(http.StatusAccepted)
}

// EvacuateInstance handles evacuating an instance from a failed host
func (svc *Service) EvacuateInstance(c *gin.Context) {
	instanceID := c.Param("id")
	projectID := c.GetString("project_id")

	var status string
	err := database.DB.QueryRow(c.Request.Context(),
		"SELECT status FROM instances WHERE id = $1 AND project_id = $2",
		instanceID, projectID,
	).Scan(&status)

	if err == pgx.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "instance not found"})
		return
	}

	_, err = database.DB.Exec(c.Request.Context(), `
		UPDATE instances SET status = $1, task_state = $2, updated_at = $3
		WHERE id = $4
	`, "ACTIVE", "evacuating", time.Now(), instanceID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.Status(http.StatusOK)
}

// MigrateInstance handles cold migration
func (svc *Service) MigrateInstance(c *gin.Context) {
	instanceID := c.Param("id")
	projectID := c.GetString("project_id")

	var status string
	err := database.DB.QueryRow(c.Request.Context(),
		"SELECT status FROM instances WHERE id = $1 AND project_id = $2",
		instanceID, projectID,
	).Scan(&status)

	if err == pgx.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "instance not found"})
		return
	}

	if status != "ACTIVE" && status != "SHUTOFF" {
		c.JSON(http.StatusConflict, gin.H{"error": fmt.Sprintf("cannot migrate instance in %s state", status)})
		return
	}

	_, err = database.DB.Exec(c.Request.Context(), `
		UPDATE instances SET status = $1, task_state = $2, updated_at = $3
		WHERE id = $4
	`, "ACTIVE", "migrating", time.Now(), instanceID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.Status(http.StatusAccepted)
}

// LiveMigrateInstance handles live migration
func (svc *Service) LiveMigrateInstance(c *gin.Context) {
	instanceID := c.Param("id")
	projectID := c.GetString("project_id")

	// Get action data from context (already parsed by ServerAction)
	actionData, exists := c.Get("action_data")
	if !exists {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing os-migrateLive data"})
		return
	}

	// Type assert to map (from JSON interface{})
	liveMigrateMap, ok := actionData.(map[string]interface{})
	if !ok {
		// Action data might be nil or empty object, which is fine
		liveMigrateMap = make(map[string]interface{})
	}

	// Extract optional host and block_migration (not used in stub mode but parsed for API compatibility)
	var host *string
	var blockMigration bool
	if hostVal, ok := liveMigrateMap["host"].(string); ok && hostVal != "" {
		host = &hostVal
	}
	if bmVal, ok := liveMigrateMap["block_migration"].(bool); ok {
		blockMigration = bmVal
	}
	_ = host
	_ = blockMigration

	var status string
	err := database.DB.QueryRow(c.Request.Context(),
		"SELECT status FROM instances WHERE id = $1 AND project_id = $2",
		instanceID, projectID,
	).Scan(&status)

	if err == pgx.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "instance not found"})
		return
	}

	if status != "ACTIVE" {
		c.JSON(http.StatusConflict, gin.H{"error": "instance must be active"})
		return
	}

	_, err = database.DB.Exec(c.Request.Context(), `
		UPDATE instances SET task_state = $1, updated_at = $2
		WHERE id = $3
	`, "migrating", time.Now(), instanceID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.Status(http.StatusAccepted)
}
