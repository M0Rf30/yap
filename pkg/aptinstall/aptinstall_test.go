package aptinstall_test

import (
	"context"
	"testing"

	"github.com/M0Rf30/yap/v2/pkg/aptinstall"
)

// TestInstallEmpty tests that Install returns nil error for empty package list.
func TestInstallEmpty(t *testing.T) {
	err := aptinstall.Install(context.Background(), []string{})
	if err != nil {
		t.Errorf("Install with empty list should not error, got: %v", err)
	}
}

// TestInstallNonexistent tests that Install returns an error
// when packages are not in the cache.
func TestInstallNonexistent(t *testing.T) {
	// This test assumes the cache is empty or doesn't have these packages.
	// We expect an error because the package won't be found in the cache.
	err := aptinstall.Install(context.Background(), []string{"nonexistent-package-xyz"})
	if err == nil {
		t.Error("Expected error for nonexistent package")
	}
}
