package database_test

import (
	"context"
	"testing"

	"github.com/cobaltcore-dev/o3k/internal/database"
	"github.com/stretchr/testify/assert"
)

func TestSeededMockDBDomainLookup(t *testing.T) {
	db := database.NewSeededMockDB()

	var domainID string
	err := db.QueryRow(context.Background(),
		"SELECT id FROM domains WHERE name = $1", "Default",
	).Scan(&domainID)

	assert.NoError(t, err)
	assert.Equal(t, "default-domain-id", domainID)
}

func TestSeededMockDBUserLookup(t *testing.T) {
	db := database.NewSeededMockDB()

	var id, name, hash string
	var enabled bool
	var domainID string
	err := db.QueryRow(context.Background(),
		"SELECT id, name, password_hash, enabled, domain_id FROM users WHERE name = $1 AND domain_id = $2",
		"admin", "default-domain-id",
	).Scan(&id, &name, &hash, &enabled, &domainID)

	assert.NoError(t, err)
	assert.Equal(t, "admin", name)
	assert.True(t, enabled)
}

func TestSeededMockDBProjectLookup(t *testing.T) {
	db := database.NewSeededMockDB()

	var id, name, desc string
	var enabled bool
	var domainID string
	err := db.QueryRow(context.Background(),
		"SELECT id, name, description, enabled, domain_id FROM projects WHERE name = $1 AND domain_id = $2",
		"default", "default-domain-id",
	).Scan(&id, &name, &desc, &enabled, &domainID)

	assert.NoError(t, err)
	assert.Equal(t, "default", name)
	assert.True(t, enabled)
}
