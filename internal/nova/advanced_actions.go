package nova

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
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

	// Admin-only operation - check roles
	roles := c.GetStringSlice("roles")
	isAdmin := false
	for _, role := range roles {
		if role == "admin" {
			isAdmin = true
			break
		}
	}
	if !isAdmin {
		c.JSON(http.StatusForbidden, gin.H{
			"error": gin.H{
				"code":    403,
				"message": "Policy doesn't allow evacuate to be performed",
			},
		})
		return
	}

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

	// In stub mode, simulate hosts (compute-1, compute-2)
	// In real mode, would query compute_nodes table for available hosts
	currentHost := "compute-1"
	destHost := "compute-2"

	// Create migration record
	migrationID := uuid.New().String()
	_, err = database.DB.Exec(c.Request.Context(), `
		INSERT INTO server_migrations (id, server_uuid, source_node, dest_node, migration_type, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, 'migration', 'running', $5, $5)
	`, migrationID, instanceID, currentHost, destHost, time.Now())

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to create migration: %v", err)})
		return
	}

	// Update instance to set migrating state
	_, err = database.DB.Exec(c.Request.Context(), `
		UPDATE instances SET task_state = 'migrating', updated_at = $1
		WHERE id = $2
	`, time.Now(), instanceID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Background goroutine to complete migration after 5 seconds (stub mode)
	go func() {
		time.Sleep(5 * time.Second)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// Clear task_state and mark migration as complete
		database.DB.Exec(ctx, `
			UPDATE instances SET task_state = NULL, updated_at = $1
			WHERE id = $2
		`, time.Now(), instanceID)

		database.DB.Exec(ctx, `
			UPDATE server_migrations SET status = 'completed', updated_at = $1
			WHERE id = $2
		`, time.Now(), migrationID)
	}()

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

// AddSecurityGroup adds a security group to an instance
func (svc *Service) AddSecurityGroup(c *gin.Context) {
	instanceID := c.Param("id")
	projectID := c.GetString("project_id")

	// Get action data from context (already parsed by ServerAction)
	actionData, exists := c.Get("action_data")
	if !exists {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing addSecurityGroup data"})
		return
	}

	addSGMap, ok := actionData.(map[string]interface{})
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid addSecurityGroup data"})
		return
	}

	sgName, ok := addSGMap["name"].(string)
	if !ok || sgName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing security group name"})
		return
	}

	// Verify instance exists
	var exists_check bool
	err := database.DB.QueryRow(c.Request.Context(),
		"SELECT EXISTS(SELECT 1 FROM instances WHERE id = $1 AND project_id = $2)",
		instanceID, projectID,
	).Scan(&exists_check)

	if err != nil || !exists_check {
		c.JSON(http.StatusNotFound, gin.H{"error": "instance not found"})
		return
	}

	// Verify security group exists
	var sgID string
	err = database.DB.QueryRow(c.Request.Context(),
		"SELECT id FROM security_groups WHERE name = $1 AND project_id = $2",
		sgName, projectID,
	).Scan(&sgID)

	if err == pgx.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "security group not found"})
		return
	}

	// Check if security group is already associated with instance
	var alreadyAssociated bool
	err = database.DB.QueryRow(c.Request.Context(),
		"SELECT EXISTS(SELECT 1 FROM server_security_groups WHERE server_id = $1 AND security_group_id = $2)",
		instanceID, sgID,
	).Scan(&alreadyAssociated)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if alreadyAssociated {
		c.JSON(http.StatusConflict, gin.H{"error": "security group already associated with instance"})
		return
	}

	// Add security group association
	_, err = database.DB.Exec(c.Request.Context(), `
		INSERT INTO server_security_groups (server_id, security_group_id, created_at)
		VALUES ($1, $2, $3)
	`, instanceID, sgID, time.Now())

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// In stub mode, just return success
	// In real mode, would apply iptables rules and update Neutron port security groups
	c.Status(http.StatusAccepted)
}

// RemoveSecurityGroup removes a security group from an instance
func (svc *Service) RemoveSecurityGroup(c *gin.Context) {
	instanceID := c.Param("id")
	projectID := c.GetString("project_id")

	// Get action data from context
	actionData, exists := c.Get("action_data")
	if !exists {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing removeSecurityGroup data"})
		return
	}

	removeSGMap, ok := actionData.(map[string]interface{})
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid removeSecurityGroup data"})
		return
	}

	sgName, ok := removeSGMap["name"].(string)
	if !ok || sgName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing security group name"})
		return
	}

	// Verify instance exists
	var exists_check bool
	err := database.DB.QueryRow(c.Request.Context(),
		"SELECT EXISTS(SELECT 1 FROM instances WHERE id = $1 AND project_id = $2)",
		instanceID, projectID,
	).Scan(&exists_check)

	if err != nil || !exists_check {
		c.JSON(http.StatusNotFound, gin.H{"error": "instance not found"})
		return
	}

	// Verify security group exists
	var sgID string
	err = database.DB.QueryRow(c.Request.Context(),
		"SELECT id FROM security_groups WHERE name = $1 AND project_id = $2",
		sgName, projectID,
	).Scan(&sgID)

	if err == pgx.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "security group not found"})
		return
	}

	// Check if security group is actually associated with instance
	var isAssociated bool
	err = database.DB.QueryRow(c.Request.Context(),
		"SELECT EXISTS(SELECT 1 FROM server_security_groups WHERE server_id = $1 AND security_group_id = $2)",
		instanceID, sgID,
	).Scan(&isAssociated)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if !isAssociated {
		c.JSON(http.StatusNotFound, gin.H{"error": "security group not associated with instance"})
		return
	}

	// Check if this is the last security group (cannot remove last one)
	var sgCount int
	err = database.DB.QueryRow(c.Request.Context(),
		"SELECT COUNT(*) FROM server_security_groups WHERE server_id = $1",
		instanceID,
	).Scan(&sgCount)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if sgCount <= 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot remove last security group from instance"})
		return
	}

	// Remove security group association
	_, err = database.DB.Exec(c.Request.Context(), `
		DELETE FROM server_security_groups
		WHERE server_id = $1 AND security_group_id = $2
	`, instanceID, sgID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// In stub mode, just return success
	// In real mode, would remove iptables rules and update Neutron port security groups
	c.Status(http.StatusAccepted)
}

// ChangePassword changes the admin password for an instance
func (svc *Service) ChangePassword(c *gin.Context) {
	instanceID := c.Param("id")
	projectID := c.GetString("project_id")

	// Get action data from context
	actionData, exists := c.Get("action_data")
	if !exists {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing changePassword data"})
		return
	}

	changePassMap, ok := actionData.(map[string]interface{})
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid changePassword data"})
		return
	}

	adminPass, ok := changePassMap["adminPass"].(string)
	if !ok || adminPass == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing adminPass"})
		return
	}

	// Verify instance exists
	var status string
	err := database.DB.QueryRow(c.Request.Context(),
		"SELECT status FROM instances WHERE id = $1 AND project_id = $2",
		instanceID, projectID,
	).Scan(&status)

	if err == pgx.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "instance not found"})
		return
	}

	// Can only change password on ACTIVE instances
	if status != "ACTIVE" {
		c.JSON(http.StatusConflict, gin.H{"error": fmt.Sprintf("cannot change password for instance in %s state", status)})
		return
	}

	// Validate password strength (minimum 8 characters)
	if len(adminPass) < 8 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "password must be at least 8 characters"})
		return
	}

	// Hash password using bcrypt
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(adminPass), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to hash password"})
		return
	}

	// Update admin password hash in database
	_, err = database.DB.Exec(c.Request.Context(), `
		UPDATE instances
		SET admin_password_hash = $1, updated_at = $2
		WHERE id = $3
	`, string(hashedPassword), time.Now(), instanceID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// In stub mode, just return success
	// In real mode, would use libvirt guest agent or cloud-init to inject password
	c.Status(http.StatusAccepted)
}

// RestoreInstance restores a soft-deleted instance
func (svc *Service) RestoreInstance(c *gin.Context) {
	instanceID := c.Param("id")
	projectID := c.GetString("project_id")

	// Check if instance exists and is soft-deleted
	var status string
	err := database.DB.QueryRow(c.Request.Context(),
		"SELECT status FROM instances WHERE id = $1 AND project_id = $2",
		instanceID, projectID,
	).Scan(&status)

	if err == pgx.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "instance not found"})
		return
	}

	if status != "SOFT_DELETED" {
		c.JSON(http.StatusConflict, gin.H{"error": "instance is not soft-deleted"})
		return
	}

	// Restore instance to SHUTOFF state
	_, err = database.DB.Exec(c.Request.Context(), `
		UPDATE instances
		SET status = $1, power_state = $2, updated_at = $3
		WHERE id = $4
	`, "SHUTOFF", 4, time.Now(), instanceID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.Status(http.StatusAccepted)
}

// CreateBackupAction creates a backup image of an instance
func (svc *Service) CreateBackupAction(c *gin.Context) {
	instanceID := c.Param("id")
	projectID := c.GetString("project_id")

	// Get action data from context
	actionData, exists := c.Get("action_data")
	if !exists {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing createBackup data"})
		return
	}

	backupMap, ok := actionData.(map[string]interface{})
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid createBackup data"})
		return
	}

	backupName, ok := backupMap["name"].(string)
	if !ok || backupName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing backup name"})
		return
	}

	backupType, _ := backupMap["backup_type"].(string)
	rotation, _ := backupMap["rotation"].(float64)

	if backupType == "" {
		backupType = "daily"
	}
	if rotation == 0 {
		rotation = 7 // default rotation
	}

	// Verify instance exists
	var sourceImageID string
	err := database.DB.QueryRow(c.Request.Context(),
		"SELECT image_id FROM instances WHERE id = $1 AND project_id = $2",
		instanceID, projectID,
	).Scan(&sourceImageID)

	if err == pgx.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "instance not found"})
		return
	}

	// Generate new UUID for backup image
	backupImageID := uuid.New().String()

	// Create backup image in images table
	// Image name pattern: {backup_name}-{backup_type}-{timestamp}
	timestamp := time.Now().Format("20060102-150405")
	fullImageName := fmt.Sprintf("%s-%s-%s", backupName, backupType, timestamp)

	_, err = database.DB.Exec(c.Request.Context(), `
		INSERT INTO images (id, name, project_id, status, visibility, disk_format, container_format, created_at, updated_at)
		VALUES ($1, $2, $3, 'active', 'private', 'qcow2', 'bare', $4, $4)
	`, backupImageID, fullImageName, projectID, time.Now())

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to create backup image: %v", err)})
		return
	}

	// Add tags to identify this as a backup
	database.DB.Exec(c.Request.Context(), `
		INSERT INTO image_tags (image_id, tag) VALUES ($1, 'backup'), ($1, $2), ($1, $3)
	`, backupImageID, fmt.Sprintf("backup_type:%s", backupType), fmt.Sprintf("source_server:%s", instanceID))

	// Implement rotation: delete old backups of same type for this server
	// Query all backup images for this server with same backup_type
	rows, err := database.DB.Query(c.Request.Context(), `
		SELECT DISTINCT i.id, i.created_at
		FROM images i
		JOIN image_tags t1 ON i.id = t1.image_id AND t1.tag = 'backup'
		JOIN image_tags t2 ON i.id = t2.image_id AND t2.tag = $1
		JOIN image_tags t3 ON i.id = t3.image_id AND t3.tag = $2
		WHERE i.project_id = $3
		ORDER BY i.created_at DESC
	`, fmt.Sprintf("backup_type:%s", backupType), fmt.Sprintf("source_server:%s", instanceID), projectID)

	if err == nil {
		defer rows.Close()

		var backupIDs []string
		for rows.Next() {
			var id string
			var createdAt time.Time
			rows.Scan(&id, &createdAt)
			backupIDs = append(backupIDs, id)
		}

		// Delete backups beyond rotation limit
		if len(backupIDs) > int(rotation) {
			oldBackups := backupIDs[int(rotation):]
			for _, oldID := range oldBackups {
				database.DB.Exec(c.Request.Context(), "DELETE FROM images WHERE id = $1", oldID)
			}
		}
	}

	// Return image location in header (OpenStack pattern)
	c.Header("Location", fmt.Sprintf("/v2/images/%s", backupImageID))

	// Microversion 2.45+ returns image_id in response body
	c.JSON(http.StatusAccepted, gin.H{
		"image_id": backupImageID,
	})
}

// ResetStateAction resets instance state (admin operation)
func (svc *Service) ResetStateAction(c *gin.Context) {
	instanceID := c.Param("id")
	projectID := c.GetString("project_id")

	// Admin-only operation - check roles
	roles := c.GetStringSlice("roles")
	isAdmin := false
	for _, role := range roles {
		if role == "admin" {
			isAdmin = true
			break
		}
	}
	if !isAdmin {
		c.JSON(http.StatusForbidden, gin.H{
			"forbidden": gin.H{
				"message": "Policy doesn't allow os-resetState to be performed",
				"code":    403,
			},
		})
		return
	}

	// Get action data from context
	actionData, exists := c.Get("action_data")
	if !exists {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing os-resetState data"})
		return
	}

	resetMap, ok := actionData.(map[string]interface{})
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid os-resetState data"})
		return
	}

	state, ok := resetMap["state"].(string)
	if !ok || state == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing state"})
		return
	}

	// Verify instance exists
	var exists_check bool
	err := database.DB.QueryRow(c.Request.Context(),
		"SELECT EXISTS(SELECT 1 FROM instances WHERE id = $1 AND project_id = $2)",
		instanceID, projectID,
	).Scan(&exists_check)

	if err != nil || !exists_check {
		c.JSON(http.StatusNotFound, gin.H{"error": "instance not found"})
		return
	}

	// Update instance state (convert lowercase to uppercase)
	statusUpper := fmt.Sprintf("%s", state)
	if state == "error" {
		statusUpper = "ERROR"
	} else if state == "active" {
		statusUpper = "ACTIVE"
	}

	_, err = database.DB.Exec(c.Request.Context(), `
		UPDATE instances
		SET status = $1, updated_at = $2
		WHERE id = $3
	`, statusUpper, time.Now(), instanceID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.Status(http.StatusAccepted)
}

// ResetNetworkAction resets instance network
func (svc *Service) ResetNetworkAction(c *gin.Context) {
	instanceID := c.Param("id")
	projectID := c.GetString("project_id")

	// Verify instance exists
	var exists_check bool
	err := database.DB.QueryRow(c.Request.Context(),
		"SELECT EXISTS(SELECT 1 FROM instances WHERE id = $1 AND project_id = $2)",
		instanceID, projectID,
	).Scan(&exists_check)

	if err != nil || !exists_check {
		c.JSON(http.StatusNotFound, gin.H{"error": "instance not found"})
		return
	}

	// In stub mode, just return success
	// In real mode, would reset network interfaces via libvirt/netlink
	c.Status(http.StatusAccepted)
}
