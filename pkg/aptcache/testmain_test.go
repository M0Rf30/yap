package aptcache_test

import (
	"os"
	"testing"
	"time"

	"github.com/M0Rf30/yap/v2/pkg/httpclient"
)

// TestMain shrinks the httpclient retry backoff so error-path tests that
// exercise failing mirrors (5xx, refused connections) don't sleep through
// the production backoff schedule.
func TestMain(m *testing.M) {
	httpclient.SetRetryPolicy(3, time.Millisecond)
	os.Exit(m.Run())
}
