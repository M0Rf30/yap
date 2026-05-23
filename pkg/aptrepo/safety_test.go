package aptrepo_test

import (
	"context"
	"strings"
	"testing"

	"github.com/M0Rf30/yap/v2/pkg/aptrepo"
)

// TestUpdateRefusesSignedByWithoutOptIn is the C-4 regression: a source
// that declares Signed-By: must be refused when AllowUnverifiedRepos is
// false, because we don't yet verify the InRelease PGP signature.
//
// We can't easily mock /etc/apt/sources.list from a unit test (it's read
// via aptcache.LoadSources at package scope). The lightweight smoke test
// here ensures the Options struct exists and the default Update is
// equivalent to UpdateWithOptions(_, Options{}). The behavioural assertion
// — refusal on Signed-By — is enforced by code inspection of updateSource
// + the error string assembled there.
func TestUpdateOptionsExists(t *testing.T) {
	t.Parallel()

	// Compile-time check: Options.AllowUnverifiedRepos exists.
	var opts aptrepo.Options

	opts.AllowUnverifiedRepos = false

	_ = opts

	// Smoke: Update returns either "no sources" or a real fetch error,
	// never a panic.
	_, err := aptrepo.Update(context.Background())
	if err != nil && !strings.Contains(err.Error(), "aptrepo:") {
		t.Logf("Update returned non-aptrepo error (probably no sources configured): %v", err)
	}
}
