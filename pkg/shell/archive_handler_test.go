package shell

import (
	"archive/zip"
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

func TestArchiveHandler_FallThrough(t *testing.T) {
	dir := t.TempDir()

	// "echo" is not intercepted — should fall through to OS binary
	script := "echo hello > " + filepath.Join(dir, "out.txt")

	if err := runScript(t, dir, script); err != nil {
		t.Fatalf("fall-through command returned error: %v", err)
	}
}
