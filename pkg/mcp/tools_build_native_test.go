//nolint:testpackage // exercises unexported native-build helpers
package mcp

import (
	"testing"

	"github.com/M0Rf30/yap/v2/pkg/aptrepo"
	"github.com/M0Rf30/yap/v2/pkg/project"
	"github.com/M0Rf30/yap/v2/pkg/signing"
)

// TestPropagateSigning pins that the MCP build-wide signing config reaches
// every project: post-build signing keys off Project.Signing, never
// MultipleProject.Signing.
func TestPropagateSigning(t *testing.T) {
	cfg := &signing.Config{Enabled: true}
	mpc := &project.MultipleProject{
		Signing:  cfg,
		Projects: []*project.Project{{}, {}},
	}

	propagateSigning(mpc)

	for i, p := range mpc.Projects {
		if p.Signing != cfg {
			t.Errorf("project %d: Signing not propagated", i)
		}
	}
}

func TestPropagateSigningNoopWhenUnset(t *testing.T) {
	existing := &signing.Config{Enabled: true}
	mpc := &project.MultipleProject{Projects: []*project.Project{{Signing: existing}}}

	propagateSigning(mpc)

	if mpc.Projects[0].Signing != existing {
		t.Error("propagateSigning must not clobber per-project config when unset")
	}
}

// TestApplyUnverifiedRepos pins that the unverifiedRepos opt-in reaches the
// aptrepo global for the duration of a native build and is restored after.
func TestApplyUnverifiedRepos(t *testing.T) {
	t.Setenv("YAP_ALLOW_UNVERIFIED_REPOS", "")
	aptrepo.SetAllowUnverifiedRepos(false)

	t.Cleanup(func() { aptrepo.SetAllowUnverifiedRepos(false) })

	restore := applyUnverifiedRepos(false)

	if aptrepo.AllowUnverifiedRepos() {
		t.Fatal("allow=false must not enable the opt-in")
	}

	restore()

	restore = applyUnverifiedRepos(true)

	if !aptrepo.AllowUnverifiedRepos() {
		t.Fatal("allow=true must enable the opt-in")
	}

	restore()

	if aptrepo.AllowUnverifiedRepos() {
		t.Fatal("restore must reset the opt-in")
	}
}
