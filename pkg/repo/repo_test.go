//nolint:testpackage // exercises unexported parsing helpers
package repo

import (
	"reflect"
	"testing"

	"github.com/M0Rf30/yap/v2/pkg/constants"
)

// TestParseFlagsAcceptsFullSpec verifies that ParseFlags correctly parses a
// complete repository specification with all supported fields.
func TestParseFlagsAcceptsFullSpec(t *testing.T) {
	tokens := []string{
		"name=carbonio,url=https://repo.example.com/ubuntu,suite=jammy,components=main+contrib,keyURL=https://repo.example.com/key.gpg,distros=ubuntu+debian,format=deb,gpgCheck=true",
	}

	repos, err := ParseFlags(tokens)
	if err != nil {
		t.Fatalf("ParseFlags returned error: %v", err)
	}

	if len(repos) != 1 {
		t.Fatalf("expected 1 repo, got %d", len(repos))
	}

	want := Repo{
		Name:       "carbonio",
		URL:        "https://repo.example.com/ubuntu",
		Suite:      "jammy",
		Components: []string{"main", "contrib"},
		KeyURL:     "https://repo.example.com/key.gpg",
		Distros:    []string{"ubuntu", "debian"},
		Format:     "deb",
		GPGCheck:   true,
	}

	if !reflect.DeepEqual(repos[0], want) {
		t.Fatalf("repo mismatch:\n got %+v\nwant %+v", repos[0], want)
	}
}

// TestParseFlagsRejectsBadInput verifies that ParseFlags rejects malformed input.
func TestParseFlagsRejectsBadInput(t *testing.T) {
	cases := []struct {
		name  string
		token string
	}{
		{"missing equals", "nameonly"},
		{"unknown key", "name=foo,bogus=bar"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := ParseFlags([]string{c.token}); err == nil {
				t.Fatalf("expected error for %q", c.token)
			}
		})
	}
}

// TestParseFlagsAcceptsKeyAlias verifies that the "key" alias for "keyURL" works.
func TestParseFlagsAcceptsKeyAlias(t *testing.T) {
	repos, err := ParseFlags([]string{"name=x,url=https://example.com,suite=jammy,key=https://example.com/k.gpg"})
	if err != nil {
		t.Fatalf("ParseFlags returned error: %v", err)
	}

	if repos[0].KeyURL != "https://example.com/k.gpg" {
		t.Fatalf("alias `key` did not populate KeyURL: %+v", repos[0])
	}
}

// TestParseFlagsEmptyTokenList verifies that an empty token list returns an
// empty repo slice without error.
func TestParseFlagsEmptyTokenList(t *testing.T) {
	repos, err := ParseFlags([]string{})
	if err != nil {
		t.Fatalf("ParseFlags returned error: %v", err)
	}

	if len(repos) != 0 {
		t.Fatalf("expected 0 repos, got %d", len(repos))
	}
}

// TestParseFlagsMultipleTokens verifies that ParseFlags correctly handles
// multiple repository specifications.
func TestParseFlagsMultipleTokens(t *testing.T) {
	tokens := []string{
		"name=repo1,url=https://example.com/repo1",
		"name=repo2,url=https://example.com/repo2,suite=focal",
	}

	repos, err := ParseFlags(tokens)
	if err != nil {
		t.Fatalf("ParseFlags returned error: %v", err)
	}

	if len(repos) != 2 {
		t.Fatalf("expected 2 repos, got %d", len(repos))
	}

	if repos[0].Name != "repo1" || repos[1].Name != "repo2" {
		t.Fatalf("repo names mismatch: got %q, %q", repos[0].Name, repos[1].Name)
	}
}

// TestParseFlagsGPGCheckVariants verifies that gpgCheck accepts both "true"
// and "1" as truthy values.
func TestParseFlagsGPGCheckVariants(t *testing.T) {
	cases := []struct {
		name  string
		token string
		want  bool
	}{
		{"gpgCheck=true", "name=x,url=https://example.com,gpgCheck=true", true},
		{"gpgCheck=1", "name=x,url=https://example.com,gpgCheck=1", true},
		{"gpgCheck=false", "name=x,url=https://example.com,gpgCheck=false", false},
		{"gpgCheck=0", "name=x,url=https://example.com,gpgCheck=0", false},
		{"gpgCheck=anything", "name=x,url=https://example.com,gpgCheck=anything", false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			repos, err := ParseFlags([]string{c.token})
			if err != nil {
				t.Fatalf("ParseFlags returned error: %v", err)
			}

			if repos[0].GPGCheck != c.want {
				t.Fatalf("gpgCheck = %v, want %v", repos[0].GPGCheck, c.want)
			}
		})
	}
}

// TestParseFlagsComponentsWithPlus verifies that components are correctly
// split by '+' separator.
func TestParseFlagsComponentsWithPlus(t *testing.T) {
	repos, err := ParseFlags([]string{"name=x,url=https://example.com,components=main+contrib+non-free"})
	if err != nil {
		t.Fatalf("ParseFlags returned error: %v", err)
	}

	want := []string{"main", "contrib", "non-free"}
	if !reflect.DeepEqual(repos[0].Components, want) {
		t.Fatalf("components = %v, want %v", repos[0].Components, want)
	}
}

// TestParseFlagsDistrosWithPlus verifies that distros are correctly split by
// '+' separator.
func TestParseFlagsDistrosWithPlus(t *testing.T) {
	repos, err := ParseFlags([]string{"name=x,url=https://example.com,distros=ubuntu+debian+linuxmint"})
	if err != nil {
		t.Fatalf("ParseFlags returned error: %v", err)
	}

	want := []string{"ubuntu", "debian", "linuxmint"}
	if !reflect.DeepEqual(repos[0].Distros, want) {
		t.Fatalf("distros = %v, want %v", repos[0].Distros, want)
	}
}

// TestParseFlagsMinimalSpec verifies that ParseFlags accepts a minimal
// specification with only required fields.
func TestParseFlagsMinimalSpec(t *testing.T) {
	repos, err := ParseFlags([]string{"name=minimal,url=https://example.com"})
	if err != nil {
		t.Fatalf("ParseFlags returned error: %v", err)
	}

	if repos[0].Name != "minimal" || repos[0].URL != "https://example.com" {
		t.Fatalf("minimal spec failed: %+v", repos[0])
	}

	// Verify optional fields are zero-valued
	if repos[0].Suite != "" || len(repos[0].Components) != 0 || repos[0].GPGCheck {
		t.Fatalf("optional fields not zero-valued: %+v", repos[0])
	}
}

// TestParseFlagsWhitespaceHandling verifies that ParseFlags trims whitespace
// around keys and values.
func TestParseFlagsWhitespaceHandling(t *testing.T) {
	repos, err := ParseFlags([]string{"  name  =  myrepo  ,  url  =  https://example.com  "})
	if err != nil {
		t.Fatalf("ParseFlags returned error: %v", err)
	}

	if repos[0].Name != "myrepo" || repos[0].URL != "https://example.com" {
		t.Fatalf("whitespace not trimmed: %+v", repos[0])
	}
}

// TestSplitPlusTrimsAndDropsEmpty verifies that splitPlus correctly handles
// whitespace and empty entries.
func TestSplitPlusTrimsAndDropsEmpty(t *testing.T) {
	got := splitPlus("  main + contrib +  ")

	want := []string{"main", "contrib"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("splitPlus = %#v, want %#v", got, want)
	}
}

// TestSplitPlusSingleEntry verifies that splitPlus handles a single entry.
func TestSplitPlusSingleEntry(t *testing.T) {
	got := splitPlus("main")

	want := []string{"main"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("splitPlus = %#v, want %#v", got, want)
	}
}

// TestSplitPlusEmpty verifies that splitPlus returns an empty slice for empty input.
func TestSplitPlusEmpty(t *testing.T) {
	got := splitPlus("")

	want := []string{}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("splitPlus = %#v, want %#v", got, want)
	}
}

// TestAppliesTo verifies the appliesTo function correctly filters repos by distro.
func TestAppliesTo(t *testing.T) {
	cases := []struct {
		name   string
		repo   Repo
		distro string
		want   bool
	}{
		{"no filter applies to all", Repo{}, "ubuntu", true},
		{"matching entry passes", Repo{Distros: []string{"debian", "ubuntu"}}, "ubuntu", true},
		{"non-matching entry blocked", Repo{Distros: []string{"debian"}}, "ubuntu", false},
		{"single distro match", Repo{Distros: []string{"fedora"}}, "fedora", true},
		{"single distro no match", Repo{Distros: []string{"fedora"}}, "ubuntu", false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := appliesTo(&c.repo, c.distro); got != c.want {
				t.Fatalf("appliesTo(%+v, %q) = %v, want %v", c.repo, c.distro, got, c.want)
			}
		})
	}
}

// TestFormatForCoversKnownManagers verifies that formatFor returns the correct
// format string for each package manager.
func TestFormatForCoversKnownManagers(t *testing.T) {
	cases := map[string]string{
		constants.PMApt:    formatDeb,
		constants.PMYum:    formatRPM,
		constants.PMZypper: formatRPM,
		constants.PMPacman: "",
		constants.PMApk:    "",
		"unknown":          "",
	}

	for pm, want := range cases {
		t.Run(pm, func(t *testing.T) {
			if got := formatFor(pm); got != want {
				t.Fatalf("formatFor(%q) = %q, want %q", pm, got, want)
			}
		})
	}
}

// TestSetupEmptyRepos verifies that Setup returns nil without error when given
// an empty repo list.
func TestSetupEmptyRepos(t *testing.T) {
	err := Setup("ubuntu", []Repo{})
	if err != nil {
		t.Fatalf("Setup returned error for empty repos: %v", err)
	}
}

// TestSetupUnknownDistro verifies that Setup returns nil (with a warning) when
// given an unknown distro.
func TestSetupUnknownDistro(t *testing.T) {
	repos := []Repo{
		{Name: "test", URL: "https://example.com"},
	}

	err := Setup("unknowndistro", repos)
	if err != nil {
		t.Fatalf("Setup returned error for unknown distro: %v", err)
	}
}

// TestSetupRepoAppliesToAllDistros verifies that a repo with empty Distros
// applies to any distro.
func TestSetupRepoAppliesToAllDistros(t *testing.T) {
	// This test verifies the filtering logic; actual file writes are not tested
	// since they require /etc/ access.
	repo := Repo{
		Name:    "universal",
		URL:     "https://example.com",
		Distros: []string{}, // Empty means applies to all
	}

	// Verify appliesTo returns true for any distro
	if !appliesTo(&repo, "ubuntu") {
		t.Fatalf("repo with empty Distros should apply to ubuntu")
	}

	if !appliesTo(&repo, "fedora") {
		t.Fatalf("repo with empty Distros should apply to fedora")
	}

	if !appliesTo(&repo, "alpine") {
		t.Fatalf("repo with empty Distros should apply to alpine")
	}
}

// TestSetupRepoAppliesToSpecificDistros verifies that a repo with specific
// Distros only applies to matching distros.
func TestSetupRepoAppliesToSpecificDistros(t *testing.T) {
	repo := Repo{
		Name:    "debian-only",
		URL:     "https://example.com",
		Distros: []string{"debian", "ubuntu"},
	}

	if !appliesTo(&repo, "debian") {
		t.Fatalf("repo should apply to debian")
	}

	if !appliesTo(&repo, "ubuntu") {
		t.Fatalf("repo should apply to ubuntu")
	}

	if appliesTo(&repo, "fedora") {
		t.Fatalf("repo should not apply to fedora")
	}
}
