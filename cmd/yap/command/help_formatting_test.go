package command

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func TestSetupEnhancedHelp(t *testing.T) {
	// Test that SetupEnhancedHelp doesn't panic
	assert.NotPanics(t, func() {
		SetupEnhancedHelp()
	})
}

func TestCustomErrorHandler(t *testing.T) {
	// Create a test command
	cmd := &cobra.Command{
		Use:   "test",
		Short: "Test command",
	}

	// Test with a sample error
	testErr := assert.AnError

	// Should not panic and should return the same error
	var result error

	assert.NotPanics(t, func() {
		result = CustomErrorHandler(cmd, testErr)
	})

	assert.Equal(t, testErr, result)
}

func TestCustomErrorHandlerWithNil(t *testing.T) {
	// Create a test command
	cmd := &cobra.Command{
		Use:   "test",
		Short: "Test command",
	}

	// Test with nil error
	var result error

	assert.NotPanics(t, func() {
		result = CustomErrorHandler(cmd, nil)
	})

	assert.Nil(t, result)
}
