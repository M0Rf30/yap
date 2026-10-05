// SPDX-FileCopyrightText: 2021-2026 Gianluca Boiano
// SPDX-License-Identifier: GPL-3.0-only

//nolint:testpackage // exercises unexported buildArgs/append* helpers
package mcp

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
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
	wantHead := []string{"build", "ubuntu-noble", containerProjectDir}
	if !slices.Equal(got[:3], wantHead) {
		t.Errorf("head = %v, want %v", got[:3], wantHead)
	}

	mustContain := []string{
		"--allow-unverified-repos",
		"--cleanbuild",
		"--skip-sync",
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

func TestInnerDistroTag(t *testing.T) {
	if got := innerDistroTag("ubuntu", ""); got != "ubuntu" {
		t.Errorf("innerDistroTag(bare) = %q, want %q", got, "ubuntu")
	}

	if got := innerDistroTag("ubuntu", "jammy"); got != "ubuntu-jammy" {
		t.Errorf("innerDistroTag(release) = %q, want %q", got, "ubuntu-jammy")
	}
}

// TestBuildCLIArgsFromArgsBareFamilyKeepsGenericSuffix guards the container
// dispatch split: a bare distro family (release == "") MUST still reach the
// inner yap argv bare, so the deb release suffix stays generic ("1ubuntu")
// even though the container image tag dispatched into may be
// release-qualified (e.g. "ubuntu-jammy").
func TestBuildCLIArgsFromArgsBareFamilyKeepsGenericSuffix(t *testing.T) {
	args := &buildArgs{}

	distroTag := innerDistroTag("ubuntu", "")

	got := buildCLIArgsFromArgs(args, distroTag)

	wantHead := []string{"build", "ubuntu", containerProjectDir}
	if !slices.Equal(got[:3], wantHead) {
		t.Errorf("head = %v, want %v", got[:3], wantHead)
	}
}

// TestBuildCLIArgsFromArgsReleaseQualifiedUnchanged pins the existing
// behavior for release-qualified builds: image and inner argv both carry
// the qualified tag, so this must not regress with the image/identity
// split.
func TestBuildCLIArgsFromArgsReleaseQualifiedUnchanged(t *testing.T) {
	args := &buildArgs{}

	distroTag := innerDistroTag("ubuntu", "jammy")

	got := buildCLIArgsFromArgs(args, distroTag)

	wantHead := []string{"build", "ubuntu-jammy", containerProjectDir}
	if !slices.Equal(got[:3], wantHead) {
		t.Errorf("head = %v, want %v", got[:3], wantHead)
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

func TestPrepareCLIArgsCarriesReposAndArch(t *testing.T) {
	got := prepareCLIArgs(&buildArgs{
		TargetArch: "arm64",
		ExtraRepos: []string{"name=a,url=http://x"},
	}, "ubuntu-noble")

	want := []string{
		"prepare", "ubuntu-noble",
		"--repo", "name=a,url=http://x",
		"--target-arch", "arm64",
	}
	if !slices.Equal(got, want) {
		t.Errorf("prepare argv = %v, want %v", got, want)
	}
}

// TestContainerShellCmdQuotesPrepare guards the shell-injection fix: the
// distro tag of the chained prepare step is quoted like the build half.
func TestContainerShellCmdQuotesPrepare(t *testing.T) {
	got := containerShellCmd(&buildArgs{}, "ubuntu-x; touch /pwned", false)

	if strings.Contains(got, "yap prepare ubuntu-x;") {
		t.Errorf("distro tag spliced unquoted: %q", got)
	}

	if strings.Count(got, "'ubuntu-x; touch /pwned'") != 2 {
		t.Errorf("distro tag must be quoted in both halves: %q", got)
	}

	if skip := containerShellCmd(&buildArgs{}, "ubuntu", true); strings.Contains(skip, "prepare") {
		t.Errorf("skipPrepare must omit prepare: %q", skip)
	}
}

func TestValidateContainerTag(t *testing.T) {
	for _, ok := range []string{"", "ubuntu", "ubuntu-noble", "opensuse-leap", "15.5", "a_b"} {
		if err := validateContainerTag("distro", ok); err != nil {
			t.Errorf("%q rejected: %v", ok, err)
		}
	}

	for _, bad := range []string{"x; cmd", "a b", "-flag", "$(id)", "a/b", "a\nb"} {
		if err := validateContainerTag("release", bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestBuildEnvFromArgsUnverifiedRepos(t *testing.T) {
	got := buildEnvFromArgs(&buildArgs{UnverifiedRepos: true})
	if got["YAP_ALLOW_UNVERIFIED_REPOS"] != "1" {
		t.Errorf("env = %v, want YAP_ALLOW_UNVERIFIED_REPOS=1", got)
	}
}

func TestContainerOutputDir(t *testing.T) {
	write := func(dir, name, body string) {
		t.Helper()

		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	pk := t.TempDir()
	write(pk, "PKGBUILD", "pkgname=x\n")

	if got := containerOutputDir(pk); got != pk {
		t.Errorf("PKGBUILD project: got %q, want %q", got, pk)
	}

	rel := t.TempDir()
	write(rel, "yap.json", `{"output":"dist/pkgs"}`)

	if got, want := containerOutputDir(rel), filepath.Join(rel, "dist", "pkgs"); got != want {
		t.Errorf("relative output: got %q, want %q", got, want)
	}

	abs := t.TempDir()
	write(abs, "yap.json", `{"output":"/project/out"}`)

	if got, want := containerOutputDir(abs), filepath.Join(abs, "out"); got != want {
		t.Errorf("mounted absolute output: got %q, want %q", got, want)
	}

	outside := t.TempDir()
	write(outside, "yap.json", `{"output":"/var/tmp/out"}`)

	if got := containerOutputDir(outside); got != "" {
		t.Errorf("unmappable output: got %q, want empty", got)
	}

	if got := containerOutputDir(t.TempDir()); got != "" {
		t.Errorf("no project file: got %q, want empty", got)
	}
}
