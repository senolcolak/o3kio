package nova

import (
	"crypto/md5"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog/log"
	"golang.org/x/crypto/ssh"

	"github.com/cobaltcore-dev/o3k/internal/common"
)

// CreateKeypairRequest represents a keypair creation request
type CreateKeypairRequest struct {
	Keypair struct {
		Name      string `json:"name" binding:"required"`
		PublicKey string `json:"public_key"` // Optional - generate if not provided
		Type      string `json:"type"`       // ssh or x509 (default: ssh)
	} `json:"keypair"`
}

// ListKeypairs lists all keypairs for the user
func (svc *Service) ListKeypairs(c *gin.Context) {
	userID := c.GetString("user_id")

	rows, err := svc.activeDB().Query(c.Request.Context(), `
		SELECT id, name, user_id, public_key, fingerprint, created_at
		FROM keypairs
		WHERE user_id = $1
		ORDER BY created_at DESC
	`, userID)

	if err != nil {
		log.Error().Err(err).Str("operation", "list_keypairs").Msg("database error")
		common.SendError(c, common.NewInternalServerError("failed to list keypairs"))
		return
	}
	defer rows.Close()

	var keypairs []gin.H
	for rows.Next() {
		var id, name, uid, publicKey, fingerprint string
		var createdAt time.Time

		if err := rows.Scan(&id, &name, &uid, &publicKey, &fingerprint, &createdAt); err != nil {
			continue
		}

		// OpenStack format wraps each keypair in a "keypair" object
		keypairs = append(keypairs, gin.H{
			"keypair": gin.H{
				"name":        name,
				"public_key":  publicKey,
				"fingerprint": fingerprint,
			},
		})
	}
	if err := rows.Err(); err != nil {
		log.Error().Err(err).Str("operation", "list_keypairs").Msg("rows iteration error")
		common.SendError(c, common.NewInternalServerError("failed to list keypairs"))
		return
	}

	if keypairs == nil {
		keypairs = []gin.H{}
	}

	c.JSON(http.StatusOK, gin.H{"keypairs": keypairs})
}

// GetKeypair retrieves a single keypair by name
func (svc *Service) GetKeypair(c *gin.Context) {
	keypairName := c.Param("id") // In Nova API, it's the name, not UUID
	userID := c.GetString("user_id")

	var name, publicKey, fingerprint string
	var createdAt time.Time

	err := svc.activeDB().QueryRow(c.Request.Context(), `
		SELECT name, public_key, fingerprint, created_at
		FROM keypairs
		WHERE user_id = $1 AND name = $2
	`, userID, keypairName).Scan(&name, &publicKey, &fingerprint, &createdAt)

	if errors.Is(err, pgx.ErrNoRows) {
		common.SendError(c, common.NewNotFoundError("keypair"))
		return
	}
	if err != nil {
		log.Error().Err(err).Str("operation", "get_keypair").Msg("database error")
		common.SendError(c, common.NewInternalServerError("failed to get keypair"))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"keypair": gin.H{
			"name":        name,
			"public_key":  publicKey,
			"fingerprint": fingerprint,
			"created_at":  createdAt.Format(time.RFC3339),
			"user_id":     userID,
		},
	})
}

// CreateKeypair creates a new SSH keypair
func (svc *Service) CreateKeypair(c *gin.Context) {
	var req CreateKeypairRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.SendError(c, common.NewBadRequestError("invalid request body"))
		return
	}

	userID := c.GetString("user_id")
	var publicKey string
	var privateKey *string
	var fingerprint string

	if req.Keypair.PublicKey != "" {
		// Import existing public key
		publicKey = req.Keypair.PublicKey

		// Calculate fingerprint
		fp, err := calculateFingerprint(publicKey)
		if err != nil {
			common.SendError(c, common.NewBadRequestError(fmt.Sprintf("invalid public key: %v", err)))
			return
		}
		fingerprint = fp
	} else {
		// Generate new keypair
		pub, priv, fp, err := generateSSHKeyPair()
		if err != nil {
			log.Error().Err(err).Str("operation", "generate_keypair").Msg("keypair generation error")
			common.SendError(c, common.NewInternalServerError("failed to generate keypair"))
			return
		}
		publicKey = pub
		privateKey = &priv
		fingerprint = fp
	}

	// Check if keypair with same name already exists
	var existingID string
	err := svc.activeDB().QueryRow(c.Request.Context(),
		"SELECT id FROM keypairs WHERE user_id = $1 AND name = $2",
		userID, req.Keypair.Name,
	).Scan(&existingID)

	if err == nil {
		// Keypair already exists
		common.SendError(c, common.NewConflictError("Keypair with this name already exists"))
		return
	}

	// Insert into database
	now := time.Now()
	_, err = svc.activeDB().Exec(c.Request.Context(), `
		INSERT INTO keypairs (user_id, name, public_key, fingerprint, created_at)
		VALUES ($1, $2, $3, $4, $5)
	`, userID, req.Keypair.Name, publicKey, fingerprint, now)

	if err != nil {
		log.Error().Err(err).Str("operation", "create_keypair").Msg("database error")
		common.SendError(c, common.NewInternalServerError("failed to create keypair"))
		return
	}

	result := gin.H{
		"name":        req.Keypair.Name,
		"public_key":  publicKey,
		"fingerprint": fingerprint,
		"user_id":     userID,
	}

	// Include private key if generated
	if privateKey != nil {
		result["private_key"] = *privateKey
	}

	c.JSON(http.StatusCreated, gin.H{"keypair": result})
}

// DeleteKeypair deletes a keypair
func (svc *Service) DeleteKeypair(c *gin.Context) {
	keypairName := c.Param("id") // In Nova API, it's the name, not UUID
	userID := c.GetString("user_id")

	result, err := svc.activeDB().Exec(c.Request.Context(),
		"DELETE FROM keypairs WHERE user_id = $1 AND name = $2",
		userID, keypairName,
	)

	if err != nil {
		log.Error().Err(err).Str("operation", "delete_keypair").Msg("database error")
		common.SendError(c, common.NewInternalServerError("failed to delete keypair"))
		return
	}

	if result.RowsAffected() == 0 {
		common.SendError(c, common.NewNotFoundError("keypair"))
		return
	}

	// OpenStack returns 202 Accepted for async operations, but keypair deletion is synchronous
	c.Status(http.StatusAccepted)
}

// generateSSHKeyPair generates a new RSA SSH key pair
func generateSSHKeyPair() (publicKey, privateKey, fingerprint string, err error) {
	// Generate 2048-bit RSA key
	privateKeyObj, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to generate private key: %w", err)
	}

	// Generate private key in PEM format
	privateKeyPEM := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKeyObj),
	}
	privateKeyBytes := pem.EncodeToMemory(privateKeyPEM)
	privateKey = string(privateKeyBytes)

	// Generate public key in OpenSSH format
	publicKeyObj, err := ssh.NewPublicKey(&privateKeyObj.PublicKey)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to generate public key: %w", err)
	}
	publicKey = string(ssh.MarshalAuthorizedKey(publicKeyObj))

	// Calculate fingerprint
	fingerprint, err = calculateFingerprint(publicKey)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to calculate fingerprint: %w", err)
	}

	return publicKey, privateKey, fingerprint, nil
}

// calculateFingerprint calculates MD5 fingerprint of SSH public key
func calculateFingerprint(publicKey string) (string, error) {
	// Parse the public key
	key, _, _, _, err := ssh.ParseAuthorizedKey([]byte(publicKey))
	if err != nil {
		return "", err
	}

	// Calculate MD5 hash
	hash := md5.Sum(key.Marshal())

	// Format as XX:XX:XX:XX:...
	fingerprint := ""
	for i, b := range hash {
		if i > 0 {
			fingerprint += ":"
		}
		fingerprint += fmt.Sprintf("%02x", b)
	}

	return fingerprint, nil
}
