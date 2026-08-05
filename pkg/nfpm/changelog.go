package nfpm

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/M0Rf30/yap/v2/pkg/errors"
)

// rpmChangelogDateLayout formats changelog dates for the RPM %changelog
// dialect: "Wed Jan 01 2025".
const rpmChangelogDateLayout = "Mon Jan 02 2006"

// debianChangelogDateLayout formats changelog dates for the Debian
// changelog(5) trailer: "Wed, 01 Jan 2025 00:00:00 +0000".
const debianChangelogDateLayout = "Mon, 02 Jan 2006 15:04:05 -0700"

// Changelog is the goreleaser/chglog document nfpm's `changelog:` key points
// at: a package name plus an ordered (by Date, newest first once rendered)
// list of release entries.
type Changelog struct {
	// Name is the package name, used as the RenderRPM packager fallback
	// when an entry declares neither its own Packager.
	Name string `yaml:"name,omitempty"`
	// Entries is the list of release entries, one per version.
	Entries []ChangelogEntry `yaml:"entries,omitempty"`
}

// ChangelogEntry is a single release's changelog data.
type ChangelogEntry struct {
	// Semver is the version string embedded in rendered headers verbatim
	// (e.g. "1.0-1" for the RPM dialect).
	Semver string `yaml:"semver,omitempty"`
	// Date is the release date/time.
	Date time.Time `yaml:"date,omitempty"`
	// Packager identifies who cut this release; falls back to Changelog.Name
	// then "unknown" when rendering.
	Packager string `yaml:"packager,omitempty"`
	// Notes holds free-form header/footer lines rendered alongside Changes.
	Notes ChangelogNotes `yaml:"notes,omitempty"`
	// Changes lists the individual change notes for this release.
	Changes []ChangelogChange `yaml:"changes,omitempty"`
	// Deb holds Debian-specific rendering hints for this entry.
	Deb ChangelogDeb `yaml:"deb,omitempty"`
}

// ChangelogNotes holds free-form lines rendered before/after Changes.
type ChangelogNotes struct {
	// Header is rendered as the first change line of the entry.
	Header string `yaml:"header,omitempty"`
	// Footer is rendered as the last change line of the entry.
	Footer string `yaml:"footer,omitempty"`
}

// ChangelogChange is a single change note within a ChangelogEntry.
type ChangelogChange struct {
	// Commit is the source-control commit hash the note was generated from.
	Commit string `yaml:"commit,omitempty"`
	// Note is the human-readable change description.
	Note string `yaml:"note,omitempty"`
	// Author is the change's author.
	Author string `yaml:"author,omitempty"`
}

// ChangelogDeb holds Debian-specific rendering hints for one entry.
type ChangelogDeb struct {
	// Urgency is the Debian changelog urgency field; defaults to "low".
	Urgency string `yaml:"urgency,omitempty"`
	// Distributions lists the target distributions; defaults to the
	// distribution RenderDebian was called with.
	Distributions []string `yaml:"distributions,omitempty"`
}

// LoadChangelog parses the goreleaser/chglog YAML file at path. Unknown
// fields are rejected, matching Parse's strictness.
func LoadChangelog(path string) (*Changelog, error) {
	f, err := os.Open(filepath.Clean(path))
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrTypeFileSystem, "failed to open changelog file").
			WithOperation("LoadChangelog").
			WithContext("path", path)
	}
	defer func() { _ = f.Close() }()

	dec := yaml.NewDecoder(f)
	dec.KnownFields(true)

	var cl Changelog
	if err := dec.Decode(&cl); err != nil {
		return nil, errors.Wrap(err, errors.ErrTypeConfiguration, "failed to parse changelog file").
			WithOperation("LoadChangelog").
			WithContext("path", path)
	}

	return &cl, nil
}

// sortedEntries returns cl.Entries sorted newest-first by Date, without
// mutating cl.
func (cl *Changelog) sortedEntries() []ChangelogEntry {
	entries := make([]ChangelogEntry, len(cl.Entries))
	copy(entries, cl.Entries)

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Date.After(entries[j].Date)
	})

	return entries
}

// entryPackager resolves the packager identity for one entry: its own
// Packager, else the changelog's Name, else "unknown".
func (cl *Changelog) entryPackager(e *ChangelogEntry) string {
	if e.Packager != "" {
		return e.Packager
	}

	if cl.Name != "" {
		return cl.Name
	}

	return "unknown"
}

// RenderRPM emits YAP's canonical changelog text: the RPM %changelog
// dialect parsed by pkg/builders/rpm.parseRPMChangelog:
//
//   - Wed Jan 01 2025 Name <mail> - 1.0-1
//   - change
//
// Entries are newest-first. pkgName is accepted for symmetry with
// RenderDebian (whose header line embeds the package name); the RPM
// %changelog dialect has no per-line use for it, since it always lives
// inside that one package's own spec file.
//
//nolint:unparam // pkgName kept for signature symmetry with RenderDebian; see contract §3.
func (cl *Changelog) RenderRPM(pkgName string) []byte {
	entries := cl.sortedEntries()

	var buf bytes.Buffer

	for i := range entries {
		e := &entries[i]
		if i > 0 {
			buf.WriteByte('\n')
		}

		fmt.Fprintf(&buf, "* %s %s - %s\n",
			e.Date.Format(rpmChangelogDateLayout), cl.entryPackager(e), e.Semver)

		if e.Notes.Header != "" {
			fmt.Fprintf(&buf, "- %s\n", e.Notes.Header)
		}

		for _, change := range e.Changes {
			fmt.Fprintf(&buf, "- %s\n", change.Note)
		}

		if e.Notes.Footer != "" {
			fmt.Fprintf(&buf, "- %s\n", e.Notes.Footer)
		}
	}

	return buf.Bytes()
}

// RenderDebian emits Debian changelog(5) text:
//
//	name (semver) distribution; urgency=low
//
//	  * note
//
//	 -- packager  Wed, 01 Jan 2025 00:00:00 +0000
//
// Entries are newest-first. distribution is used verbatim unless an entry
// declares its own Deb.Distributions; urgency defaults to "low" unless an
// entry declares its own Deb.Urgency.
func (cl *Changelog) RenderDebian(pkgName, distribution string) []byte {
	entries := cl.sortedEntries()

	var buf bytes.Buffer

	for i := range entries {
		e := &entries[i]

		dist := distribution
		if len(e.Deb.Distributions) > 0 {
			dist = strings.Join(e.Deb.Distributions, " ")
		}

		urgency := e.Deb.Urgency
		if urgency == "" {
			urgency = "low"
		}

		fmt.Fprintf(&buf, "%s (%s) %s; urgency=%s\n\n", pkgName, e.Semver, dist, urgency)

		if e.Notes.Header != "" {
			fmt.Fprintf(&buf, "  * %s\n", e.Notes.Header)
		}

		for _, change := range e.Changes {
			fmt.Fprintf(&buf, "  * %s\n", change.Note)
		}

		if e.Notes.Footer != "" {
			fmt.Fprintf(&buf, "  * %s\n", e.Notes.Footer)
		}

		fmt.Fprintf(&buf, "\n -- %s  %s\n\n",
			cl.entryPackager(e), e.Date.Format(debianChangelogDateLayout))
	}

	return bytes.TrimRight(buf.Bytes(), "\n")
}
