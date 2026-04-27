package compat_test

import (
	"testing"

	"github.com/cobaltcore-dev/o3k/internal/compat"
	"github.com/stretchr/testify/assert"
)

func TestNewCheckerDefaults(t *testing.T) {
	c := compat.NewChecker(compat.CheckerOptions{})
	assert.Equal(t, "json", c.OutputFormat)
	assert.Equal(t, compat.DefaultListenAddr, c.ListenAddr)
}
