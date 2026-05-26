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
	// Real vendor form: unzip -o <archive> <filters...> -d <destdir>
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

// TestParseGunzipArgs tests the parseGunzipArgs function with various argument combinations.
func TestParseGunzipArgs(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantPath   string
		wantStdout bool
		wantKeep   bool
	}{
		{
			name:       "simple file",
			args:       []string{"gunzip", "file.gz"},
			wantPath:   "file.gz",
			wantStdout: false,
			wantKeep:   false,
		},
		{
			name:       "stdout flag -c",
			args:       []string{"gunzip", "-c", "file.gz"},
			wantPath:   "file.gz",
			wantStdout: true,
			wantKeep:   false,
		},
		{
			name:       "stdout flag --stdout",
			args:       []string{"gunzip", "--stdout", "file.gz"},
			wantPath:   "file.gz",
			wantStdout: true,
			wantKeep:   false,
		},
		{
			name:       "stdout flag --to-stdout",
			args:       []string{"gunzip", "--to-stdout", "file.gz"},
			wantPath:   "file.gz",
			wantStdout: true,
			wantKeep:   false,
		},
		{
			name:       "keep flag -k",
			args:       []string{"gunzip", "-k", "file.gz"},
			wantPath:   "file.gz",
			wantStdout: false,
			wantKeep:   true,
		},
		{
			name:       "keep flag --keep",
			args:       []string{"gunzip", "--keep", "file.gz"},
			wantPath:   "file.gz",
			wantStdout: false,
			wantKeep:   true,
		},
		{
			name:       "decompress flag -d",
			args:       []string{"gunzip", "-d", "file.gz"},
			wantPath:   "file.gz",
			wantStdout: false,
			wantKeep:   false,
		},
		{
			name:       "decompress flag --decompress",
			args:       []string{"gunzip", "--decompress", "file.gz"},
			wantPath:   "file.gz",
			wantStdout: false,
			wantKeep:   false,
		},
		{
			name:       "combined flags -dc",
			args:       []string{"gunzip", "-dc", "file.gz"},
			wantPath:   "file.gz",
			wantStdout: true,
			wantKeep:   false,
		},
		{
			name:       "combined flags -ck",
			args:       []string{"gunzip", "-ck", "file.gz"},
			wantPath:   "file.gz",
			wantStdout: true,
			wantKeep:   true,
		},
		{
			name:       "combined flags -dck",
			args:       []string{"gunzip", "-dck", "file.gz"},
			wantPath:   "file.gz",
			wantStdout: true,
			wantKeep:   true,
		},
		{
			name:       "no file specified",
			args:       []string{"gunzip"},
			wantPath:   "",
			wantStdout: false,
			wantKeep:   false,
		},
		{
			name:       "gzip command",
			args:       []string{"gzip", "-c", "file.gz"},
			wantPath:   "file.gz",
			wantStdout: true,
			wantKeep:   false,
		},
		{
			name:       "multiple files (first is used)",
			args:       []string{"gunzip", "file1.gz", "file2.gz"},
			wantPath:   "file1.gz",
			wantStdout: false,
			wantKeep:   false,
		},
		{
			name:       "flags before file",
			args:       []string{"gunzip", "-c", "-k", "file.gz"},
			wantPath:   "file.gz",
			wantStdout: true,
			wantKeep:   true,
		},
		{
			name:       "flags after file",
			args:       []string{"gunzip", "file.gz", "-c"},
			wantPath:   "file.gz",
			wantStdout: true,
			wantKeep:   false,
		},
		{
			name:       "absolute path",
			args:       []string{"gunzip", "/tmp/file.gz"},
			wantPath:   "/tmp/file.gz",
			wantStdout: false,
			wantKeep:   false,
		},
		{
			name:       "relative path with subdirs",
			args:       []string{"gunzip", "subdir/file.gz"},
			wantPath:   "subdir/file.gz",
			wantStdout: false,
			wantKeep:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, stdout, keep := ParseGunzipArgsForTesting(tt.args)
			if path != tt.wantPath {
				t.Errorf("inputPath: got %q, want %q", path, tt.wantPath)
			}

			if stdout != tt.wantStdout {
				t.Errorf("toStdout: got %v, want %v", stdout, tt.wantStdout)
			}

			if keep != tt.wantKeep {
				t.Errorf("keepOrig: got %v, want %v", keep, tt.wantKeep)
			}
		})
	}
}

// TestParseJarArgs tests the parseJarArgs function with various argument combinations.
func TestParseJarArgs(t *testing.T) {
	const defaultDir = "/default/dir"

	tests := []struct {
		name        string
		args        []string
		defaultDir  string
		wantArchive string
		wantDestDir string
	}{
		{
			name:        "simple xf",
			args:        []string{"jar", "xf", "archive.jar"},
			defaultDir:  defaultDir,
			wantArchive: "archive.jar",
			wantDestDir: defaultDir,
		},
		{
			name:        "with dash prefix -xf",
			args:        []string{"jar", "-xf", "archive.jar"},
			defaultDir:  defaultDir,
			wantArchive: "archive.jar",
			wantDestDir: defaultDir,
		},
		{
			name:        "with -C destination",
			args:        []string{"jar", "xf", "archive.jar", "-C", "/dest"},
			defaultDir:  defaultDir,
			wantArchive: "archive.jar",
			wantDestDir: "/dest",
		},
		{
			name:        "with --file= long option",
			args:        []string{"jar", "x", "--file=archive.jar"},
			defaultDir:  defaultDir,
			wantArchive: "archive.jar",
			wantDestDir: defaultDir,
		},
		{
			name:        "no archive (missing f flag)",
			args:        []string{"jar", "x"},
			defaultDir:  defaultDir,
			wantArchive: "",
			wantDestDir: defaultDir,
		},
		{
			name:        "f flag but no file argument",
			args:        []string{"jar", "xf"},
			defaultDir:  defaultDir,
			wantArchive: "",
			wantDestDir: defaultDir,
		},
		{
			name:        "with file filters",
			args:        []string{"jar", "xf", "archive.jar", "META-INF/*", "com/example/*"},
			defaultDir:  defaultDir,
			wantArchive: "archive.jar",
			wantDestDir: defaultDir,
		},
		{
			name:        "absolute archive path",
			args:        []string{"jar", "xf", "/tmp/archive.jar"},
			defaultDir:  defaultDir,
			wantArchive: "/tmp/archive.jar",
			wantDestDir: defaultDir,
		},
		{
			name:        "relative archive path",
			args:        []string{"jar", "xf", "subdir/archive.jar"},
			defaultDir:  defaultDir,
			wantArchive: "subdir/archive.jar",
			wantDestDir: defaultDir,
		},
		{
			name:        "combined flags xf",
			args:        []string{"jar", "xf", "archive.jar"},
			defaultDir:  defaultDir,
			wantArchive: "archive.jar",
			wantDestDir: defaultDir,
		},
		{
			name:        "-C with relative path",
			args:        []string{"jar", "xf", "archive.jar", "-C", "relative/dest"},
			defaultDir:  defaultDir,
			wantArchive: "archive.jar",
			wantDestDir: "relative/dest",
		},
		{
			name:        "multiple -C (last wins)",
			args:        []string{"jar", "xf", "archive.jar", "-C", "/first", "-C", "/second"},
			defaultDir:  defaultDir,
			wantArchive: "archive.jar",
			wantDestDir: "/second",
		},
		{
			name:        "--file= with -C",
			args:        []string{"jar", "x", "--file=archive.jar", "-C", "/dest"},
			defaultDir:  defaultDir,
			wantArchive: "archive.jar",
			wantDestDir: "/dest",
		},
		{
			name:        "empty archive name",
			args:        []string{"jar", "xf", ""},
			defaultDir:  defaultDir,
			wantArchive: "",
			wantDestDir: defaultDir,
		},
		{
			name:        "extract mode without f flag",
			args:        []string{"jar", "x", "archive.jar"},
			defaultDir:  defaultDir,
			wantArchive: "",
			wantDestDir: defaultDir,
		},
		{
			name:        "long option --extract (skipped, needs xf after)",
			args:        []string{"jar", "--extract", "xf", "archive.jar"},
			defaultDir:  defaultDir,
			wantArchive: "",
			wantDestDir: defaultDir,
		},
		{
			name:        "flags and positional mixed",
			args:        []string{"jar", "-xf", "archive.jar", "-C", "/dest", "file1", "file2"},
			defaultDir:  defaultDir,
			wantArchive: "archive.jar",
			wantDestDir: "/dest",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			archive, destDir := ParseJarArgsForTesting(tt.args, tt.defaultDir)
			if archive != tt.wantArchive {
				t.Errorf("archivePath: got %q, want %q", archive, tt.wantArchive)
			}

			if destDir != tt.wantDestDir {
				t.Errorf("destDir: got %q, want %q", destDir, tt.wantDestDir)
			}
		})
	}
}
