package common

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/M0Rf30/yap/v2/pkg/archive"
)

// TestExtractRPM_RealCarbonioX264 is an opt-in end-to-end test against the
// exact RPM that triggered the "invalid cpio path /opt/..." failure. Skips
// when YAP_TEST_REAL_RPM is unset so CI is unaffected.
//
// Run with:
//
//	YAP_TEST_REAL_RPM=/path/to/carbonio-x264-...rpm \
//	  go test -run RealCarbonio ./pkg/builders/common -v
func TestExtractRPM_RealCarbonioX264(t *testing.T) {
	rpmPath := os.Getenv("YAP_TEST_REAL_RPM")
	if rpmPath == "" {
		t.Skip("YAP_TEST_REAL_RPM not set")
	}

	dest := t.TempDir()

	if err := archive.ExtractRPM(rpmPath, dest); err != nil {
		t.Fatalf("ExtractRPM on real RPM failed: %v", err)
	}

	// Walk and assert at least one binary lands under opt/zextras/common.
	var found []string

	_ = filepath.Walk(dest, func(p string, fi os.FileInfo, err error) error {
		if err == nil && !fi.IsDir() {
			rel, _ := filepath.Rel(dest, p)
			if strings.HasPrefix(rel, "opt/zextras/common") {
				found = append(found, rel)
			}
		}

		return nil
	})

	if len(found) == 0 {
		t.Fatal("no files extracted under opt/zextras/common — regression")
	}

	t.Logf("extracted %d files; sample: %v", len(found), found[:min(3, len(found))])
}
