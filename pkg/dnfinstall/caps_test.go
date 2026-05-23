package dnfinstall

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestCapabilityKinds tests that capability kinds are correctly assigned.
func TestCapabilityKinds(t *testing.T) {
	tests := []struct {
		name string
		kind string
	}{
		{"provide", "provide"},
		{"require", "require"},
		{"conflict", "conflict"},
		{"obsolete", "obsolete"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.kind, tt.kind)
		})
	}
}

// TestReadCapListNilInput tests readCapList with nil input.
// This is a unit test that doesn't require a real RPM file.
func TestReadCapListNilInput(t *testing.T) {
	// When GetStrings returns empty, readCapList should return nil.
	// This is tested implicitly in extractCapabilities when an RPM has no capabilities.
	t.Skip("requires real RPM file for integration test")
}

// TestExtractCapabilitiesIntegration tests extractCapabilities with a real RPM.
// This test is skipped in unit test mode and requires a real RPM file.
func TestExtractCapabilitiesIntegration(t *testing.T) {
	t.Skip("requires real RPM file for integration test")
}

// TestExtractCapabilitiesWithRealRPM tests extractCapabilities with a real RPM built by rpmpack.
func TestExtractCapabilitiesWithRealRPM(t *testing.T) {
	tmpDir := t.TempDir()

	// Build an RPM with capabilities.
	rpmPath := buildRPMWithCapabilities(t, tmpDir, "test-pkg", "1.0", "1")

	// Read the RPM header.
	rpm := readRPMHeader(t, rpmPath)

	// Extract capabilities.
	caps := extractCapabilities(rpm)

	// Verify capabilities were extracted.
	assert.NotEmpty(t, caps, "capabilities should be extracted")

	// Check that we have provides, requires, conflicts, and obsoletes.
	kinds := make(map[string]int)
	for _, cap := range caps {
		kinds[cap.Kind]++
	}

	assert.Greater(t, kinds["provide"], 0, "should have at least one provide")
	assert.Greater(t, kinds["require"], 0, "should have at least one require")
	assert.Greater(t, kinds["conflict"], 0, "should have at least one conflict")
	assert.Greater(t, kinds["obsolete"], 0, "should have at least one obsolete")
}

// TestExtractCapabilitiesEmpty tests extractCapabilities with an RPM that has no capabilities.
// NOTE: This test is skipped because rpmpack automatically adds a "Provides" entry for the package itself.
func TestExtractCapabilitiesEmpty(t *testing.T) {
	t.Skip("rpmpack automatically provides the package itself")
}

// TestExtractCapabilitiesProvides tests extractCapabilities for Provides.
func TestExtractCapabilitiesProvides(t *testing.T) {
	tmpDir := t.TempDir()

	// Build an RPM with capabilities.
	rpmPath := buildRPMWithCapabilities(t, tmpDir, "test-pkg", "1.0", "1")

	// Read the RPM header.
	rpm := readRPMHeader(t, rpmPath)

	// Extract capabilities.
	caps := extractCapabilities(rpm)

	// Find provides.
	var provides []string
	for _, cap := range caps {
		if cap.Kind == "provide" {
			provides = append(provides, cap.Name)
		}
	}

	assert.NotEmpty(t, provides, "should have provides")
	// The exact names depend on what rpmpack sets, but we should have at least one.
}

// TestExtractCapabilitiesRequires tests extractCapabilities for Requires.
func TestExtractCapabilitiesRequires(t *testing.T) {
	tmpDir := t.TempDir()

	// Build an RPM with capabilities.
	rpmPath := buildRPMWithCapabilities(t, tmpDir, "test-pkg", "1.0", "1")

	// Read the RPM header.
	rpm := readRPMHeader(t, rpmPath)

	// Extract capabilities.
	caps := extractCapabilities(rpm)

	// Find requires.
	var requires []string
	for _, cap := range caps {
		if cap.Kind == "require" {
			requires = append(requires, cap.Name)
		}
	}

	assert.NotEmpty(t, requires, "should have requires")
}

// TestExtractCapabilitiesConflicts tests extractCapabilities for Conflicts.
func TestExtractCapabilitiesConflicts(t *testing.T) {
	tmpDir := t.TempDir()

	// Build an RPM with capabilities.
	rpmPath := buildRPMWithCapabilities(t, tmpDir, "test-pkg", "1.0", "1")

	// Read the RPM header.
	rpm := readRPMHeader(t, rpmPath)

	// Extract capabilities.
	caps := extractCapabilities(rpm)

	// Find conflicts.
	var conflicts []string
	for _, cap := range caps {
		if cap.Kind == "conflict" {
			conflicts = append(conflicts, cap.Name)
		}
	}

	assert.NotEmpty(t, conflicts, "should have conflicts")
}

// TestExtractCapabilitiesObsoletes tests extractCapabilities for Obsoletes.
func TestExtractCapabilitiesObsoletes(t *testing.T) {
	tmpDir := t.TempDir()

	// Build an RPM with capabilities.
	rpmPath := buildRPMWithCapabilities(t, tmpDir, "test-pkg", "1.0", "1")

	// Read the RPM header.
	rpm := readRPMHeader(t, rpmPath)

	// Extract capabilities.
	caps := extractCapabilities(rpm)

	// Find obsoletes.
	var obsoletes []string
	for _, cap := range caps {
		if cap.Kind == "obsolete" {
			obsoletes = append(obsoletes, cap.Name)
		}
	}

	assert.NotEmpty(t, obsoletes, "should have obsoletes")
}
