package dnfcache_test

import (
	"testing"

	"github.com/M0Rf30/yap/v2/pkg/dnfcache"
)

func TestParseRepoFileContent(t *testing.T) {
	content := `
[base]
baseurl=http://mirror.example.com/centos/8/BaseOS/x86_64/os/
enabled=1

[extras]
baseurl=http://mirror.example.com/centos/8/extras/x86_64/os/
enabled=0
`

	repos := dnfcache.ParseRepoFileContent(content)

	if len(repos) != 2 {
		t.Fatalf("expected 2 repos, got %d", len(repos))
	}

	if repos[0].ID != "base" {
		t.Errorf("expected ID=base, got %s", repos[0].ID)
	}

	if !repos[0].Enabled {
		t.Error("expected base to be enabled")
	}

	if repos[1].Enabled {
		t.Error("expected extras to be disabled")
	}
}

func TestStripRPMConstraint(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"glibc >= 2.17", "glibc"},
		{"libfoo", "libfoo"},
		{"rpmlib(CompressedFileNames)", "rpmlib(CompressedFileNames)"},
		{"  foo  ", "foo"},
	}

	for _, c := range cases {
		got := dnfcache.StripRPMConstraint(c.in)
		if got != c.want {
			t.Errorf("StripRPMConstraint(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
