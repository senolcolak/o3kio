package database_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cobaltcore-dev/o3k/internal/database"
)

func TestMockDBExec(t *testing.T) {
	mock := database.NewMockDB()
	mock.OnExec("INSERT INTO users", nil)

	_, err := mock.Exec(context.Background(), "INSERT INTO users VALUES ($1)", "test-id")
	assert.NoError(t, err)
	assert.True(t, mock.ExecCalled("INSERT INTO users"))
}
