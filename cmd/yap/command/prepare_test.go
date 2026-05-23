package command

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInitializePrepareDescriptions(t *testing.T) {
	InitializePrepareDescriptions()
	assert.NotEmpty(t, prepareCmd.Short, "prepareCmd.Short should be non-empty after InitializePrepareDescriptions")
}

func TestPrepareCommandDefinition(t *testing.T) {
	assert.Equal(t, "prepare [distro]", prepareCmd.Use)
	assert.NotNil(t, prepareCmd.Run)
	assert.Contains(t, prepareCmd.Aliases, "prep")
}
