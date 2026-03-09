package glance_test

import (
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack"
	"github.com/gophercloud/gophercloud/openstack/imageservice/v2/images"
	"github.com/gophercloud/gophercloud/openstack/imageservice/v2/members"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper functions for Glance contract tests
func setupGlanceClient(t *testing.T) *gophercloud.ServiceClient {
	t.Helper()

	authURL := getEnvOrDefault("OS_AUTH_URL", "http://localhost:5001/v3")
	username := getEnvOrDefault("OS_USERNAME", "admin")
	password := getEnvOrDefault("OS_PASSWORD", "secret")
	projectName := getEnvOrDefault("OS_PROJECT_NAME", "default")
	domainName := getEnvOrDefault("OS_USER_DOMAIN_NAME", "Default")

	opts := gophercloud.AuthOptions{
		IdentityEndpoint: authURL,
		Username:         username,
		Password:         password,
		TenantName:       projectName,
		DomainName:       domainName,
	}

	provider, err := openstack.AuthenticatedClient(opts)
	require.NoError(t, err, "Failed to create authenticated client")

	client, err := openstack.NewImageServiceV2(provider, gophercloud.EndpointOpts{})
	require.NoError(t, err, "Failed to create Glance client")

	return client
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func skipIfO3KNotRunning(t *testing.T) {
	t.Helper()
	if os.Getenv("SKIP_CONTRACT_TESTS") == "1" {
		t.Skip("Contract tests skipped (SKIP_CONTRACT_TESTS=1)")
	}
}

// TestGlanceImageMemberCreate_Contract tests POST /v2/images/{image_id}/members
func TestGlanceImageMemberCreate_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupGlanceClient(t)

	// Setup: Create a test image first
	imageName := "test-image-members-" + uuid.New().String()[:8]
	createOpts := images.CreateOpts{
		Name:            imageName,
		ContainerFormat: "bare",
		DiskFormat:      "qcow2",
		Visibility:      func() *images.ImageVisibility { v := images.ImageVisibilityPrivate; return &v }(), // Must be private for sharing
	}

	image, err := images.Create(client, createOpts).Extract()
	require.NoError(t, err, "Setup: CreateImage should succeed")
	defer images.Delete(client, image.ID)

	// Test: Add image member (share with another project)
	memberProjectID := "00000000-0000-0000-0000-000000000003" // Dummy project ID

	member, err := members.Create(client, image.ID, memberProjectID).Extract()

	// Assertions
	require.NoError(t, err, "CreateImageMember should succeed")
	assert.NotEmpty(t, member.MemberID, "Member ID should be set")
	assert.Equal(t, memberProjectID, member.MemberID, "Member ID should match")
	assert.Equal(t, image.ID, member.ImageID, "Image ID should match")
	assert.Equal(t, "pending", member.Status, "Initial status should be pending")
}

// TestGlanceImageMemberList_Contract tests GET /v2/images/{image_id}/members
func TestGlanceImageMemberList_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupGlanceClient(t)

	// Setup: Create image and add member
	imageName := "test-image-members-list-" + uuid.New().String()[:8]
	image, err := images.Create(client, images.CreateOpts{
		Name:            imageName,
		ContainerFormat: "bare",
		DiskFormat:      "qcow2",
		Visibility:      func() *images.ImageVisibility { v := images.ImageVisibilityPrivate; return &v }(),
	}).Extract()
	require.NoError(t, err, "Setup: CreateImage should succeed")
	defer images.Delete(client, image.ID)

	memberProjectID := "00000000-0000-0000-0000-000000000003"
	_, err = members.Create(client, image.ID, memberProjectID).Extract()
	require.NoError(t, err, "Setup: CreateMember should succeed")

	// Test: List image members
	allPages, err := members.List(client, image.ID).AllPages()
	require.NoError(t, err, "ListImageMembers should succeed")

	memberList, err := members.ExtractMembers(allPages)
	require.NoError(t, err, "ExtractMembers should succeed")

	// Assertions
	assert.NotEmpty(t, memberList, "Should have at least one member")
	assert.Equal(t, memberProjectID, memberList[0].MemberID, "Member ID should match")
}

// TestGlanceImageMemberGet_Contract tests GET /v2/images/{image_id}/members/{member_id}
func TestGlanceImageMemberGet_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupGlanceClient(t)

	// Setup: Create image and add member
	imageName := "test-image-member-get-" + uuid.New().String()[:8]
	image, err := images.Create(client, images.CreateOpts{
		Name:            imageName,
		ContainerFormat: "bare",
		DiskFormat:      "qcow2",
		Visibility:      func() *images.ImageVisibility { v := images.ImageVisibilityPrivate; return &v }(),
	}).Extract()
	require.NoError(t, err, "Setup: CreateImage should succeed")
	defer images.Delete(client, image.ID)

	memberProjectID := "00000000-0000-0000-0000-000000000003"
	createdMember, err := members.Create(client, image.ID, memberProjectID).Extract()
	require.NoError(t, err, "Setup: CreateMember should succeed")

	// Test: Get specific image member
	member, err := members.Get(client, image.ID, memberProjectID).Extract()

	// Assertions
	require.NoError(t, err, "GetImageMember should succeed")
	assert.Equal(t, createdMember.MemberID, member.MemberID, "Member ID should match")
	assert.Equal(t, image.ID, member.ImageID, "Image ID should match")
	assert.Equal(t, "pending", member.Status, "Status should be pending")
}

// TestGlanceImageMemberUpdate_Contract tests PUT /v2/images/{image_id}/members/{member_id}
func TestGlanceImageMemberUpdate_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupGlanceClient(t)

	// Setup: Create image and add member
	imageName := "test-image-member-update-" + uuid.New().String()[:8]
	image, err := images.Create(client, images.CreateOpts{
		Name:            imageName,
		ContainerFormat: "bare",
		DiskFormat:      "qcow2",
		Visibility:      func() *images.ImageVisibility { v := images.ImageVisibilityPrivate; return &v }(),
	}).Extract()
	require.NoError(t, err, "Setup: CreateImage should succeed")
	defer images.Delete(client, image.ID)

	memberProjectID := "00000000-0000-0000-0000-000000000003"
	_, err = members.Create(client, image.ID, memberProjectID).Extract()
	require.NoError(t, err, "Setup: CreateMember should succeed")

	// Test: Update member status to accepted
	// NOTE: In real OpenStack, only the member can accept/reject, but owner can set to pending
	// For testing purposes, we'll just update to pending as the owner
	updateOpts := members.UpdateOpts{
		Status: "pending",
	}

	member, err := members.Update(client, image.ID, memberProjectID, updateOpts).Extract()

	// Assertions
	require.NoError(t, err, "UpdateImageMember should succeed")
	assert.Equal(t, "pending", member.Status, "Status should remain pending")
}

// TestGlanceImageMemberDelete_Contract tests DELETE /v2/images/{image_id}/members/{member_id}
func TestGlanceImageMemberDelete_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupGlanceClient(t)

	// Setup: Create image and add member
	imageName := "test-image-member-delete-" + uuid.New().String()[:8]
	image, err := images.Create(client, images.CreateOpts{
		Name:            imageName,
		ContainerFormat: "bare",
		DiskFormat:      "qcow2",
		Visibility:      func() *images.ImageVisibility { v := images.ImageVisibilityPrivate; return &v }(),
	}).Extract()
	require.NoError(t, err, "Setup: CreateImage should succeed")
	defer images.Delete(client, image.ID)

	memberProjectID := "00000000-0000-0000-0000-000000000003"
	_, err = members.Create(client, image.ID, memberProjectID).Extract()
	require.NoError(t, err, "Setup: CreateMember should succeed")

	// Test: Delete image member
	err = members.Delete(client, image.ID, memberProjectID).ExtractErr()

	// Assertions
	require.NoError(t, err, "DeleteImageMember should succeed")

	// Verify deletion: List should be empty
	allPages, err := members.List(client, image.ID).AllPages()
	require.NoError(t, err, "List after delete should succeed")

	memberList, err := members.ExtractMembers(allPages)
	require.NoError(t, err, "ExtractMembers should succeed")
	assert.Empty(t, memberList, "Member list should be empty after deletion")
}
