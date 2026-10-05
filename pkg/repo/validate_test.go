//nolint:testpackage // exercises unexported validation helpers
package repo

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateRepo(t *testing.T) {
	ok := Repo{
		Name: "my-repo_1.0", URL: "https://example.com/$basearch",
		Suite: "jammy-updates", Components: []string{"main", "non-free"},
		KeyURL: "https://example.com/key.gpg",
	}
	require.NoError(t, validateRepo(&ok))

	cases := map[string]func(r *Repo){
		"traversal name":    func(r *Repo) { r.Name = "../../x" },
		"slash name":        func(r *Repo) { r.Name = "a/b" },
		"dot name":          func(r *Repo) { r.Name = ".." },
		"newline name":      func(r *Repo) { r.Name = "a\nb" },
		"newline url":       func(r *Repo) { r.URL = "https://e.com/\ngpgcheck=0" },
		"space url":         func(r *Repo) { r.URL = "https://e.com/ x" },
		"file scheme":       func(r *Repo) { r.URL = "file:///etc" },
		"no host":           func(r *Repo) { r.URL = "https://" },
		"newline suite":     func(r *Repo) { r.Suite = "jammy\nTrusted: yes" },
		"colon suite":       func(r *Repo) { r.Suite = "jammy Trusted: yes" },
		"newline component": func(r *Repo) { r.Components = []string{"main\nTrusted: yes"} },
		"bad key url":       func(r *Repo) { r.KeyURL = "ftp://e.com/k" },
	}

	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			r := ok
			mutate(&r)
			assert.Error(t, validateRepo(&r))
		})
	}
}

func TestSetupOneRejectsInjection(t *testing.T) {
	r := &Repo{Name: "../../evil", URL: "https://example.com", Suite: "jammy"}
	require.Error(t, setupOneContext(context.Background(), "apt", r, "ubuntu", "", 0))
}
