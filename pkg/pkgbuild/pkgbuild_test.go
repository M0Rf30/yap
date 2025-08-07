package pkgbuild

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"text/template"
)

func TestPKGBUILD_Init(t *testing.T) {
	pb := &PKGBUILD{}
	pb.Init()

	if pb.priorities == nil {
		t.Error("Init() did not initialize priorities map")
	}

	// Test FullDistroName assignment without codename
	pb.Distro = "ubuntu"
	pb.Init()

	if pb.FullDistroName != "ubuntu" {
		t.Errorf("Expected FullDistroName 'ubuntu', got '%s'", pb.FullDistroName)
	}

	// Test FullDistroName assignment with codename
	pb.Codename = "focal"
	pb.Init()

	if pb.FullDistroName != "ubuntu_focal" {
		t.Errorf("Expected FullDistroName 'ubuntu_focal', got '%s'", pb.FullDistroName)
	}
}

func TestPKGBUILD_AddItem(t *testing.T) {
	pb := &PKGBUILD{}
	pb.Init()

	// Test adding a simple variable
	err := pb.AddItem("pkgname", "test-package")
	if err != nil {
		t.Errorf("AddItem() returned error: %v", err)
	}

	if pb.PkgName != "test-package" {
		t.Errorf("Expected PkgName 'test-package', got '%s'", pb.PkgName)
	}

	// Test adding an array
	err = pb.AddItem("arch", []string{"x86_64", "any"})
	if err != nil {
		t.Errorf("AddItem() returned error: %v", err)
	}

	if len(pb.Arch) != 2 || pb.Arch[0] != "x86_64" || pb.Arch[1] != "any" {
		t.Errorf("Expected Arch ['x86_64', 'any'], got %v", pb.Arch)
	}

	// Test adding a function
	err = pb.AddItem("build", "make && make install")
	if err != nil {
		t.Errorf("AddItem() returned error: %v", err)
	}

	if pb.Build != "make && make install" {
		t.Errorf("Expected Build 'make && make install', got '%s'", pb.Build)
	}
}

func TestPKGBUILD_ComputeArchitecture(t *testing.T) {
	pb := &PKGBUILD{}

	// Test "any" architecture
	pb.Arch = []string{"any"}
	pb.ComputeArchitecture()

	if pb.ArchComputed != "any" {
		t.Errorf("Expected ArchComputed 'any', got '%s'", pb.ArchComputed)
	}

	// Test with x86_64 (assuming this is the current architecture)
	pb.Arch = []string{"x86_64", "amd64"}
	pb.ComputeArchitecture()
	// ArchComputed should be set to the current architecture if supported
	if pb.ArchComputed == "" {
		t.Error("ArchComputed should not be empty for supported architecture")
	}
}

func TestPKGBUILD_SetMainFolders(t *testing.T) {
	tempDir := t.TempDir()

	pb := &PKGBUILD{
		StartDir:  tempDir,
		SourceDir: tempDir,
		PkgName:   "test-pkg",
		Distro:    "ubuntu",
	}

	// Store original environment
	originalPkgdir := os.Getenv("pkgdir")
	originalSrcdir := os.Getenv("srcdir")
	originalStartdir := os.Getenv("startdir")

	defer func() {
		// Restore original environment
		_ = os.Setenv("pkgdir", originalPkgdir)
		_ = os.Setenv("srcdir", originalSrcdir)
		_ = os.Setenv("startdir", originalStartdir)
	}()

	pb.SetMainFolders()

	// Check environment variables were set
	if os.Getenv("pkgdir") == "" {
		t.Error("pkgdir environment variable not set")
	}

	if os.Getenv("srcdir") != tempDir {
		t.Errorf("Expected srcdir '%s', got '%s'", tempDir, os.Getenv("srcdir"))
	}

	if os.Getenv("startdir") != tempDir {
		t.Errorf("Expected startdir '%s', got '%s'", tempDir, os.Getenv("startdir"))
	}

	// Check PackageDir was set
	if pb.PackageDir == "" {
		t.Error("PackageDir should not be empty")
	}

	// Test Alpine specific behavior
	pb.Distro = "alpine"
	pb.SetMainFolders()

	expectedAlpineDir := filepath.Join(tempDir, "apk", "pkg", "test-pkg")
	if pb.PackageDir != expectedAlpineDir {
		t.Errorf("Expected Alpine PackageDir '%s', got '%s'", expectedAlpineDir, pb.PackageDir)
	}
}

func TestPKGBUILD_ValidateMandatoryItems(t *testing.T) {
	// This test needs to be careful since ValidateMandatoryItems calls os.Exit
	// We'll test the positive case where all mandatory items are present
	pb := &PKGBUILD{
		PkgName: "test-package",
		PkgVer:  "1.0.0",
		PkgRel:  "1",
		PkgDesc: "Test package description",
	}

	// This should not panic or exit if all mandatory items are present
	pb.ValidateMandatoryItems()

	// Test will pass if we reach this point without exiting
}

func TestPKGBUILD_ValidateGeneral(t *testing.T) {
	pb := &PKGBUILD{
		PkgName:   "test-package",
		License:   []string{"MIT"},
		SourceURI: []string{"https://example.com/source.tar.gz"},
		HashSums:  []string{"sha256sum"},
		Package:   "cp file $pkgdir/",
	}

	// This should not panic or exit if validation passes
	pb.ValidateGeneral()

	// Test will pass if we reach this point without exiting
}

func TestPKGBUILD_checkLicense(t *testing.T) {
	pb := &PKGBUILD{}

	// Test PROPRIETARY license
	pb.License = []string{"PROPRIETARY"}
	if !pb.checkLicense() {
		t.Error("PROPRIETARY license should be valid")
	}

	// Test CUSTOM license
	pb.License = []string{"CUSTOM"}
	if !pb.checkLicense() {
		t.Error("CUSTOM license should be valid")
	}

	// Test valid SPDX license
	pb.License = []string{"MIT"}
	if !pb.checkLicense() {
		t.Error("MIT license should be valid")
	}

	// Test multiple valid licenses
	pb.License = []string{"MIT", "GPL-2.0"}
	if !pb.checkLicense() {
		t.Error("Multiple valid licenses should be valid")
	}
}

func TestPKGBUILD_processOptions(t *testing.T) {
	pb := &PKGBUILD{}

	// Test default values
	pb.processOptions()

	if !pb.StaticEnabled {
		t.Error("StaticEnabled should default to true")
	}

	if !pb.StripEnabled {
		t.Error("StripEnabled should default to true")
	}

	// Test disabling strip
	pb.Options = []string{"!strip"}
	pb.processOptions()

	if pb.StripEnabled {
		t.Error("StripEnabled should be false when !strip option is set")
	}

	if !pb.StaticEnabled {
		t.Error("StaticEnabled should remain true")
	}

	// Test disabling staticlibs
	pb.Options = []string{"!staticlibs"}
	pb.processOptions()

	if pb.StaticEnabled {
		t.Error("StaticEnabled should be false when !staticlibs option is set")
	}

	// Reset and test multiple options
	pb.StaticEnabled = true
	pb.StripEnabled = true
	pb.Options = []string{"!strip", "!staticlibs"}
	pb.processOptions()

	if pb.StripEnabled {
		t.Error("StripEnabled should be false")
	}

	if pb.StaticEnabled {
		t.Error("StaticEnabled should be false")
	}
}

func TestPKGBUILD_mapVariables(t *testing.T) {
	pb := &PKGBUILD{}

	// Store original environment
	originalPkgname := os.Getenv("pkgname")
	originalEpoch := os.Getenv("epoch")

	defer func() {
		// Restore original environment
		_ = os.Setenv("pkgname", originalPkgname)
		_ = os.Setenv("epoch", originalEpoch)
	}()

	// Test mapping pkgname
	pb.mapVariables("pkgname", "test-package")

	if pb.PkgName != "test-package" {
		t.Errorf("Expected PkgName 'test-package', got '%s'", pb.PkgName)
	}

	if os.Getenv("pkgname") != "test-package" {
		t.Error("pkgname environment variable not set correctly")
	}

	// Test mapping epoch
	pb.mapVariables("epoch", "1")

	if pb.Epoch != "1" {
		t.Errorf("Expected Epoch '1', got '%s'", pb.Epoch)
	}

	// Test mapping other variables
	pb.mapVariables("pkgver", "2.0.0")

	if pb.PkgVer != "2.0.0" {
		t.Errorf("Expected PkgVer '2.0.0', got '%s'", pb.PkgVer)
	}

	pb.mapVariables("pkgdesc", "Test description")

	if pb.PkgDesc != "Test description" {
		t.Errorf("Expected PkgDesc 'Test description', got '%s'", pb.PkgDesc)
	}
}

func TestPKGBUILD_mapArrays(t *testing.T) {
	pb := &PKGBUILD{}

	// Test mapping arch
	pb.mapArrays("arch", []string{"x86_64", "any"})

	if len(pb.Arch) != 2 || pb.Arch[0] != "x86_64" || pb.Arch[1] != "any" {
		t.Errorf("Expected Arch ['x86_64', 'any'], got %v", pb.Arch)
	}

	// Test mapping depends
	pb.mapArrays("depends", []string{"glibc", "gcc"})

	if len(pb.Depends) != 2 || pb.Depends[0] != "glibc" || pb.Depends[1] != "gcc" {
		t.Errorf("Expected Depends ['glibc', 'gcc'], got %v", pb.Depends)
	}

	// Test mapping source
	pb.mapArrays("source", []string{"https://example.com/source.tar.gz"})

	if len(pb.SourceURI) != 1 || pb.SourceURI[0] != "https://example.com/source.tar.gz" {
		t.Errorf("Expected SourceURI ['https://example.com/source.tar.gz'], got %v", pb.SourceURI)
	}

	// Test mapping sha256sums
	pb.mapArrays("sha256sums", []string{"abcd1234"})

	if len(pb.HashSums) != 1 || pb.HashSums[0] != "abcd1234" {
		t.Errorf("Expected HashSums ['abcd1234'], got %v", pb.HashSums)
	}
}

func TestPKGBUILD_mapFunctions(t *testing.T) {
	pb := &PKGBUILD{}

	// Test mapping build function
	pb.mapFunctions("build", "make")

	if pb.Build != "make" {
		t.Errorf("Expected Build 'make', got '%s'", pb.Build)
	}

	// Test mapping package function
	pb.mapFunctions("package", "make install DESTDIR=$pkgdir")

	if pb.Package != "make install DESTDIR=$pkgdir" {
		t.Errorf("Expected Package 'make install DESTDIR=$pkgdir', got '%s'", pb.Package)
	}

	// Test mapping prepare function
	pb.mapFunctions("prepare", "patch -p1 < fix.patch")

	if pb.Prepare != "patch -p1 < fix.patch" {
		t.Errorf("Expected Prepare 'patch -p1 < fix.patch', got '%s'", pb.Prepare)
	}

	// Test mapping scriptlets
	pb.mapFunctions("preinst", "echo pre-install")

	if pb.PreInst != "echo pre-install" {
		t.Errorf("Expected PreInst 'echo pre-install', got '%s'", pb.PreInst)
	}

	pb.mapFunctions("postinst", "echo post-install")

	if pb.PostInst != "echo post-install" {
		t.Errorf("Expected PostInst 'echo post-install', got '%s'", pb.PostInst)
	}
}

func TestPKGBUILD_parseDirective(t *testing.T) {
	pb := &PKGBUILD{
		FullDistroName: "ubuntu_focal",
		Distro:         "ubuntu",
	}
	pb.Init()

	// Test simple key without directive
	key, priority, err := pb.parseDirective("pkgname")
	if err != nil {
		t.Errorf("parseDirective() returned error: %v", err)
	}

	if key != "pkgname" {
		t.Errorf("Expected key 'pkgname', got '%s'", key)
	}

	if priority != 0 {
		t.Errorf("Expected priority 0, got %d", priority)
	}

	// Test directive with some suffix - the actual logic depends on constants
	// so let's just test that the parsing works correctly
	key, _, err = pb.parseDirective("depends__some_suffix")
	if err != nil {
		t.Errorf("parseDirective() returned error: %v", err)
	}

	if key != "depends" {
		t.Errorf("Expected key 'depends', got '%s'", key)
	}
	// Priority will be -1 if the suffix doesn't match known patterns
	// This is expected behavior

	// Test invalid directive (too many underscores)
	_, _, err = pb.parseDirective("depends__ubuntu__extra")
	if err == nil {
		t.Error("parseDirective() should return error for invalid directive")
	}
}

func TestPKGBUILD_CreateSpec(t *testing.T) {
	tempDir := t.TempDir()
	specFile := filepath.Join(tempDir, "test.spec")

	pb := &PKGBUILD{
		PkgName: "test-package",
		PkgVer:  "1.0.0",
		PkgDesc: "Test package",
	}

	// Create a simple template
	tmplText := `Name: {{.PkgName}}
Version: {{.PkgVer}}
Summary: {{.PkgDesc}}`

	tmpl := template.Must(template.New("test").Parse(tmplText))

	// Test creating spec file
	err := pb.CreateSpec(specFile, tmpl)
	if err != nil {
		t.Errorf("CreateSpec() returned error: %v", err)
	}

	// Verify file was created and has correct content
	content, err := os.ReadFile(specFile)
	if err != nil {
		t.Errorf("Failed to read spec file: %v", err)
	}

	expectedContent := `Name: test-package
Version: 1.0.0
Summary: Test package`

	if strings.TrimSpace(string(content)) != expectedContent {
		t.Errorf("Spec file content mismatch.\nExpected:\n%s\nGot:\n%s", expectedContent, string(content))
	}
}

func TestPKGBUILD_RenderSpec(t *testing.T) {
	pb := &PKGBUILD{}

	script := `{{join .Items}} - {{multiline .Description}}`

	tmpl := pb.RenderSpec(script)
	if tmpl == nil {
		t.Error("RenderSpec() returned nil template")
	}

	// Test that custom functions are available
	data := struct {
		Items       []string
		Description string
	}{
		Items:       []string{"item1", "item2", "item3"},
		Description: "Line 1\nLine 2\nLine 3",
	}

	var buf strings.Builder

	err := tmpl.Execute(&buf, data)
	if err != nil {
		t.Errorf("Template execution failed: %v", err)
	}

	result := buf.String()
	if !strings.Contains(result, "item1, item2, item3") {
		t.Error("join function did not work correctly")
	}

	if !strings.Contains(result, "Line 1\n Line 2\n Line 3") {
		t.Error("multiline function did not work correctly")
	}
}

func TestPKGBUILD_Integration(t *testing.T) {
	tempDir := t.TempDir()

	pb := &PKGBUILD{
		StartDir:  tempDir,
		SourceDir: tempDir,
		Distro:    "ubuntu",
		Codename:  "focal",
	}
	pb.Init()

	// Test complete workflow
	err := pb.AddItem("pkgname", "integration-test")
	if err != nil {
		t.Errorf("AddItem failed: %v", err)
	}

	err = pb.AddItem("pkgver", "1.0.0")
	if err != nil {
		t.Errorf("AddItem failed: %v", err)
	}

	err = pb.AddItem("pkgrel", "1")
	if err != nil {
		t.Errorf("AddItem failed: %v", err)
	}

	err = pb.AddItem("pkgdesc", "Integration test package")
	if err != nil {
		t.Errorf("AddItem failed: %v", err)
	}

	err = pb.AddItem("arch", []string{"x86_64"})
	if err != nil {
		t.Errorf("AddItem failed: %v", err)
	}

	err = pb.AddItem("license", []string{"MIT"})
	if err != nil {
		t.Errorf("AddItem failed: %v", err)
	}

	err = pb.AddItem("source", []string{"https://example.com/source.tar.gz"})
	if err != nil {
		t.Errorf("AddItem failed: %v", err)
	}

	err = pb.AddItem("sha256sums", []string{"abcd1234"})
	if err != nil {
		t.Errorf("AddItem failed: %v", err)
	}

	err = pb.AddItem("package", "make install DESTDIR=$pkgdir")
	if err != nil {
		t.Errorf("AddItem failed: %v", err)
	}

	// Test architecture computation
	pb.ComputeArchitecture()

	// Test folder setup
	pb.SetMainFolders()

	// Verify all fields are set correctly
	if pb.PkgName != "integration-test" {
		t.Error("PkgName not set correctly")
	}

	if pb.FullDistroName != "ubuntu_focal" {
		t.Error("FullDistroName not set correctly")
	}

	if pb.Package != "make install DESTDIR=$pkgdir" {
		t.Error("Package function not set correctly")
	}

	// Test validation (should not panic)
	pb.ValidateMandatoryItems()
	pb.ValidateGeneral()
}
