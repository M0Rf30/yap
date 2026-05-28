//nolint:testpackage // exercises unexported buildArgs/append* helpers
package mcp

import (
	"reflect"
	"slices"
	"testing"
)

func TestBuildEnvFromArgs(t *testing.T) {
	cases := []struct {
		name string
		args buildArgs
		want map[string]string
	}{
		{"no sign", buildArgs{}, nil},
		{"sign no passphrase", buildArgs{Sign: true}, nil},
		{
			"sign with passphrase",
			buildArgs{Sign: true, SignPassphrase: "s3cret"},
			map[string]string{"YAP_SIGN_PASSPHRASE": "s3cret"},
		},
	}

	for _, c := range cases {
		got := buildEnvFromArgs(&c.args)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

func TestBuildCLIArgsFromArgs(t *testing.T) {
	args := &buildArgs{
		TargetArch:      "arm64",
		Parallel:        true,
		CleanBuild:      true,
		SkipSyncDeps:    true,
		Sign:            true,
		SignKey:         "/k.gpg",
		SignKeyName:     "ci",
		SignPassphrase:  "secret",
		SBOM:            true,
		SBOMFormat:      "both",
		OverridePkgVer:  "1.2.3",
		ExtraRepos:      []string{"deb http://repo/ noble main"},
		SkipDeps:        []string{"gcc"},
		UnverifiedRepos: true,
	}

	got := buildCLIArgsFromArgs(args, "ubuntu-noble")

	// Positional preamble.
	wantHead := []string{"build", "ubuntu-noble", "/project"}
	if !slices.Equal(got[:3], wantHead) {
		t.Errorf("head = %v, want %v", got[:3], wantHead)
	}

	mustContain := []string{
		"--allow-unverified-repos",
		"--cleanbuild",
		"--skip-sync-deps",
		"--parallel",
		"--sbom",
		"--target-arch", "arm64",
		"--sbom-format", "both",
		"--pkgver", "1.2.3",
		"--skip-deps", "gcc",
		"--repo", "deb http://repo/ noble main",
		"--sign",
		"--sign-key", "/k.gpg",
		"--sign-key-name", "ci",
	}

	for _, w := range mustContain {
		if !slices.Contains(got, w) {
			t.Errorf("argv missing %q\nfull: %v", w, got)
		}
	}

	// Passphrase MUST NOT appear in argv — it travels via env.
	if slices.Contains(got, "secret") || slices.Contains(got, "--sign-passphrase") {
		t.Errorf("passphrase leaked into argv: %v", got)
	}
}

func TestAppendBoolFlagsSkipsFalse(t *testing.T) {
	got := appendBoolFlags(nil, &buildArgs{})
	if len(got) != 0 {
		t.Errorf("appendBoolFlags on zero args returned %v", got)
	}
}

func TestAppendStringFlagsSkipsEmpty(t *testing.T) {
	got := appendStringFlags(nil, &buildArgs{})
	if len(got) != 0 {
		t.Errorf("appendStringFlags on zero args returned %v", got)
	}
}

func TestAppendSigningFlagsNoopWithoutSign(t *testing.T) {
	got := appendSigningFlags(nil, &buildArgs{SignKey: "/k", SignKeyName: "n"})
	if len(got) != 0 {
		t.Errorf("without Sign, signing flags returned %v", got)
	}
}
