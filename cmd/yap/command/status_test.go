package command

import (
	"testing"
)

func TestStatusCommand(t *testing.T) {
	tests := []struct {
		name string
	}{
		{
			name: "Status command execution",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test that showSystemStatus doesn't panic
			showSystemStatus()
		})
	}
}

func TestStatusCommandDefinition(t *testing.T) {
	if statusCmd.Use != "status" {
		t.Errorf("Expected status command use to be 'status', got %q", statusCmd.Use)
	}

	if statusCmd.Short == "" {
		t.Error("Status command should have a short description")
	}

	if statusCmd.Long == "" {
		t.Error("Status command should have a long description")
	}

	if statusCmd.Run == nil {
		t.Error("Status command should have a Run function")
	}

	// Test aliases
	expectedAliases := []string{"info", "env"}
	if len(statusCmd.Aliases) != len(expectedAliases) {
		t.Errorf("Expected %d aliases, got %d", len(expectedAliases), len(statusCmd.Aliases))
	}

	for i, alias := range expectedAliases {
		if i >= len(statusCmd.Aliases) || statusCmd.Aliases[i] != alias {
			t.Errorf("Expected alias %q at position %d, got %q", alias, i, statusCmd.Aliases[i])
		}
	}
}

func TestGetDistroFamily(t *testing.T) {
	tests := []struct {
		name     string
		distro   string
		expected string
	}{
		{name: "debian", distro: "debian", expected: "debian"},
		{name: "ubuntu", distro: "ubuntu", expected: "debian"},
		{name: "fedora", distro: "fedora", expected: "redhat"},
		{name: "centos", distro: "centos", expected: "redhat"},
		{name: "alpine", distro: "alpine", expected: "alpine"},
		{name: "arch", distro: "arch", expected: "arch"},
		{name: "unknown", distro: "unknown", expected: "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getDistroFamily(tt.distro)
			if result != tt.expected {
				t.Errorf("Expected %q for distro %q, got %q", tt.expected, tt.distro, result)
			}
		})
	}
}

func TestContains(t *testing.T) {
	tests := []struct {
		name     string
		str      string
		substr   string
		expected bool
	}{
		{name: "substring exists", str: "hello world", substr: "world", expected: true},
		{name: "substring not exists", str: "hello world", substr: "foo", expected: false},
		{name: "empty substring", str: "hello world", substr: "", expected: true},
		{name: "empty string", str: "", substr: "hello", expected: false},
		{name: "equal strings", str: "hello", substr: "hello", expected: true},
		{name: "substring longer than string", str: "hi", substr: "hello", expected: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := contains(tt.str, tt.substr)
			if result != tt.expected {
				t.Errorf("Expected %v for contains(%q, %q), got %v", tt.expected, tt.str, tt.substr, result)
			}
		})
	}
}

func TestCheckContainerRuntime(t *testing.T) {
	tests := []struct {
		name string
	}{
		{
			name: "Check container runtime",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test that checkContainerRuntime doesn't panic
			// The actual functionality depends on system state
			checkContainerRuntime()
		})
	}
}
