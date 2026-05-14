//nolint:testpackage // exercises unexported parsing helpers
package repo

import (
	"reflect"
	"testing"
)

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

func TestParseFlagsAcceptsKeyAlias(t *testing.T) {
	repos, err := ParseFlags([]string{"name=x,url=https://example.com,suite=jammy,key=https://example.com/k.gpg"})
	if err != nil {
		t.Fatalf("ParseFlags returned error: %v", err)
	}

	if repos[0].KeyURL != "https://example.com/k.gpg" {
		t.Fatalf("alias `key` did not populate KeyURL: %+v", repos[0])
	}
}

func TestSplitPlusTrimsAndDropsEmpty(t *testing.T) {
	got := splitPlus("  main + contrib +  ")

	want := []string{"main", "contrib"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("splitPlus = %#v, want %#v", got, want)
	}
}

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
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := appliesTo(&c.repo, c.distro); got != c.want {
				t.Fatalf("appliesTo(%+v, %q) = %v, want %v", c.repo, c.distro, got, c.want)
			}
		})
	}
}

func TestFormatForCoversKnownManagers(t *testing.T) {
	cases := map[string]string{
		"apt":     "deb",
		"yum":     "rpm",
		"zypper":  "rpm",
		"pacman":  "",
		"unknown": "",
	}

	for pm, want := range cases {
		t.Run(pm, func(t *testing.T) {
			if got := formatFor(pm); got != want {
				t.Fatalf("formatFor(%q) = %q, want %q", pm, got, want)
			}
		})
	}
}
