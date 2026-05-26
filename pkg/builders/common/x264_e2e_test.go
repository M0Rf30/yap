package common

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/M0Rf30/yap/v2/pkg/archive"
)

// TestExtractRPM_RealVendorX264 is an opt-in end-to-end test against the
// exact RPM that triggered the "invalid cpio path /opt/..." failure. Skips
// when YAP_TEST_REAL_RPM is unset so CI is unaffected.
//
// Run with:
//
//	YAP_TEST_REAL_RPM=/path/to/vendor-x264-...rpm \
//	  go test -run RealVendor ./pkg/builders/common -v
func TestExtractRPM_RealVendorX264(t *testing.T) {
	rpmPath := os.Getenv("YAP_TEST_REAL_RPM")
	if rpmPath == "" {
		t.Skip("YAP_TEST_REAL_RPM not set")
	}

	dest := t.TempDir()

	if err := archive.ExtractRPM(rpmPath, dest); err != nil {
		t.Fatalf("ExtractRPM on real RPM failed: %v", err)
	}

	// Walk and assert at least one binary lands under opt/vendor/common.
	var found []string

	_ = filepath.Walk(dest, func(p string, fi os.FileInfo, err error) error {
		if err == nil && !fi.IsDir() {
			rel, _ := filepath.Rel(dest, p)
			if strings.HasPrefix(rel, "opt/vendor/common") {
				found = append(found, rel)
			}
		}

		return nil
	})

	if len(found) == 0 {
		t.Fatal("no files extracted under opt/vendor/common — regression")
	}

	t.Logf("extracted %d files; sample: %v", len(found), found[:min(3, len(found))])
}
