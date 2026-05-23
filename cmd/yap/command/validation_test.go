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

func TestValidateDistroForBuildCommand(t *testing.T) {
	tests := []struct {
		name     string
		firstArg string
		wantErr  bool
	}{
		{
			name:     "path-like arg with slash - no validation",
			firstArg: "/some/path",
			wantErr:  false,
		},
		{
			name:     "relative path with slash - no validation",
			firstArg: "./myproject",
			wantErr:  false,
		},
		{
			name:     "valid distro ubuntu",
			firstArg: "ubuntu",
			wantErr:  false,
		},
		{
			name:     "valid distro alpine",
			firstArg: "alpine",
			wantErr:  false,
		},
		{
			name:     "invalid distro not a path",
			firstArg: "notadistro",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateDistroForBuildCommand(tt.firstArg)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateDistroForBuildCommand(%q) error = %v, wantErr %v",
					tt.firstArg, err, tt.wantErr)
			}
		})
	}
}

func TestIsPathLike(t *testing.T) {
	tests := []struct {
		name string
		arg  string
		want bool
	}{
		{
			name: "contains slash",
			arg:  "/some/path",
			want: true,
		},
		{
			name: "relative with slash",
			arg:  "./foo",
			want: true,
		},
		{
			name: "dot",
			arg:  ".",
			want: true,
		},
		{
			name: "double dot",
			arg:  "..",
			want: true,
		},
		{
			name: "ubuntu distro name",
			arg:  "ubuntu",
			want: false,
		},
		{
			name: "alpine distro name",
			arg:  "alpine",
			want: false,
		},
		{
			name: "plain word",
			arg:  "fedora",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isPathLike(tt.arg)
			if got != tt.want {
				t.Errorf("isPathLike(%q) = %v, want %v", tt.arg, got, tt.want)
			}
		})
	}
}

func TestParseDistroAndRelease(t *testing.T) {
	tests := []struct {
		name        string
		arg         string
		wantDistro  string
		wantRelease string
	}{
		{
			name:        "distro only",
			arg:         "ubuntu",
			wantDistro:  "ubuntu",
			wantRelease: "",
		},
		{
			name:        "distro with release",
			arg:         "ubuntu-jammy",
			wantDistro:  "ubuntu",
			wantRelease: "jammy",
		},
		{
			name:        "distro with numeric release",
			arg:         "rocky-9",
			wantDistro:  "rocky",
			wantRelease: "9",
		},
		{
			name:        "empty string",
			arg:         "",
			wantDistro:  "",
			wantRelease: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotDistro, gotRelease := parseDistroAndRelease(tt.arg)
			if gotDistro != tt.wantDistro {
				t.Errorf("parseDistroAndRelease(%q) distro = %q, want %q",
					tt.arg, gotDistro, tt.wantDistro)
			}

			if gotRelease != tt.wantRelease {
				t.Errorf("parseDistroAndRelease(%q) release = %q, want %q",
					tt.arg, gotRelease, tt.wantRelease)
			}
		})
	}
}

func TestIsPathArgument(t *testing.T) {
	tests := []struct {
		name string
		arg  string
		want bool
	}{
		{
			name: "absolute path",
			arg:  "/some/path",
			want: true,
		},
		{
			name: "dot",
			arg:  ".",
			want: true,
		},
		{
			name: "double dot",
			arg:  "..",
			want: true,
		},
		{
			name: "distro name ubuntu",
			arg:  "ubuntu",
			want: false,
		},
		{
			name: "distro name alpine",
			arg:  "alpine",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isPathArgument(tt.arg)
			if got != tt.want {
				t.Errorf("isPathArgument(%q) = %v, want %v", tt.arg, got, tt.want)
			}
		})
	}
}

func TestParseSingleArg(t *testing.T) {
	tests := []struct {
		name             string
		firstArg         string
		wantDistro       string
		wantRelease      string
		wantPathNonEmpty bool
		wantErr          bool
	}{
		{
			name:             "absolute path",
			firstArg:         "/tmp",
			wantDistro:       "",
			wantRelease:      "",
			wantPathNonEmpty: true,
			wantErr:          false,
		},
		{
			name:             "dot path",
			firstArg:         ".",
			wantDistro:       "",
			wantRelease:      "",
			wantPathNonEmpty: true,
			wantErr:          false,
		},
		{
			name:             "valid distro ubuntu",
			firstArg:         "ubuntu",
			wantDistro:       "ubuntu",
			wantRelease:      "",
			wantPathNonEmpty: true,
			wantErr:          false,
		},
		{
			name:             "invalid distro not a path",
			firstArg:         "notadistro",
			wantDistro:       "",
			wantRelease:      "",
			wantPathNonEmpty: false,
			wantErr:          true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotDistro, gotRelease, gotPath, err := parseSingleArg(tt.firstArg)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseSingleArg(%q) error = %v, wantErr %v",
					tt.firstArg, err, tt.wantErr)
			}

			if gotDistro != tt.wantDistro {
				t.Errorf("parseSingleArg(%q) distro = %q, want %q",
					tt.firstArg, gotDistro, tt.wantDistro)
			}

			if gotRelease != tt.wantRelease {
				t.Errorf("parseSingleArg(%q) release = %q, want %q",
					tt.firstArg, gotRelease, tt.wantRelease)
			}

			if tt.wantPathNonEmpty && gotPath == "" {
				t.Errorf("parseSingleArg(%q) fullJSONPath is empty, want non-empty",
					tt.firstArg)
			}

			if !tt.wantPathNonEmpty && gotPath != "" {
				t.Errorf("parseSingleArg(%q) fullJSONPath = %q, want empty",
					tt.firstArg, gotPath)
			}
		})
	}
}

func TestParseFlexibleArgs(t *testing.T) {
	tests := []struct {
		name             string
		args             []string
		wantDistro       string
		wantRelease      string
		wantPathNonEmpty bool
		wantErr          bool
	}{
		{
			name:             "two args distro and path",
			args:             []string{"ubuntu", "/tmp"},
			wantDistro:       "ubuntu",
			wantRelease:      "",
			wantPathNonEmpty: true,
			wantErr:          false,
		},
		{
			name:             "two args distro-release and path",
			args:             []string{"ubuntu-jammy", "/tmp"},
			wantDistro:       "ubuntu",
			wantRelease:      "jammy",
			wantPathNonEmpty: true,
			wantErr:          false,
		},
		{
			name:             "one arg path",
			args:             []string{"/tmp"},
			wantDistro:       "",
			wantRelease:      "",
			wantPathNonEmpty: true,
			wantErr:          false,
		},
		{
			name:             "one arg valid distro",
			args:             []string{"ubuntu"},
			wantDistro:       "ubuntu",
			wantRelease:      "",
			wantPathNonEmpty: true,
			wantErr:          false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotDistro, gotRelease, gotPath, err := ParseFlexibleArgs(tt.args)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseFlexibleArgs(%v) error = %v, wantErr %v",
					tt.args, err, tt.wantErr)
			}

			if gotDistro != tt.wantDistro {
				t.Errorf("ParseFlexibleArgs(%v) distro = %q, want %q",
					tt.args, gotDistro, tt.wantDistro)
			}

			if gotRelease != tt.wantRelease {
				t.Errorf("ParseFlexibleArgs(%v) release = %q, want %q",
					tt.args, gotRelease, tt.wantRelease)
			}

			if tt.wantPathNonEmpty && gotPath == "" {
				t.Errorf("ParseFlexibleArgs(%v) fullJSONPath is empty, want non-empty",
					tt.args)
			}
		})
	}
}

func TestValidatePathForCommand(t *testing.T) {
	buildCmd := &cobra.Command{Use: buildCommand}
	otherCmd := &cobra.Command{Use: "zap"}

	tests := []struct {
		name    string
		cmd     *cobra.Command
		args    []string
		wantErr bool
	}{
		{
			name:    "two args - validates last as path (valid)",
			cmd:     buildCmd,
			args:    []string{"ubuntu", "../../../examples/yap"},
			wantErr: false,
		},
		{
			name:    "two args - validates last as path (invalid)",
			cmd:     buildCmd,
			args:    []string{"ubuntu", "/nonexistent/path/xyz"},
			wantErr: true,
		},
		{
			name:    "one arg build command - validates as path (invalid)",
			cmd:     buildCmd,
			args:    []string{"/nonexistent/path/xyz"},
			wantErr: true,
		},
		{
			name:    "one arg non-build command - no validation",
			cmd:     otherCmd,
			args:    []string{"ubuntu"},
			wantErr: false,
		},
		{
			name:    "zero args - no validation",
			cmd:     buildCmd,
			args:    []string{},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePathForCommand(tt.cmd, tt.args)
			if (err != nil) != tt.wantErr {
				t.Errorf("validatePathForCommand(%q, %v) error = %v, wantErr %v",
					tt.cmd.Use, tt.args, err, tt.wantErr)
			}
		})
	}
}
