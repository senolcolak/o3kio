package scheduler_test

import (
	"testing"

	"github.com/cobaltcore-dev/o3k/internal/database"
	"github.com/cobaltcore-dev/o3k/internal/scheduler"
	"github.com/stretchr/testify/assert"
)

func TestNewReconciler(t *testing.T) {
	mock := database.NewMockDB()
	r := scheduler.NewReconciler(mock, 30)
	assert.NotNil(t, r)
}

func TestNewReconcilerDefaultInterval(t *testing.T) {
	mock := database.NewMockDB()
	r := scheduler.NewReconciler(mock, 0)
	assert.NotNil(t, r)
}
