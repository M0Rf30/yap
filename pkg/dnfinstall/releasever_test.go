// SPDX-FileCopyrightText: 2021-2026 Gianluca Boiano
// SPDX-License-Identifier: GPL-3.0-only

package dnfinstall //nolint:testpackage

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const rockyOSRelease = "NAME=\"Rocky Linux\"\nVERSION_ID=\"9.7\"\nID=rocky\n"

// releaseverFixture is a fake host: variable directories, an os-release file
// and an rpm that answers (or fails) as the test says.
type releaseverFixture struct {
	varsDirs []string
	osPath   string
	rpm      commandRunner
	rpmCalls *int
}

func newReleaseverFixture(t *testing.T) releaseverFixture {
	t.Helper()

	root := t.TempDir()
	dnfVars := filepath.Join(root, "dnf-vars")
	yumVars := filepath.Join(root, "yum-vars")

	require.NoError(t, os.MkdirAll(dnfVars, 0o755))
	require.NoError(t, os.MkdirAll(yumVars, 0o755))

	osPath := filepath.Join(root, "os-release")
	require.NoError(t, os.WriteFile(osPath, []byte(rockyOSRelease), 0o600))

	calls := 0

	return releaseverFixture{
		varsDirs: []string{dnfVars, yumVars},
		osPath:   osPath,
		rpmCalls: &calls,
		rpm: func(context.Context, string, ...string) ([]byte, error) {
			calls++

			return nil, errors.New("rpm: command not found")
		},
	}
}

func (f releaseverFixture) resolve(t *testing.T) string {
	t.Helper()

	return resolveReleasever(t.Context(), f.varsDirs, f.osPath, f.rpm)
}

func rpmAnswers(out string) commandRunner {
	return func(context.Context, string, ...string) ([]byte, error) {
		return []byte(out), nil
	}
}

// TestResolveReleaseverFromVarsFile: the dnf variable file beats the rpm
// provide and os-release.
func TestResolveReleaseverFromVarsFile(t *testing.T) {
	f := newReleaseverFixture(t)
	f.rpm = rpmAnswers("system-release(releasever)\t9\n")
	require.NoError(t, os.WriteFile(filepath.Join(f.varsDirs[0], "releasever"), []byte(" 8 \nignored\n"), 0o600))

	assert.Equal(t, "8", f.resolve(t))
}

// TestResolveReleaseverYumVarsDir: /etc/yum/vars is honoured after /etc/dnf/vars.
func TestResolveReleaseverYumVarsDir(t *testing.T) {
	f := newReleaseverFixture(t)
	require.NoError(t, os.WriteFile(filepath.Join(f.varsDirs[1], "releasever"), []byte("10\n"), 0o600))

	assert.Equal(t, "10", f.resolve(t))
}

// TestResolveReleaseverEmptyVarsFileIsSkipped: an empty variable file falls
// through to the next source.
func TestResolveReleaseverEmptyVarsFileIsSkipped(t *testing.T) {
	f := newReleaseverFixture(t)
	f.rpm = rpmAnswers("system-release(releasever)\t9\n")
	require.NoError(t, os.WriteFile(filepath.Join(f.varsDirs[0], "releasever"), []byte("\n"), 0o600))

	assert.Equal(t, "9", f.resolve(t))
}

// TestResolveReleaseverFromRPMProvide: without a variable file the release of
// the "system-release(releasever)" provide wins over the dotted VERSION_ID, the
// Rocky 9.7 case: the mirrors serve 9, 9.7 is gone to the vault.
func TestResolveReleaseverFromRPMProvide(t *testing.T) {
	f := newReleaseverFixture(t)
	f.rpm = func(_ context.Context, name string, args ...string) ([]byte, error) {
		assert.Equal(t, "rpm", name)
		assert.Contains(t, args, "system-release(releasever)")

		return []byte("rocky-release\t9.7-1.1.el9\n" +
			"system-release\t9.7-1.1.el9\n" +
			"system-release(releasever_minor)\t7\n" +
			"system-release(releasever_major)\t9\n" +
			"system-release(releasever)\t9\n"), nil
	}

	assert.Equal(t, "9", f.resolve(t))
}

// TestResolveReleaseverFallsBackToOSRelease: rpm failing, or not listing the
// provide, leaves VERSION_ID.
func TestResolveReleaseverFallsBackToOSRelease(t *testing.T) {
	cases := map[string]commandRunner{
		"rpm fails":           nil,
		"no matching provide": rpmAnswers("rocky-release\t9.7-1.1.el9\n"),
		"empty version":       rpmAnswers("system-release(releasever)\t\n"),
		"empty output":        rpmAnswers(""),
	}

	for name, rpm := range cases {
		t.Run(name, func(t *testing.T) {
			f := newReleaseverFixture(t)
			if rpm != nil {
				f.rpm = rpm
			}

			assert.Equal(t, "9.7", f.resolve(t))

			if rpm == nil {
				assert.Equal(t, 1, *f.rpmCalls)
			}
		})
	}
}

// TestResolveReleaseverNothingKnown: no variable, no provide, no os-release.
func TestResolveReleaseverNothingKnown(t *testing.T) {
	f := newReleaseverFixture(t)
	f.osPath = filepath.Join(t.TempDir(), "missing")

	assert.Empty(t, f.resolve(t))
}

// TestReleaseverFromOSReleaseQuoting: quoted, single-quoted and bare VERSION_ID.
func TestReleaseverFromOSReleaseQuoting(t *testing.T) {
	for in, want := range map[string]string{
		"VERSION_ID=\"40\"\n": "40",
		"VERSION_ID='9.7'\n":  "9.7",
		"VERSION_ID=8.10\n":   "8.10",
		"ID=rocky\n":          "",
	} {
		path := filepath.Join(t.TempDir(), "os-release")
		require.NoError(t, os.WriteFile(path, []byte(in), 0o600))

		assert.Equal(t, want, releaseverFromOSRelease(path), in)
	}
}
