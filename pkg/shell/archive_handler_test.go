package shell

import (
	"archive/zip"
	"compress/gzip"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"
)

// createTestZip creates a minimal ZIP archive at path containing one file.
func createTestZip(t *testing.T, path, entryName, entryContent string) {
	t.Helper()

	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("createTestZip: create %s: %v", path, err)
	}

	defer func() {
		if err := f.Close(); err != nil {
			t.Errorf("createTestZip: close: %v", err)
		}
	}()

	w := zip.NewWriter(f)

	fw, err := w.Create(entryName)
	if err != nil {
		t.Fatalf("createTestZip: create entry %s: %v", entryName, err)
	}

	if _, err := fw.Write([]byte(entryContent)); err != nil {
		t.Fatalf("createTestZip: write entry: %v", err)
	}

	if err := w.Close(); err != nil {
		t.Fatalf("createTestZip: close writer: %v", err)
	}
}

// runScript executes a shell script string through the mvdan/sh interpreter
// with the archiveExecHandler wired in, using dir as the working directory.
func runScript(t *testing.T, dir, script string) error {
	t.Helper()

	parsed, err := syntax.NewParser(syntax.Variant(syntax.LangBash)).
		Parse(strings.NewReader(script), "test")
	if err != nil {
		t.Fatalf("runScript: parse: %v", err)
	}

	runner, err := interp.New(
		interp.Dir(dir),
		interp.Env(expand.ListEnviron(os.Environ()...)),
		interp.ExecHandlers(archiveExecHandler),
	)
	if err != nil {
		t.Fatalf("runScript: new runner: %v", err)
	}

	return runner.Run(context.Background(), parsed)
}

func TestArchiveHandler_Unzip(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "test.zip")
	destDir := filepath.Join(dir, "out")

	createTestZip(t, zipPath, "hello.txt", "hello world")

	if err := os.MkdirAll(destDir, 0o755); err != nil {
		t.Fatal(err)
	}

	script := "unzip -o -d " + destDir + " " + zipPath

	if err := runScript(t, dir, script); err != nil {
		t.Fatalf("unzip handler returned error: %v", err)
	}

	extracted := filepath.Join(destDir, "hello.txt")
	if _, err := os.Stat(extracted); err != nil {
		t.Fatalf("expected extracted file %s to exist: %v", extracted, err)
	}
}

func TestArchiveHandler_Unzip_CurrentDir(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "test.zip")

	createTestZip(t, zipPath, "readme.txt", "content")

	// No -d flag — should extract to working directory (dir)
	script := "unzip " + zipPath

	if err := runScript(t, dir, script); err != nil {
		t.Fatalf("unzip handler (no -d) returned error: %v", err)
	}

	extracted := filepath.Join(dir, "readme.txt")
	if _, err := os.Stat(extracted); err != nil {
		t.Fatalf("expected extracted file %s to exist: %v", extracted, err)
	}
}

func TestArchiveHandler_Jar(t *testing.T) {
	dir := t.TempDir()
	jarPath := filepath.Join(dir, "app.jar")

	// JAR files are ZIP archives
	createTestZip(t, jarPath, "META-INF/MANIFEST.MF", "Manifest-Version: 1.0\n")

	script := "jar xf " + jarPath

	if err := runScript(t, dir, script); err != nil {
		t.Fatalf("jar handler returned error: %v", err)
	}

	extracted := filepath.Join(dir, "META-INF", "MANIFEST.MF")
	if _, err := os.Stat(extracted); err != nil {
		t.Fatalf("expected extracted file %s to exist: %v", extracted, err)
	}
}

func TestArchiveHandler_Unzip_WithFilters(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "test.zip")
	destDir := filepath.Join(dir, "out")

	// Create ZIP with two entries: conf/attrs.xml and other/skip.txt
	f, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}

	w := zip.NewWriter(f)
	for _, entry := range []struct{ name, body string }{
		{"conf/attrs.xml", "<attrs/>"},
		{"other/skip.txt", "skip me"},
	} {
		fw, err := w.Create(entry.name)
		if err != nil {
			t.Fatal(err)
		}

		if _, err := fw.Write([]byte(entry.body)); err != nil {
			t.Fatal(err)
		}
	}

	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	if err := os.MkdirAll(destDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Only extract conf/* — other/skip.txt should NOT appear.
	// Real carbonio form: unzip -o <archive> <filters...> -d <destdir>
	script := "unzip -o " + zipPath + " 'conf/*' -d " + destDir

	if err := runScript(t, dir, script); err != nil {
		t.Fatalf("unzip with filter returned error: %v", err)
	}

	if _, err := os.Stat(filepath.Join(destDir, "conf", "attrs.xml")); err != nil {
		t.Fatalf("expected conf/attrs.xml to be extracted: %v", err)
	}

	if _, err := os.Stat(filepath.Join(destDir, "other", "skip.txt")); err == nil {
		t.Fatal("other/skip.txt should NOT have been extracted")
	}
}

func TestArchiveHandler_Gunzip_Stdout(t *testing.T) {
	dir := t.TempDir()
	gzPath := filepath.Join(dir, "data.gz")

	// Create a gzip file
	f, err := os.Create(gzPath)
	if err != nil {
		t.Fatal(err)
	}

	gw := gzip.NewWriter(f)
	if _, err := gw.Write([]byte("hello gunzip")); err != nil {
		t.Fatal(err)
	}

	if err := gw.Close(); err != nil {
		t.Fatal(err)
	}

	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	outPath := filepath.Join(dir, "data.txt")
	script := "gunzip -dc " + gzPath + " > " + outPath

	if err := runScript(t, dir, script); err != nil {
		t.Fatalf("gunzip -dc returned error: %v", err)
	}

	content, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("expected output file %s to exist: %v", outPath, err)
	}

	if string(content) != "hello gunzip" {
		t.Fatalf("unexpected content: %q", string(content))
	}
}

func TestArchiveHandler_FallThrough(t *testing.T) {
	dir := t.TempDir()

	// "echo" is not intercepted — should fall through to OS binary
	script := "echo hello > " + filepath.Join(dir, "out.txt")

	if err := runScript(t, dir, script); err != nil {
		t.Fatalf("fall-through command returned error: %v", err)
	}
}

func TestArchiveHandler_DpkgDeb(t *testing.T) {
	// This test verifies that dpkg-deb -x is intercepted and handled.
	// We use a minimal DEB created by the archive package tests.
	// For now, we'll skip this test as it requires complex DEB creation
	// The actual dpkg-deb handler is tested via integration with archive.ExtractDEB
	t.Skip("dpkg-deb test requires complex DEB creation; covered by archive.ExtractDEB tests")
}

func TestArchiveHandler_Rpm2Cpio(t *testing.T) {
	// This test verifies that rpm2cpio is intercepted and handled.
	// We use a minimal RPM created by the archive package tests.
	// For now, we'll skip this test as it requires complex RPM creation
	// The actual rpm2cpio handler is tested via integration with archive.ExtractRPM
	t.Skip("rpm2cpio test requires complex RPM creation; covered by archive.ExtractRPM tests")
}
