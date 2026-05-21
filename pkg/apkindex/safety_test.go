package apkindex_test

import (
	"context"
	"strings"
	"testing"

	"github.com/M0Rf30/yap/v2/pkg/apkindex"
)

// TestInstallRefusesWithoutOptIn is the C-5 regression: by default,
// InstallPackages must refuse to run because the APK package signature is
// not yet verified. Callers must explicitly acknowledge the gap via
// InstallOptions.AllowUnverifiedPackages.
func TestInstallRefusesWithoutOptIn(t *testing.T) {
	t.Parallel()

	idx := apkindex.NewIndex()

	err := idx.InstallPackages(context.Background(), []string{"hello"})
	if err == nil {
		t.Fatal("expected refusal error, got nil")
	}

	if !strings.Contains(err.Error(), "signature verification") {
		t.Fatalf("expected signature-verification refusal, got: %v", err)
	}
}

// TestInstallProceedsWithOptIn confirms the opt-in path actually runs
// past the gate. We pass an empty package list so the call returns nil
// once it gets through the verification gate.
func TestInstallProceedsWithOptIn(t *testing.T) {
	t.Parallel()

	idx := apkindex.NewIndex()

	err := idx.InstallPackagesWithOptions(
		context.Background(),
		nil, // empty list
		apkindex.InstallOptions{AllowUnverifiedPackages: true},
	)
	if err != nil {
		t.Fatalf("expected nil for empty install with opt-in, got %v", err)
	}
}
