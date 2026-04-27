package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsSubcommand(t *testing.T) {
	assert.True(t, isSubcommand("server"))
	assert.True(t, isSubcommand("agent"))
	assert.True(t, isSubcommand("token"))
	assert.False(t, isSubcommand("--config"))
	assert.False(t, isSubcommand(""))
}
