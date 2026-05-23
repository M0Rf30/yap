package command

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInitializeZapDescriptions(t *testing.T) {
	InitializeZapDescriptions()
	assert.NotEmpty(t, zapCmd.Short, "zapCmd.Short should be non-empty after InitializeZapDescriptions")
}

func TestZapCommandDefinition(t *testing.T) {
	assert.Equal(t, "zap [distro] <path>", zapCmd.Use)
	assert.NotNil(t, zapCmd.RunE)
	assert.Contains(t, zapCmd.Aliases, "clean")
}
