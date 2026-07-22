//nolint:testpackage // exercises the unexported validateJSON path
package project

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/M0Rf30/yap/v2/pkg/repo"
)

// validMPC returns a MultipleProject satisfying every required-field tag so
// tests can isolate the repos[].country validation.
func validMPC() *MultipleProject {
	return &MultipleProject{
		BuildDir:    "/tmp/build",
		Description: "desc",
		Name:        "proj",
		Output:      "/tmp/out",
		Projects:    []*Project{{Name: "pkg"}},
	}
}

// TestValidateJSONRepoCountry proves the validator module enforces the
// repos[].country tag through the nested dive: a 2-letter code passes,
// longer or non-alpha values fail at yap.json validation time.
func TestValidateJSONRepoCountry(t *testing.T) {
	t.Parallel()

	mpc := validMPC()
	mpc.Repos = []repo.Repo{{Name: "u", URL: "http://{country}.example/", Country: "it"}}
	require.NoError(t, mpc.validateJSON(), "2-letter code must pass")

	mpc.Repos[0].Country = ""
	require.NoError(t, mpc.validateJSON(), "empty country must pass (omitempty)")

	mpc.Repos[0].Country = "ITA"
	assert.Error(t, mpc.validateJSON(), "3-letter code must fail len=2")

	mpc.Repos[0].Country = "1t"
	assert.Error(t, mpc.validateJSON(), "non-alpha code must fail alpha")
}
