package aptinstall_test

import (
	"archive/tar"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"

	"github.com/M0Rf30/yap/v2/pkg/aptinstall"
)

// makeDataTarGz builds a synthetic data.tar.gz containing one regular
// file at "etc/myconf" with the given body.
func makeDataTarGz(t *testing.T, dir string, body []byte) string {
	t.Helper()

	path := filepath.Join(dir, "data.tar.gz")

	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}

	gw := gzip.NewWriter(f)
	tw := tar.NewWriter(gw)

	hdr := &tar.Header{
		Name:     "./etc/myconf",
		Mode:     0o644,
		Size:     int64(len(body)),
		Typeflag: tar.TypeReg,
	}

	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatal(err)
	}

	if _, err := tw.Write(body); err != nil {
		t.Fatal(err)
	}

	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}

	if err := gw.Close(); err != nil {
		t.Fatal(err)
	}

	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	return path
}

// TestConffilePreservedOnUpgrade is the C-6 regression test. The legacy
// code looked up `"/"+filepath.Base(fullPath)` which never matched any
// real conffile entry — so conffiles were ALWAYS overwritten on upgrade,
// violating dpkg's non-interactive semantics.
func TestConffilePreservedOnUpgrade(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()

	// Pre-existing local edit at destDir/etc/myconf.
	confPath := filepath.Join(tmp, "etc", "myconf")
	if err := os.MkdirAll(filepath.Dir(confPath), 0o755); err != nil {
		t.Fatal(err)
	}

	const localEdit = "USER_EDITED_VALUE=keep-me\n"
	if err := os.WriteFile(confPath, []byte(localEdit), 0o644); err != nil {
		t.Fatal(err)
	}

	// data.tar.gz containing the upstream default for /etc/myconf.
	dataTar := makeDataTarGz(t, tmp, []byte("UPSTREAM_DEFAULT=overwrite-me\n"))

	// The control file's conffiles entry is the absolute path.
	conffiles := []string{"/etc/myconf"}

	if err := aptinstall.ExtractDataTarWithConffilesForTesting(dataTar, tmp, conffiles); err != nil {
		t.Fatalf("extract failed: %v", err)
	}

	got, err := os.ReadFile(confPath)
	if err != nil {
		t.Fatal(err)
	}

	if string(got) != localEdit {
		t.Fatalf("conffile was overwritten!\n  want %q\n  got  %q", localEdit, string(got))
	}
}

// TestConffileCreatedWhenAbsent confirms first-install behaviour: when the
// conffile does not yet exist on disk, the upstream copy is written.
func TestConffileCreatedWhenAbsent(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	dataTar := makeDataTarGz(t, tmp, []byte("FRESH_INSTALL=ok\n"))

	if err := aptinstall.ExtractDataTarWithConffilesForTesting(
		dataTar, tmp, []string{"/etc/myconf"},
	); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(filepath.Join(tmp, "etc", "myconf"))
	if err != nil {
		t.Fatal(err)
	}

	if string(got) != "FRESH_INSTALL=ok\n" {
		t.Fatalf("first-install conffile content wrong: %q", got)
	}
}
