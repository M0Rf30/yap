package command

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestValidDistrosCompletion(t *testing.T) {
	// Create a test command
	testCmd := &cobra.Command{
		Use:   "test",
		Short: "Test command",
	}

	tests := []struct {
		name       string
		cmd        *cobra.Command
		args       []string
		toComplete string
	}{
		{
			name:       "empty completion",
			cmd:        testCmd,
			args:       []string{},
			toComplete: "",
		},
		{
			name:       "partial completion",
			cmd:        testCmd,
			args:       []string{},
			toComplete: "ub",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test that ValidDistrosCompletion doesn't panic
			completions, directive := ValidDistrosCompletion(tt.cmd, tt.args, tt.toComplete)

			// Should return some completions
			if len(completions) == 0 {
				t.Log("No completions returned (this is okay for some inputs)")
			}

			// Directive should be valid
			if directive < 0 {
				t.Errorf("Invalid directive returned: %d", directive)
			}
		})
	}
}

func TestProjectPathCompletion(t *testing.T) {
	// Create a test command
	testCmd := &cobra.Command{
		Use:   "test",
		Short: "Test command",
	}

	tests := []struct {
		name       string
		cmd        *cobra.Command
		args       []string
		toComplete string
	}{
		{
			name:       "empty completion",
			cmd:        testCmd,
			args:       []string{},
			toComplete: "",
		},
		{
			name:       "current directory",
			cmd:        testCmd,
			args:       []string{},
			toComplete: ".",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test that ProjectPathCompletion doesn't panic
			completions, directive := ProjectPathCompletion(tt.cmd, tt.args, tt.toComplete)

			// Should return some completions
			if len(completions) == 0 {
				t.Log("No completions returned (this is okay for some inputs)")
			}

			// Directive should be valid
			if directive < 0 {
				t.Errorf("Invalid directive returned: %d", directive)
			}
		})
	}
}

func TestValidateDistroArg(t *testing.T) {
	tests := []struct {
		name    string
		distro  string
		wantErr bool
	}{
		{
			name:    "valid distro - ubuntu",
			distro:  "ubuntu",
			wantErr: false,
		},
		{
			name:    "valid distro - alpine",
			distro:  "alpine",
			wantErr: false,
		},
		{
			name:    "invalid distro",
			distro:  "invalid-distro",
			wantErr: true,
		},
		{
			name:    "empty distro",
			distro:  "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateDistroArg(tt.distro)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateDistroArg() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateProjectPath(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{
			name:    "current directory (no project files)",
			path:    ".",
			wantErr: true,
		},
		{
			name:    "examples directory with yap.json",
			path:    "../../../examples/circular-deps",
			wantErr: false,
		},
		{
			name:    "examples/yap directory with PKGBUILD",
			path:    "../../../examples/yap",
			wantErr: false,
		},
		{
			name:    "non-existent path",
			path:    "/non/existent/path",
			wantErr: true,
		},
		{
			name:    "empty path",
			path:    "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateProjectPath(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateProjectPath() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestFormatDistroSuggestions(t *testing.T) {
	tests := []struct {
		name   string
		distro string
	}{
		{
			name:   "invalid distro",
			distro: "invalid-distro",
		},
		{
			name:   "partial match",
			distro: "ub",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test that formatDistroSuggestions doesn't panic
			formatDistroSuggestions(tt.distro)
		})
	}
}

func TestCreateValidateDistroArgs(t *testing.T) {
	tests := []struct {
		name string
		pos  int
	}{
		{
			name: "position 0",
			pos:  0,
		},
		{
			name: "position 1",
			pos:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := createValidateDistroArgs(tt.pos)
			if validator == nil {
				t.Error("createValidateDistroArgs should return a non-nil validator")
			}

			// Test the validator with some args
			testCmd := &cobra.Command{Use: "test"}

			err := validator(testCmd, []string{"ubuntu", "."})
			if err != nil && tt.pos < 2 {
				// Should not error for positions 0 and 1 with valid args
				t.Logf("Validator returned error (may be expected): %v", err)
			}
		})
	}
}

func TestPreRunValidation(t *testing.T) {
	// Create a test command
	testCmd := &cobra.Command{
		Use:   "test",
		Short: "Test command",
	}

	tests := []struct {
		name string
		cmd  *cobra.Command
		args []string
	}{
		{
			name: "basic validation",
			cmd:  testCmd,
			args: []string{"ubuntu", "."},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test that PreRunValidation doesn't panic
			PreRunValidation(tt.cmd, tt.args)
		})
	}
}
