package gensum_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/M0Rf30/yap/v2/pkg/gensum"
)

// writePKGBUILD writes content to a temporary PKGBUILD and returns the dir.
func writePKGBUILD(t *testing.T, content string) string {
	t.Helper()

	dir := t.TempDir()

	err := os.WriteFile(filepath.Join(dir, "PKGBUILD"), []byte(content), 0o644)
	require.NoError(t, err)

	return dir
}

// readPKGBUILD reads the PKGBUILD from dir.
func readPKGBUILD(t *testing.T, dir string) string {
	t.Helper()

	data, err := os.ReadFile(filepath.Join(dir, "PKGBUILD"))
	require.NoError(t, err)

	return string(data)
}

// --- replaceChecksumValues (white-box via exported wrapper) ---

func TestReplaceHashesInBlock_SingleLine(t *testing.T) {
	content := `pkgname=foo
sha256sums=('SKIP')
`
	newHashes := []string{"aabbcc"}

	result, err := gensum.ReplaceChecksumValuesExported(content, "sha256sums", newHashes)
	require.NoError(t, err)
	assert.Contains(t, result, "'aabbcc'")
	assert.NotContains(t, result, "SKIP")
}

func TestReplaceHashesInBlock_MultiLine(t *testing.T) {
	content := `pkgname=foo
sha256sums=(
  'SKIP'
  'SKIP'
)
`
	newHashes := []string{"aaa111", "bbb222"}

	result, err := gensum.ReplaceChecksumValuesExported(content, "sha256sums", newHashes)
	require.NoError(t, err)
	assert.Contains(t, result, "'aaa111'")
	assert.Contains(t, result, "'bbb222'")
	// Indentation preserved
	assert.Contains(t, result, "  'aaa111'")
}

func TestReplaceHashesInBlock_PreservesQuoteStyle(t *testing.T) {
	content := `sha256sums=(
  "SKIP"
  "SKIP"
)
`
	newHashes := []string{"hash1", "hash2"}

	result, err := gensum.ReplaceChecksumValuesExported(content, "sha256sums", newHashes)
	require.NoError(t, err)
	// Double-quote style preserved
	assert.Contains(t, result, `"hash1"`)
	assert.Contains(t, result, `"hash2"`)
}

func TestReplaceHashesInBlock_ArchSpecific(t *testing.T) {
	content := `sha256sums=('SKIP')
sha256sums_x86_64=('SKIP')
sha256sums_aarch64=('SKIP')
`
	newHashes := []string{"newhash"}

	result, err := gensum.ReplaceChecksumValuesExported(content, "sha256sums_x86_64", newHashes)
	require.NoError(t, err)
	// Only the x86_64 block is updated
	assert.Contains(t, result, "sha256sums_x86_64=('newhash')")
	assert.Contains(t, result, "sha256sums=('SKIP')")
	assert.Contains(t, result, "sha256sums_aarch64=('SKIP')")
}

func TestReplaceHashesInBlock_FieldAbsent_Appends(t *testing.T) {
	content := `pkgname=foo
source=('https://example.com/foo.tar.gz')
`
	newHashes := []string{"deadbeef"}

	result, err := gensum.ReplaceChecksumValuesExported(content, "sha256sums", newHashes)
	require.NoError(t, err)
	assert.Contains(t, result, "sha256sums=(")
	assert.Contains(t, result, "'deadbeef'")
}

func TestReplaceHashesInBlock_PreservesOtherContent(t *testing.T) {
	content := `pkgname=foo
pkgver=1.0
source=('https://example.com/foo.tar.gz')
sha256sums=('SKIP')

build() {
  make
}
`
	newHashes := []string{"cafebabe"}

	result, err := gensum.ReplaceChecksumValuesExported(content, "sha256sums", newHashes)
	require.NoError(t, err)
	assert.Contains(t, result, "pkgname=foo")
	assert.Contains(t, result, "pkgver=1.0")
	assert.Contains(t, result, "build() {")
	assert.Contains(t, result, "'cafebabe'")
}

// --- extractArrayBlocks ---

func TestExtractArrayBlocks_Base(t *testing.T) {
	content := `source=('https://example.com/foo.tar.gz')`

	blocks := gensum.ExtractSourceBlocksExported(content)
	require.Contains(t, blocks, "")
	assert.Contains(t, blocks[""], "foo.tar.gz")
}

func TestExtractArrayBlocks_MultiArch(t *testing.T) {
	content := `source=('https://example.com/generic.tar.gz')
source_x86_64=('https://example.com/x86_64.tar.gz')
source_aarch64=('https://example.com/aarch64.tar.gz')
`
	blocks := gensum.ExtractSourceBlocksExported(content)
	assert.Contains(t, blocks, "")
	assert.Contains(t, blocks, "_x86_64")
	assert.Contains(t, blocks, "_aarch64")
}

// --- parseArrayValues ---

func TestParseArrayValues_SingleLine(t *testing.T) {
	block := `source=('https://example.com/foo.tar.gz' 'https://example.com/bar.tar.gz')`
	vals := gensum.ParseArrayValuesExported(block)
	assert.Equal(t, []string{
		"https://example.com/foo.tar.gz",
		"https://example.com/bar.tar.gz",
	}, vals)
}

func TestParseArrayValues_MultiLine(t *testing.T) {
	block := `source=(
  'https://example.com/foo.tar.gz'
  'git+https://github.com/example/repo'
)`
	vals := gensum.ParseArrayValuesExported(block)
	assert.Len(t, vals, 2)
	assert.Equal(t, "https://example.com/foo.tar.gz", vals[0])
	assert.Equal(t, "git+https://github.com/example/repo", vals[1])
}

// --- UpdateChecksums with local file sources ---

func TestUpdateChecksums_LocalFile(t *testing.T) {
	// Create a local source file to hash.
	srcDir := t.TempDir()
	srcFile := filepath.Join(srcDir, "hello.txt")

	err := os.WriteFile(srcFile, []byte("hello yap"), 0o644)
	require.NoError(t, err)

	pkgbuildContent := `pkgname=test
pkgver=1.0
pkgrel=1
arch=('any')
license=('MIT')
source=('` + srcFile + `')
sha256sums=('SKIP')
`
	dir := writePKGBUILD(t, pkgbuildContent)

	err = gensum.UpdateChecksums(dir)
	require.NoError(t, err)

	result := readPKGBUILD(t, dir)
	// Should no longer contain SKIP
	assert.NotContains(t, result, "'SKIP'")
	// Should contain a 64-char hex hash
	for line := range strings.SplitSeq(result, "\n") {
		if strings.Contains(line, "sha256sums") {
			assert.Regexp(t, `[0-9a-f]{64}`, line)
		}
	}
}

func TestUpdateChecksums_VCSSourceKeptAsSkip(t *testing.T) {
	pkgbuildContent := `pkgname=test
pkgver=1.0
pkgrel=1
arch=('any')
license=('MIT')
source=('git+https://github.com/example/repo')
sha256sums=('SKIP')
`
	dir := writePKGBUILD(t, pkgbuildContent)

	err := gensum.UpdateChecksums(dir)
	require.NoError(t, err)

	result := readPKGBUILD(t, dir)
	// VCS source: SKIP must be preserved
	assert.Contains(t, result, "'SKIP'")
}

func TestUpdateChecksums_NoSources(t *testing.T) {
	pkgbuildContent := `pkgname=test
pkgver=1.0
pkgrel=1
arch=('any')
license=('MIT')
`
	dir := writePKGBUILD(t, pkgbuildContent)

	err := gensum.UpdateChecksums(dir)
	require.NoError(t, err)

	// File should be unchanged
	result := readPKGBUILD(t, dir)
	assert.Equal(t, pkgbuildContent, result)
}

// --- extractScalarVars ---

func TestExtractScalarVars_BasicSubstitution(t *testing.T) {
	content := `pkgname=nginx
pkgver=1.25.3
pkgrel=1
source=("https://nginx.org/download/${pkgname}-${pkgver}.tar.gz")
`
	expand := gensum.ExtractScalarVarsExported(content)

	assert.Equal(t, "nginx", expand("pkgname"))
	assert.Equal(t, "1.25.3", expand("pkgver"))
	assert.Equal(t, "1", expand("pkgrel"))
}

func TestExtractScalarVars_QuotedValues(t *testing.T) {
	content := `pkgname="my-package"
pkgver='2.0.0'
`
	expand := gensum.ExtractScalarVarsExported(content)

	assert.Equal(t, "my-package", expand("pkgname"))
	assert.Equal(t, "2.0.0", expand("pkgver"))
}

func TestExtractScalarVars_InlineComment(t *testing.T) {
	// Renovate-style inline comments must not bleed into the value.
	content := `pkgver="1.30.1" # renovate: datasource=github-tags depName=nginx/nginx
pkgname="vendor-nginx"
`
	expand := gensum.ExtractScalarVarsExported(content)

	assert.Equal(t, "1.30.1", expand("pkgver"))
	assert.Equal(t, "vendor-nginx", expand("pkgname"))
}

func TestExtractScalarVars_MissingVarFallsBackToEnv(t *testing.T) {
	t.Setenv("MY_TEST_VAR", "from-env")

	expand := gensum.ExtractScalarVarsExported("")

	assert.Equal(t, "from-env", expand("MY_TEST_VAR"))
	assert.Equal(t, "", expand("NONEXISTENT_VAR_XYZ"))
}
