package apk

import (
	"archive/tar"
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/M0Rf30/yap/v2/pkg/pkgbuild"
)

func createTestPKGBUILD() *pkgbuild.PKGBUILD {
	return &pkgbuild.PKGBUILD{
		PkgName:    "test-package",
		PkgVer:     "1.0.0",
		PkgRel:     "1",
		Arch:       []string{"x86_64"},
		PkgDesc:    "Test package description",
		Maintainer: "test@example.com",
	}
}

func TestNewBuilder(t *testing.T) {
	pkgBuild := createTestPKGBUILD()
	builder := NewBuilder(pkgBuild)

	if builder == nil {
		t.Fatal("NewBuilder returned nil")
	}

	if builder.BaseBuilder == nil {
		t.Fatal("BaseBuilder is nil")
	}
}

func TestBuildPackage(t *testing.T) {
	pkgBuild := createTestPKGBUILD()
	builder := NewBuilder(pkgBuild)

	tempDir, err := os.MkdirTemp("", "apk-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	defer func() { _ = os.RemoveAll(tempDir) }()

	// Create a fake package directory with some content
	pkgDir := filepath.Join(tempDir, "pkg")

	err = os.MkdirAll(pkgDir, 0o755)
	if err != nil {
		t.Fatalf("Failed to create package dir: %v", err)
	}

	builder.PKGBUILD.PackageDir = pkgDir
	builder.PKGBUILD.ArchComputed = "x86_64" // Set computed arch before building

	// Create a test file in the package
	testFile := filepath.Join(pkgDir, "testfile")

	err = os.WriteFile(testFile, []byte("test content"), 0o644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	_, err = builder.BuildPackage(context.Background(), tempDir, "")
	if err != nil {
		t.Errorf("BuildPackage failed: %v", err)
	}

	// Check that the package was created
	// APK format is: name-version-release.arch.apk
	expectedPkg := filepath.Join(tempDir, "test-package-1.0.0-1.x86_64.apk")
	if _, err := os.Stat(expectedPkg); os.IsNotExist(err) {
		t.Errorf("Package file was not created: %s", expectedPkg)
	}
}

func TestPrepareFakeroot(t *testing.T) {
	pkgBuild := createTestPKGBUILD()
	builder := NewBuilder(pkgBuild)

	tempDir, err := os.MkdirTemp("", "apk-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	defer func() { _ = os.RemoveAll(tempDir) }()

	// Create a fake package directory
	pkgDir := filepath.Join(tempDir, "pkg")

	err = os.MkdirAll(pkgDir, 0o755)
	if err != nil {
		t.Fatalf("Failed to create package dir: %v", err)
	}

	builder.PKGBUILD.PackageDir = pkgDir

	err = builder.PrepareFakeroot(context.Background(), tempDir, "")
	if err != nil {
		t.Errorf("PrepareFakeroot failed: %v", err)
	}

	// Build date should be set
	if builder.PKGBUILD.BuildDate == 0 {
		t.Error("BuildDate was not set")
	}

	// Check that .PKGINFO was created
	pkginfoPath := filepath.Join(pkgDir, ".PKGINFO")
	if _, err := os.Stat(pkginfoPath); os.IsNotExist(err) {
		t.Error(".PKGINFO file was not created")
	}
}

func TestPrepareFakerootWithScripts(t *testing.T) {
	pkgBuild := createTestPKGBUILD()
	pkgBuild.PreInst = "echo 'pre-install'"
	pkgBuild.PostInst = "echo 'post-install'"
	builder := NewBuilder(pkgBuild)

	tempDir, err := os.MkdirTemp("", "apk-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	defer func() { _ = os.RemoveAll(tempDir) }()

	// Create a fake package directory
	pkgDir := filepath.Join(tempDir, "pkg")

	err = os.MkdirAll(pkgDir, 0o755)
	if err != nil {
		t.Fatalf("Failed to create package dir: %v", err)
	}

	builder.PKGBUILD.PackageDir = pkgDir

	err = builder.PrepareFakeroot(context.Background(), tempDir, "")
	if err != nil {
		t.Errorf("PrepareFakeroot with scripts failed: %v", err)
	}

	// Check that install script was created
	installPath := filepath.Join(pkgDir, ".install")
	if _, err := os.Stat(installPath); os.IsNotExist(err) {
		t.Error(".install file was not created")
	}
}

func TestPrepare(t *testing.T) {
	// Skip if not running as root and no sudo available (would prompt for password)
	if os.Geteuid() != 0 && os.Getenv("SUDO_USER") == "" && os.Getenv("CI") == "" {
		t.Skip("Skipping Prepare test - requires sudo privileges or CI environment")
	}

	pkgBuild := createTestPKGBUILD()
	builder := NewBuilder(pkgBuild)

	makeDepends := []string{"make", "gcc"}
	err := builder.Prepare(context.Background(), makeDepends, "")
	// This will likely fail since apk isn't installed, but we test the method call
	if err == nil {
		t.Log("Prepare succeeded (unexpected in test environment)")
	}
}

func TestPrepareEnvironment(t *testing.T) {
	// Skip if not running as root and no sudo available (would prompt for password)
	if os.Geteuid() != 0 && os.Getenv("SUDO_USER") == "" && os.Getenv("CI") == "" {
		t.Skip("Skipping PrepareEnvironment test - requires sudo privileges or CI environment")
	}

	pkgBuild := createTestPKGBUILD()
	builder := NewBuilder(pkgBuild)

	err := builder.PrepareEnvironment(context.Background(), false, "")
	// This will likely fail since apk isn't installed, but we test the method call
	if err == nil {
		t.Log("PrepareEnvironment succeeded (unexpected in test environment)")
	}
}

func TestPrepareEnvironmentWithGolang(t *testing.T) {
	// Skip if not running as root and no sudo available (would prompt for password)
	if os.Geteuid() != 0 && os.Getenv("SUDO_USER") == "" && os.Getenv("CI") == "" {
		t.Skip("Skipping PrepareEnvironmentWithGolang test - requires sudo privileges or CI environment")
	}

	pkgBuild := createTestPKGBUILD()
	builder := NewBuilder(pkgBuild)

	err := builder.PrepareEnvironment(context.Background(), true, "")
	// This will likely fail since apk isn't installed, but we test the method call
	if err == nil {
		t.Log("PrepareEnvironment with golang succeeded (unexpected in test environment)")
	}
}

func TestUpdate(t *testing.T) {
	// Skip if not running as root and no sudo available (would prompt for password)
	if os.Geteuid() != 0 && os.Getenv("SUDO_USER") == "" && os.Getenv("CI") == "" {
		t.Skip("Skipping Update test - requires sudo privileges or CI environment")
	}

	// Check if apk is available
	if _, err := exec.LookPath("apk"); err != nil {
		t.Skip("Skipping Update test - apk not found")
	}

	pkgBuild := createTestPKGBUILD()
	builder := NewBuilder(pkgBuild)

	err := builder.Update(context.Background())
	// This will likely fail since apk isn't installed, but we test the method call
	if err == nil {
		t.Log("Update succeeded (unexpected in test environment)")
	}
}

func TestCreateAPKPackage(t *testing.T) {
	pkgBuild := createTestPKGBUILD()
	builder := NewBuilder(pkgBuild)

	tempDir, err := os.MkdirTemp("", "apk-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	defer func() { _ = os.RemoveAll(tempDir) }()

	// Create a package directory with content
	pkgDir := filepath.Join(tempDir, "pkg")

	err = os.MkdirAll(pkgDir, 0o755)
	if err != nil {
		t.Fatalf("Failed to create package dir: %v", err)
	}

	builder.PKGBUILD.PackageDir = pkgDir

	// Create a test file
	testFile := filepath.Join(pkgDir, "testfile")

	err = os.WriteFile(testFile, []byte("test content"), 0o644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	pkgFilePath := filepath.Join(tempDir, "test.apk")

	err = builder.createTarGzWithChecksums(context.Background(), builder.PKGBUILD.PackageDir, pkgFilePath)
	if err != nil {
		t.Errorf("createTarGzWithChecksums failed: %v", err)
	}

	// Check that package file was created
	if _, err := os.Stat(pkgFilePath); os.IsNotExist(err) {
		t.Error("APK package file was not created")
	}
}

func TestCreatePkgInfo(t *testing.T) {
	pkgBuild := createTestPKGBUILD()
	builder := NewBuilder(pkgBuild)

	tempDir, err := os.MkdirTemp("", "apk-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	defer func() { _ = os.RemoveAll(tempDir) }()

	builder.PKGBUILD.PackageDir = tempDir
	builder.PKGBUILD.ArchComputed = "x86_64"

	err = builder.createPkgInfo()
	if err != nil {
		t.Errorf("createPkgInfo failed: %v", err)
	}

	// Check that .PKGINFO was created
	pkginfoPath := filepath.Join(tempDir, ".PKGINFO")
	if _, err := os.Stat(pkginfoPath); os.IsNotExist(err) {
		t.Error(".PKGINFO file was not created")
	}

	// Read and verify content
	content, err := os.ReadFile(pkginfoPath)
	if err != nil {
		t.Fatalf("Failed to read .PKGINFO: %v", err)
	}

	contentStr := string(content)
	if !contains(contentStr, "pkgname = test-package") {
		t.Error(".PKGINFO missing package name")
	}

	if !contains(contentStr, "pkgver = 1.0.0-r1") {
		t.Error(".PKGINFO missing version")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && containsInner(s, substr))
}

func containsInner(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}

	return false
}

func TestCreateInstallScript(t *testing.T) {
	pkgBuild := createTestPKGBUILD()
	pkgBuild.PreInst = "echo 'pre-install'"
	pkgBuild.PostInst = "echo 'post-install'"
	builder := NewBuilder(pkgBuild)

	tempDir, err := os.MkdirTemp("", "apk-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	defer func() { _ = os.RemoveAll(tempDir) }()

	builder.PKGBUILD.PackageDir = tempDir

	err = builder.createInstallScript()
	if err != nil {
		t.Errorf("createInstallScript failed: %v", err)
	}

	// Check that .install was created
	installPath := filepath.Join(tempDir, ".install")
	if _, err := os.Stat(installPath); os.IsNotExist(err) {
		t.Error(".install file was not created")
	}

	// Read and verify content
	content, err := os.ReadFile(installPath)
	if err != nil {
		t.Fatalf("Failed to read .install: %v", err)
	}

	contentStr := string(content)
	if !contains(contentStr, "#!/bin/sh") {
		t.Error(".install missing shebang")
	}

	if !contains(contentStr, "pre_install()") {
		t.Error(".install missing pre_install function")
	}

	if !contains(contentStr, "post_install()") {
		t.Error(".install missing post_install function")
	}
}

func TestApkFileNameInArchive(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "apkfile-*")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}

	defer func() { _ = os.Remove(tmpFile.Name()) }()

	_ = tmpFile.Close()

	info, err := os.Stat(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to stat temp file: %v", err)
	}

	af := apkFile{
		FileInfo:      info,
		diskPath:      tmpFile.Name(),
		nameInArchive: "usr/bin/myprog",
	}

	if got := af.NameInArchive(); got != "usr/bin/myprog" {
		t.Errorf("NameInArchive() = %q, want %q", got, "usr/bin/myprog")
	}
}

func TestApkFileLinkTarget(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "apkfile-*")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}

	defer func() { _ = os.Remove(tmpFile.Name()) }()

	_ = tmpFile.Close()

	info, err := os.Stat(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to stat temp file: %v", err)
	}

	af := apkFile{
		FileInfo:      info,
		diskPath:      tmpFile.Name(),
		nameInArchive: "usr/bin/myprog",
		linkTarget:    "/usr/bin/real",
	}

	if got := af.LinkTarget(); got != "/usr/bin/real" {
		t.Errorf("LinkTarget() = %q, want %q", got, "/usr/bin/real")
	}
}

func TestApkFileOpenRegular(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "apkfile-*")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}

	defer func() { _ = os.Remove(tmpFile.Name()) }()

	content := []byte("hello apk")
	if _, err := tmpFile.Write(content); err != nil {
		t.Fatalf("Failed to write to temp file: %v", err)
	}

	_ = tmpFile.Close()

	info, err := os.Stat(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to stat temp file: %v", err)
	}

	af := apkFile{
		FileInfo:      info,
		diskPath:      tmpFile.Name(),
		nameInArchive: "testfile",
	}

	rc, err := af.Open()
	if err != nil {
		t.Fatalf("Open() returned error: %v", err)
	}

	defer func() { _ = rc.Close() }()

	buf := make([]byte, len(content))
	if _, err := io.ReadFull(rc, buf); err != nil {
		t.Fatalf("ReadFull() returned error: %v", err)
	}

	if !bytes.Equal(buf, content) {
		t.Errorf("Open() content = %q, want %q", string(buf), string(content))
	}
}

func TestApkFileOpenDirectory(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "apkdir-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	defer func() { _ = os.RemoveAll(tmpDir) }()

	info, err := os.Stat(tmpDir)
	if err != nil {
		t.Fatalf("Failed to stat temp dir: %v", err)
	}

	af := apkFile{
		FileInfo:      info,
		diskPath:      tmpDir,
		nameInArchive: "usr/",
	}

	rc, err := af.Open()
	if err != nil {
		t.Fatalf("Open() on directory returned error: %v", err)
	}

	defer func() { _ = rc.Close() }()

	// Should return empty reader
	data, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll() returned error: %v", err)
	}

	if len(data) != 0 {
		t.Errorf("Open() on directory returned %d bytes, want 0", len(data))
	}
}

func TestApkFileOpenSymlink(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "apksym-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	defer func() { _ = os.RemoveAll(tmpDir) }()

	target := filepath.Join(tmpDir, "target")
	if err := os.WriteFile(target, []byte("target content"), 0o644); err != nil {
		t.Fatalf("Failed to create target file: %v", err)
	}

	link := filepath.Join(tmpDir, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("Failed to create symlink: %v", err)
	}

	info, err := os.Lstat(link)
	if err != nil {
		t.Fatalf("Failed to lstat symlink: %v", err)
	}

	af := apkFile{
		FileInfo:      info,
		diskPath:      link,
		nameInArchive: "link",
		linkTarget:    target,
	}

	rc, err := af.Open()
	if err != nil {
		t.Fatalf("Open() on symlink returned error: %v", err)
	}

	defer func() { _ = rc.Close() }()

	// Should return empty reader (symlink is not regular)
	data, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll() returned error: %v", err)
	}

	if len(data) != 0 {
		t.Errorf("Open() on symlink returned %d bytes, want 0", len(data))
	}
}

func TestWalkAPKFilesSymlink(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "apkwalk-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Create a regular file
	regularFile := filepath.Join(tmpDir, "regular")
	if err := os.WriteFile(regularFile, []byte("data"), 0o644); err != nil {
		t.Fatalf("Failed to create regular file: %v", err)
	}

	// Create a symlink pointing to the regular file
	link := filepath.Join(tmpDir, "mylink")
	if err := os.Symlink(regularFile, link); err != nil {
		t.Fatalf("Failed to create symlink: %v", err)
	}

	files, err := walkAPKFiles(tmpDir)
	if err != nil {
		t.Fatalf("walkAPKFiles returned error: %v", err)
	}

	var foundSymlink bool

	for _, f := range files {
		if f.nameInArchive == "mylink" {
			foundSymlink = true

			if f.linkTarget == "" {
				t.Error("symlink entry has empty linkTarget")
			}
		}
	}

	if !foundSymlink {
		t.Error("walkAPKFiles did not return symlink entry")
	}
}

func TestWalkAPKFilesNonExistent(t *testing.T) {
	_, err := walkAPKFiles("/nonexistent/path/that/does/not/exist")
	if err == nil {
		t.Error("walkAPKFiles on non-existent path should return error")
	}
}

func TestWalkAPKFilesEmpty(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "apkempty-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	defer func() { _ = os.RemoveAll(tmpDir) }()

	files, err := walkAPKFiles(tmpDir)
	if err != nil {
		t.Fatalf("walkAPKFiles on empty dir returned error: %v", err)
	}

	if len(files) != 0 {
		t.Errorf("walkAPKFiles on empty dir returned %d entries, want 0", len(files))
	}
}

func TestCreateTarGzWithChecksumsNonExistentDir(t *testing.T) {
	pkgBuild := createTestPKGBUILD()
	builder := NewBuilder(pkgBuild)

	tmpDir, err := os.MkdirTemp("", "apk-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	defer func() { _ = os.RemoveAll(tmpDir) }()

	err = builder.createTarGzWithChecksums(context.Background(), "/nonexistent/source/dir", filepath.Join(tmpDir, "out.apk"))
	if err == nil {
		t.Error("createTarGzWithChecksums with non-existent sourceDir should return error")
	}
}

func TestWriteFileWithChecksumDirectory(t *testing.T) {
	pkgBuild := createTestPKGBUILD()
	builder := NewBuilder(pkgBuild)

	tmpDir, err := os.MkdirTemp("", "apkwfc-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	defer func() { _ = os.RemoveAll(tmpDir) }()

	info, err := os.Stat(tmpDir)
	if err != nil {
		t.Fatalf("Failed to stat temp dir: %v", err)
	}

	af := apkFile{
		FileInfo:      info,
		diskPath:      tmpDir,
		nameInArchive: "usr/",
	}

	var buf bytes.Buffer

	tw := tar.NewWriter(&buf)

	ctx := context.Background()

	if err := builder.writeFileWithChecksum(ctx, tw, af); err != nil {
		t.Errorf("writeFileWithChecksum on directory returned error: %v", err)
	}

	_ = tw.Close()
}

func TestCreateTarGzWithChecksumsWithSubdirAndControlFile(t *testing.T) {
	// Exercises: directory nameInArchive suffix append, isControlFile skip in data pass,
	// and control file write in control pass.
	pkgBuild := createTestPKGBUILD()
	builder := NewBuilder(pkgBuild)

	tmpDir, err := os.MkdirTemp("", "apk-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	defer func() { _ = os.RemoveAll(tmpDir) }()

	pkgDir := filepath.Join(tmpDir, "pkg")
	if err := os.MkdirAll(pkgDir, 0o755); err != nil {
		t.Fatalf("Failed to create pkg dir: %v", err)
	}

	// Create a subdirectory (exercises IsDir branch)
	subDir := filepath.Join(pkgDir, "usr", "bin")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatalf("Failed to create subdir: %v", err)
	}

	// Create a regular data file inside the subdir
	dataFile := filepath.Join(subDir, "myprog")
	if err := os.WriteFile(dataFile, []byte("#!/bin/sh\necho hello"), 0o755); err != nil {
		t.Fatalf("Failed to create data file: %v", err)
	}

	// Create a .PKGINFO control file (exercises control file path in writeFileWithChecksum)
	pkginfoContent := "pkgname = test-package\npkgver = 1.0.0-r1\n"
	if err := os.WriteFile(filepath.Join(pkgDir, ".PKGINFO"), []byte(pkginfoContent), 0o644); err != nil {
		t.Fatalf("Failed to create .PKGINFO: %v", err)
	}

	builder.PKGBUILD.PackageDir = pkgDir

	outFile := filepath.Join(tmpDir, "test.apk")

	if err := builder.createTarGzWithChecksums(context.Background(), pkgDir, outFile); err != nil {
		t.Errorf("createTarGzWithChecksums with subdir and control file failed: %v", err)
	}

	if _, err := os.Stat(outFile); os.IsNotExist(err) {
		t.Error("APK output file was not created")
	}
}

func TestWriteFileWithChecksumCancelledContext(t *testing.T) {
	// Exercises the ctx.Err() early-return branch.
	pkgBuild := createTestPKGBUILD()
	builder := NewBuilder(pkgBuild)

	tmpFile, err := os.CreateTemp("", "apkwfc-*")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}

	defer func() { _ = os.Remove(tmpFile.Name()) }()

	if _, err := tmpFile.WriteString("data"); err != nil {
		t.Fatalf("Failed to write: %v", err)
	}

	_ = tmpFile.Close()

	info, err := os.Stat(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to stat: %v", err)
	}

	af := apkFile{
		FileInfo:      info,
		diskPath:      tmpFile.Name(),
		nameInArchive: "testfile",
	}

	var buf bytes.Buffer

	tw := tar.NewWriter(&buf)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	err = builder.writeFileWithChecksum(ctx, tw, af)
	if err == nil {
		t.Error("writeFileWithChecksum with cancelled context should return error")
	}

	_ = tw.Close()
}

func TestWriteFileWithChecksumEmptyNameInArchive(t *testing.T) {
	// Exercises the hdr.Name == "" fallback to file.Name().
	pkgBuild := createTestPKGBUILD()
	builder := NewBuilder(pkgBuild)

	tmpFile, err := os.CreateTemp("", "apkwfc-*")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}

	defer func() { _ = os.Remove(tmpFile.Name()) }()

	if _, err := tmpFile.WriteString("data"); err != nil {
		t.Fatalf("Failed to write: %v", err)
	}

	_ = tmpFile.Close()

	info, err := os.Stat(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to stat: %v", err)
	}

	// Empty nameInArchive triggers the hdr.Name == "" branch
	af := apkFile{
		FileInfo:      info,
		diskPath:      tmpFile.Name(),
		nameInArchive: "",
	}

	var buf bytes.Buffer

	tw := tar.NewWriter(&buf)
	ctx := context.Background()

	if err := builder.writeFileWithChecksum(ctx, tw, af); err != nil {
		t.Errorf("writeFileWithChecksum with empty nameInArchive returned error: %v", err)
	}

	_ = tw.Close()
}

func TestWriteFileWithChecksumControlFileRegular(t *testing.T) {
	// Exercises the else branch of writeFileWithChecksum: control file that is TypeReg
	// (isControlFile == true, so no SHA1 checksum, but data is still written).
	pkgBuild := createTestPKGBUILD()
	builder := NewBuilder(pkgBuild)

	tmpDir, err := os.MkdirTemp("", "apkwfc-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Create a .PKGINFO file (a control file that is a regular file)
	pkginfoPath := filepath.Join(tmpDir, ".PKGINFO")
	if err := os.WriteFile(pkginfoPath, []byte("pkgname = test\n"), 0o644); err != nil {
		t.Fatalf("Failed to create .PKGINFO: %v", err)
	}

	info, err := os.Stat(pkginfoPath)
	if err != nil {
		t.Fatalf("Failed to stat .PKGINFO: %v", err)
	}

	af := apkFile{
		FileInfo:      info,
		diskPath:      pkginfoPath,
		nameInArchive: ".PKGINFO",
	}

	var buf bytes.Buffer

	tw := tar.NewWriter(&buf)
	ctx := context.Background()

	if err := builder.writeFileWithChecksum(ctx, tw, af); err != nil {
		t.Errorf("writeFileWithChecksum on control file returned error: %v", err)
	}

	_ = tw.Close()

	// Verify something was written to the tar
	if buf.Len() == 0 {
		t.Error("writeFileWithChecksum wrote nothing for control file")
	}
}

func TestWriteFileWithChecksumSymlink(t *testing.T) {
	pkgBuild := createTestPKGBUILD()
	builder := NewBuilder(pkgBuild)

	tmpDir, err := os.MkdirTemp("", "apkwfc-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	defer func() { _ = os.RemoveAll(tmpDir) }()

	target := filepath.Join(tmpDir, "target")
	if err := os.WriteFile(target, []byte("target content"), 0o644); err != nil {
		t.Fatalf("Failed to create target file: %v", err)
	}

	link := filepath.Join(tmpDir, "mylink")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("Failed to create symlink: %v", err)
	}

	info, err := os.Lstat(link)
	if err != nil {
		t.Fatalf("Failed to lstat symlink: %v", err)
	}

	af := apkFile{
		FileInfo:      info,
		diskPath:      link,
		nameInArchive: "mylink",
		linkTarget:    target,
	}

	var buf bytes.Buffer

	tw := tar.NewWriter(&buf)

	ctx := context.Background()

	if err := builder.writeFileWithChecksum(ctx, tw, af); err != nil {
		t.Errorf("writeFileWithChecksum on symlink returned error: %v", err)
	}

	_ = tw.Close()
}
