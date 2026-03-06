package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/cobaltcore-dev/o3k/internal/keystone"
)

func TestAuthMiddlewareNoToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	authService := keystone.NewAuthService("test-secret", 24*time.Hour)
	router.Use(AuthMiddleware(authService))
	router.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401, got %d", w.Code)
	}
}

func TestAuthMiddlewareValidToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	secret := "test-secret"
	userID := "user-123"
	userName := "testuser"
	projectID := "project-456"
	roles := []string{"admin"}

	authService := keystone.NewAuthService(secret, 24*time.Hour)

	// Generate valid token
	now := time.Now()
	claims := &keystone.TokenClaims{
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

	router.Use(AuthMiddleware(authService))
	router.GET("/test", func(c *gin.Context) {
		// Verify context values are set
		contextUserID := c.GetString("user_id")
		contextProjectID := c.GetString("project_id")

		if contextUserID != userID {
			t.Errorf("Expected user_id %s in context, got %s", userID, contextUserID)
		}

		if contextProjectID != projectID {
			t.Errorf("Expected project_id %s in context, got %s", projectID, contextProjectID)
		}

		c.JSON(200, gin.H{"status": "ok"})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Auth-Token", tokenString)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

func TestAuthMiddlewareInvalidToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	authService := keystone.NewAuthService("test-secret", 24*time.Hour)
	router.Use(AuthMiddleware(authService))
	router.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Auth-Token", "invalid-token")
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401, got %d", w.Code)
	}
}

func TestAuthMiddlewareExpiredToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	secret := "test-secret"
	userID := "user-123"
	userName := "testuser"
	projectID := "project-456"
	roles := []string{"admin"}

	authService := keystone.NewAuthService(secret, 24*time.Hour)

	// Generate expired token
	now := time.Now()
	claims := &keystone.TokenClaims{
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

	router.Use(AuthMiddleware(authService))
	router.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Auth-Token", tokenString)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401, got %d", w.Code)
	}
}
