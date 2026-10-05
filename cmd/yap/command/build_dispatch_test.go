// SPDX-FileCopyrightText: 2021-2026 Gianluca Boiano
// SPDX-License-Identifier: GPL-3.0-only

package command

import (
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// resetBuildFlags restores every build flag to its default and clears the
// Changed marker so tests do not leak state into each other.
func resetBuildFlags(t *testing.T) {
	t.Helper()

	_ = buildCmd.LocalFlags() // merges persistent flags (pkgver/pkgrel) into Flags()

	t.Cleanup(func() {
		buildCmd.Flags().VisitAll(func(f *pflag.Flag) {
			if sv, ok := f.Value.(pflag.SliceValue); ok {
				_ = sv.Replace(nil)
			} else {
				_ = f.Value.Set(f.DefValue)
			}

			f.Changed = false
		})
	})
}

func TestForwardedBuildFlags_ReplaysChangedFlags(t *testing.T) {
	resetBuildFlags(t)

	for name, val := range map[string]string{
		"sign":            "true",
		"sign-key":        "/keys/k.gpg",
		"sign-passphrase": "s3cret",
		"ssh-password":    "pw",
		"no-container":    "true",
		"sbom":            "true",
		"pkgver":          "1.2.3",
		"zap":             "true",
	} {
		require.NoError(t, buildCmd.Flags().Set(name, val))
	}

	require.NoError(t, buildCmd.Flags().Set("repo", "name=a,url=http://x"))
	require.NoError(t, buildCmd.Flags().Set("repo", "name=b,url=http://y"))

	got := forwardedBuildFlags()

	assert.ElementsMatch(t, []string{
		"--sign=true",
		"--sign-key=/keys/k.gpg",
		"--sbom=true",
		"--pkgver=1.2.3",
		"--zap=true",
		"--repo=name=a,url=http://x",
		"--repo=name=b,url=http://y",
	}, got)

	// Secrets and host-only flags must never reach the container argv.
	for _, a := range got {
		assert.NotContains(t, a, "s3cret")
		assert.NotContains(t, a, "ssh-password")
		assert.NotContains(t, a, "no-container")
	}
}

func TestForwardedBuildFlags_NothingChanged(t *testing.T) {
	resetBuildFlags(t)

	assert.Empty(t, forwardedBuildFlags())
}

func TestForwardedBuildFlagsFor_RewritesKeyPath(t *testing.T) {
	resetBuildFlags(t)

	require.NoError(t, buildCmd.Flags().Set("sign-key", "/work/proj/keys/k.gpg"))

	assert.Equal(t, []string{"--sign-key=/project/keys/k.gpg"},
		forwardedBuildFlagsFor(buildCmd, "/work/proj"))
	assert.Equal(t, []string{"--sign-key=/work/proj/keys/k.gpg"},
		forwardedBuildFlagsFor(buildCmd, "/other"))
}

func TestForwardedBuildEnv(t *testing.T) {
	origSign, origPass := sign, signPassphrase

	t.Cleanup(func() { sign, signPassphrase = origSign, origPass })

	t.Setenv("YAP_SIGN_PASSPHRASE", "")

	sign, signPassphrase = false, "x"

	assert.Nil(t, forwardedBuildEnv())

	sign, signPassphrase = true, "pw"

	assert.Equal(t, map[string]string{"YAP_SIGN_PASSPHRASE": "pw"}, forwardedBuildEnv())

	sign, signPassphrase = true, ""

	assert.Nil(t, forwardedBuildEnv())
}

func TestPreparePassthroughFlags(t *testing.T) {
	origGo, origArch := GoLang, TargetArch
	origSkip, origTC := prepareSkipSyncDeps, prepareSkipToolchainValidation
	origRepos := prepareExtraRepos

	t.Cleanup(func() {
		GoLang, TargetArch = origGo, origArch
		prepareSkipSyncDeps, prepareSkipToolchainValidation = origSkip, origTC
		prepareExtraRepos = origRepos
	})

	GoLang, TargetArch = true, "arm64"
	prepareSkipSyncDeps, prepareSkipToolchainValidation = true, true
	prepareExtraRepos = []string{"name=a,url=u"}

	assert.Equal(t, []string{
		"--golang", "--target-arch", "arm64", "--repo", "name=a,url=u",
		"--skip-sync", "--skip-toolchain-validation",
	}, preparePassthroughFlags())
}
