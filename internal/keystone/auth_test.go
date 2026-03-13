package keystone

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestGenerateAndValidateToken(t *testing.T) {
	secret := "test-secret"
	authService := NewAuthService(secret, 24*time.Hour)

	userID := "test-user-id"
	userName := "test-user"
	projectID := "test-project-id"
	roles := []string{"admin", "member"}

	// Generate token manually
	now := time.Now()
	claims := &TokenClaims{
		UserID:    userID,
		UserName:  userName,
		ProjectID: projectID,
		Roles:     roles,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(24 * time.Hour)),
			Subject:   userID,
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	if tokenString == "" {
		t.Fatal("Token string is empty")
	}

	// Validate token
	validatedClaims, err := authService.ValidateToken(tokenString)
	if err != nil {
		t.Fatalf("Failed to validate token: %v", err)
	}

	if validatedClaims.UserID != userID {
		t.Errorf("Expected user_id %s, got %s", userID, validatedClaims.UserID)
	}

	if validatedClaims.ProjectID != projectID {
		t.Errorf("Expected project_id %s, got %s", projectID, validatedClaims.ProjectID)
	}
}

func TestValidateTokenExpired(t *testing.T) {
	secret := "test-secret"
	authService := NewAuthService(secret, 24*time.Hour)

	userID := "test-user-id"
	userName := "test-user"
	projectID := "test-project-id"
	roles := []string{"admin"}

	// Generate expired token
	now := time.Now()
	claims := &TokenClaims{
		UserID:    userID,
		UserName:  userName,
		ProjectID: projectID,
		Roles:     roles,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now.Add(-25 * time.Hour)),
			ExpiresAt: jwt.NewNumericDate(now.Add(-1 * time.Hour)),
			Subject:   userID,
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	// Validate token - should fail
	_, err = authService.ValidateToken(tokenString)
	if err == nil {
		t.Fatal("Expected error for expired token, got nil")
	}
}

func TestValidateTokenInvalidSecret(t *testing.T) {
	secret := "test-secret"
	wrongSecret := "wrong-secret"
	wrongAuthService := NewAuthService(wrongSecret, 24*time.Hour)

	userID := "test-user-id"
	userName := "test-user"
	projectID := "test-project-id"
	roles := []string{"admin"}

	// Generate token with one secret
	now := time.Now()
	claims := &TokenClaims{
		UserID:    userID,
		UserName:  userName,
		ProjectID: projectID,
		Roles:     roles,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(24 * time.Hour)),
			Subject:   userID,
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	// Validate with different secret - should fail
	_, err = wrongAuthService.ValidateToken(tokenString)
	if err == nil {
		t.Fatal("Expected error for invalid secret, got nil")
	}
}

func TestBuildServiceCatalog(t *testing.T) {
	projectID := "test-project-id"
	catalog := BuildServiceCatalog(projectID)

	if len(catalog) != 5 {
		t.Errorf("Expected 5 services in catalog, got %d", len(catalog))
	}

	// Check identity service
	identityFound := false
	for _, service := range catalog {
		if service.Type == "identity" {
			identityFound = true
			if len(service.Endpoints) == 0 {
				t.Error("Identity service missing endpoints")
			}
		}
	}

	if !identityFound {
		t.Error("Identity service not found in catalog")
	}
}

func TestSubstituteURLTemplates(t *testing.T) {
	projectID := "abc123-def456"

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "braces format",
			input:    "http://localhost:8776/v3/{project_id}/volumes",
			expected: "http://localhost:8776/v3/abc123-def456/volumes",
		},
		{
			name:     "dollar format",
			input:    "http://localhost:8776/v3/$(project_id)s/volumes",
			expected: "http://localhost:8776/v3/abc123-def456/volumes",
		},
		{
			name:     "percent format",
			input:    "http://localhost:8776/v3/%(project_id)s/volumes",
			expected: "http://localhost:8776/v3/abc123-def456/volumes",
		},
		{
			name:     "multiple occurrences",
			input:    "http://localhost:8776/v3/{project_id}/volumes/{project_id}",
			expected: "http://localhost:8776/v3/abc123-def456/volumes/abc123-def456",
		},
		{
			name:     "no placeholder",
			input:    "http://localhost:9292/v2/images",
			expected: "http://localhost:9292/v2/images",
		},
		{
			name:     "mixed placeholders",
			input:    "http://localhost:8776/v3/{project_id}/volumes/%(project_id)s",
			expected: "http://localhost:8776/v3/abc123-def456/volumes/abc123-def456",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := substituteURLTemplates(tt.input, projectID)
			if result != tt.expected {
				t.Errorf("substituteURLTemplates(%q, %q) = %q; want %q",
					tt.input, projectID, result, tt.expected)
			}
		})
	}
}
