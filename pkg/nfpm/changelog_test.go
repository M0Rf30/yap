package nfpm_test

import (
	"bufio"
	"bytes"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/M0Rf30/yap/v2/pkg/nfpm"
)

func TestLoadChangelog_Fixture(t *testing.T) {
	cl, err := nfpm.LoadChangelog("testdata/changelog.yaml")
	require.NoError(t, err)

	assert.Equal(t, "examplepkg", cl.Name)
	require.Len(t, cl.Entries, 2)

	first := cl.Entries[0]
	assert.Equal(t, "1.2.3-3", first.Semver)
	assert.True(t, first.Date.Equal(time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)))
	assert.Equal(t, "Release Bot <release@example.com>", first.Packager)
	assert.Equal(t, "Highlights", first.Notes.Header)
	assert.Equal(t, "See https://example.com/changes for details.", first.Notes.Footer)
	require.Len(t, first.Changes, 2)
	assert.Equal(t, "Fix crash on startup", first.Changes[0].Note)
	assert.Equal(t, "abc1234", first.Changes[0].Commit)
	assert.Equal(t, "Jane Doe", first.Changes[0].Author)
	assert.Equal(t, "medium", first.Deb.Urgency)
	assert.Equal(t, []string{"unstable", "experimental"}, first.Deb.Distributions)

	second := cl.Entries[1]
	assert.Equal(t, "1.0.0-1", second.Semver)
	assert.Empty(t, second.Packager)
}

func TestLoadChangelog_MissingFile(t *testing.T) {
	_, err := nfpm.LoadChangelog("testdata/does-not-exist.yaml")
	require.Error(t, err)
}

func TestLoadChangelog_UnknownFieldRejected(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/changelog.yaml"
	require.NoError(t, os.WriteFile(path, []byte("name: x\nbogus_field: 1\n"), 0o644))

	_, err := nfpm.LoadChangelog(path)
	require.Error(t, err)
}

// rpmChangelogHeaderLines returns every header line ("* ...") and confirms
// every non-header, non-blank line is a change line ("- ..."), mirroring
// the two rules pkg/builders/rpm.parseRPMChangelog relies on.
func rpmChangelogHeaderLines(t *testing.T, data []byte) []string {
	t.Helper()

	var headers []string

	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := scanner.Text()

		switch {
		case line == "":
			continue
		case strings.HasPrefix(line, "* "):
			headers = append(headers, line)
		case strings.HasPrefix(line, "- "):
			// change line, fine.
		default:
			t.Fatalf("line does not match the RPM %%changelog dialect (header %q or change %q): %q",
				"* ", "- ", line)
		}
	}

	require.NoError(t, scanner.Err())

	return headers
}

func TestRenderRPM_ParseableShapeAndOrder(t *testing.T) {
	cl, err := nfpm.LoadChangelog("testdata/changelog.yaml")
	require.NoError(t, err)

	out := cl.RenderRPM("examplepkg")

	headers := rpmChangelogHeaderLines(t, out)
	require.Len(t, headers, 2)
	// Newest entry (2025-01-15) first.
	assert.Equal(t, "* Wed Jan 15 2025 Release Bot <release@example.com> - 1.2.3-3", headers[0])
	assert.Equal(t, "* Sat Jun 01 2024 examplepkg - 1.0.0-1", headers[1])

	text := string(out)
	assert.Contains(t, text, "- Highlights\n")
	assert.Contains(t, text, "- Fix crash on startup\n")
	assert.Contains(t, text, "- Improve startup time\n")
	assert.Contains(t, text, "- See https://example.com/changes for details.\n")
	assert.Contains(t, text, "- Initial release\n")

	// The first entry's changes must appear before the second entry's
	// header, proving newest-first ordering (not just header order).
	firstChangeIdx := strings.Index(text, "- Fix crash on startup")
	secondHeaderIdx := strings.Index(text, "* Sat Jun 01 2024")
	assert.Less(t, firstChangeIdx, secondHeaderIdx)
}

func TestChangelogRenderRPM_PackagerFallback(t *testing.T) {
	cl := &nfpm.Changelog{
		Entries: []nfpm.ChangelogEntry{
			{Semver: "1.0-1", Date: time.Date(2025, 3, 4, 0, 0, 0, 0, time.UTC)},
		},
	}

	// No Changelog.Name, no entry.Packager: falls back to "unknown".
	out := string(cl.RenderRPM("pkg"))
	assert.Contains(t, out, "* Tue Mar 04 2025 unknown - 1.0-1")

	cl.Name = "Changelog Name <cl@example.com>"
	out = string(cl.RenderRPM("pkg"))
	assert.Contains(t, out, "* Tue Mar 04 2025 Changelog Name <cl@example.com> - 1.0-1")

	cl.Entries[0].Packager = "Entry Packager <entry@example.com>"
	out = string(cl.RenderRPM("pkg"))
	assert.Contains(t, out, "* Tue Mar 04 2025 Entry Packager <entry@example.com> - 1.0-1")
}

func TestChangelogRenderDebian_Shape(t *testing.T) {
	cl, err := nfpm.LoadChangelog("testdata/changelog.yaml")
	require.NoError(t, err)

	out := string(cl.RenderDebian("examplepkg", "stable"))

	// First entry: per-entry distributions/urgency override the call args.
	assert.Contains(t, out, "examplepkg (1.2.3-3) unstable experimental; urgency=medium\n")
	assert.Contains(t, out, "  * Highlights\n")
	assert.Contains(t, out, "  * Fix crash on startup\n")
	assert.Contains(t, out, "  * Improve startup time\n")
	assert.Contains(t, out, "  * See https://example.com/changes for details.\n")
	assert.Contains(t, out, " -- Release Bot <release@example.com>  Wed, 15 Jan 2025 10:00:00 +0000")

	// Second entry: no per-entry override, falls back to the call
	// argument distribution and the "low" default urgency, and "unknown"
	// packager fallback (Changelog.Name is set, so that wins over
	// "unknown").
	assert.Contains(t, out, "examplepkg (1.0.0-1) stable; urgency=low\n")
	assert.Contains(t, out, "  * Initial release\n")
	assert.Contains(t, out, " -- examplepkg  Sat, 01 Jun 2024 08:30:00 +0000")

	// Newest-first ordering.
	assert.Less(t, strings.Index(out, "1.2.3-3"), strings.Index(out, "1.0.0-1"))
}

func TestChangelogRenderDebian_Empty(t *testing.T) {
	cl := &nfpm.Changelog{}
	out := cl.RenderDebian("pkg", "stable")
	assert.Empty(t, out)
}

func TestChangelogRenderRPM_Empty(t *testing.T) {
	cl := &nfpm.Changelog{}
	out := cl.RenderRPM("pkg")
	assert.Empty(t, out)
}
