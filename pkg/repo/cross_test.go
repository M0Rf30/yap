//nolint:testpackage // exercises unexported helpers (patchDeb822File, portsURIFor, ...)
package repo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPortsURIForKnownDistros(t *testing.T) {
	cases := map[string]string{
		"ubuntu":  "http://ports.ubuntu.com/ubuntu-ports/",
		"debian":  "http://deb.debian.org/debian-ports/",
		"rocky":   "",
		"unknown": "",
	}

	for distro, want := range cases {
		t.Run(distro, func(t *testing.T) {
			if got := portsURIFor(distro); got != want {
				t.Fatalf("portsURIFor(%q) = %q, want %q", distro, got, want)
			}
		})
	}
}

func TestArchiveKeyringForKnownDistros(t *testing.T) {
	cases := map[string]string{
		"ubuntu":  "/usr/share/keyrings/ubuntu-archive-keyring.gpg",
		"debian":  "/usr/share/keyrings/debian-archive-keyring.gpg",
		"rocky":   "",
		"unknown": "",
	}

	for distro, want := range cases {
		t.Run(distro, func(t *testing.T) {
			if got := archiveKeyringFor(distro); got != want {
				t.Fatalf("archiveKeyringFor(%q) = %q, want %q", distro, got, want)
			}
		})
	}
}

func TestPatchDeb822FileAddsArchitecturesOnce(t *testing.T) {
	const original = `Types: deb
URIs: http://archive.ubuntu.com/ubuntu/
Suites: noble noble-updates
Components: main
`

	path := writeTemp(t, "sample.sources", original)

	if err := patchDeb822File(path, "amd64"); err != nil {
		t.Fatalf("first patch returned error: %v", err)
	}

	first := readFile(t, path)
	if !strings.Contains(first, "Architectures: amd64") {
		t.Fatalf("expected `Architectures: amd64` in patched file, got:\n%s", first)
	}

	// Running again must be a no-op so reruns of `yap prepare`/`yap build`
	// stay idempotent on caches that already saw the constraint.
	if err := patchDeb822File(path, "amd64"); err != nil {
		t.Fatalf("second patch returned error: %v", err)
	}

	second := readFile(t, path)
	if first != second {
		t.Fatalf("idempotency broken:\nfirst:\n%s\nsecond:\n%s", first, second)
	}

	if strings.Count(second, "Architectures:") != 1 {
		t.Fatalf("expected exactly one Architectures: line, got:\n%s", second)
	}
}

func TestPatchDeb822FileLeavesUnrelatedStanzasUntouched(t *testing.T) {
	const original = `# comment kept verbatim
Types: deb-src
URIs: http://archive.ubuntu.com/ubuntu/
Suites: noble
Components: main
`

	path := writeTemp(t, "src.sources", original)

	if err := patchDeb822File(path, "amd64"); err != nil {
		t.Fatalf("patch returned error: %v", err)
	}

	patched := readFile(t, path)
	if !strings.HasPrefix(patched, "# comment kept verbatim\n") {
		t.Fatalf("comment header was not preserved:\n%s", patched)
	}
}

func writeTemp(t *testing.T, name, content string) string {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, name)

	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("seed file %s: %v", path, err)
	}

	return path
}

func readFile(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	return string(data)
}
