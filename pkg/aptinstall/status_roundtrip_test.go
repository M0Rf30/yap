// SPDX-FileCopyrightText: 2021-2026 Gianluca Boiano
// SPDX-License-Identifier: GPL-3.0-only

package aptinstall_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/M0Rf30/yap/v2/pkg/aptinstall"
)

const realStatusFile = "testdata/dpkg-status/status"

// stanzaLines splits a status file into its stanzas, each as its sorted lines,
// keyed by "Package:Architecture". Comparing sorted lines leaves out only the
// order of the fields inside a stanza, which writeStatusEntry sets.
func stanzaLines(t *testing.T, data string) map[string][]string {
	t.Helper()

	out := map[string][]string{}

	for stanza := range strings.SplitSeq(strings.TrimSpace(data), "\n\n") {
		lines := strings.Split(stanza, "\n")

		var pkg, arch string

		for _, l := range lines {
			if v, ok := strings.CutPrefix(l, "Package: "); ok {
				pkg = v
			}

			if v, ok := strings.CutPrefix(l, "Architecture: "); ok {
				arch = v
			}
		}

		require.NotEmpty(t, pkg, stanza)

		slices.Sort(lines)

		out[pkg+":"+arch] = lines
	}

	return out
}

// TestStatusRoundTripRealFile reads a real dpkg status file, writes it back and
// checks that nothing is lost or changed: every stanza has the same lines (the
// fields come out in the writer's order, the stanzas sorted by key), and
// writing what was read back gives the same bytes again.
func TestStatusRoundTripRealFile(t *testing.T) {
	original, err := os.ReadFile(realStatusFile)
	require.NoError(t, err)

	entries, err := aptinstall.ReadDpkgStatusFromPathForTesting(realStatusFile)
	require.NoError(t, err)
	require.Len(t, entries, 91)

	dir := t.TempDir()
	first := filepath.Join(dir, "status")

	require.NoError(t, aptinstall.WriteDpkgStatusToPathForTesting(first, entries))

	written, err := os.ReadFile(first)
	require.NoError(t, err)

	assert.Equal(t, stanzaLines(t, string(original)), stanzaLines(t, string(written)))
	assert.Contains(t, string(written), "\nConffiles:\n /", "Conffiles has no space after its colon, as dpkg writes it")
	assert.NotContains(t, string(written), ": \n", "no field ends in a space")

	again, err := aptinstall.ReadDpkgStatusFromPathForTesting(first)
	require.NoError(t, err)
	assert.Equal(t, entries, again)

	second := filepath.Join(dir, "status2")
	require.NoError(t, aptinstall.WriteDpkgStatusToPathForTesting(second, again))

	rewritten, err := os.ReadFile(second)
	require.NoError(t, err)
	assert.Equal(t, string(written), string(rewritten), "the output is stable")
}
