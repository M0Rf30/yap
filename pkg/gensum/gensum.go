// Package gensum downloads PKGBUILD sources and rewrites the checksum arrays
// in-place, matching the behaviour of makepkg's updpkgsums utility.
//
// For each source=() / source_<arch>=() block in the PKGBUILD:
//   - git+… entries keep their existing value (SKIP or a commit hash).
//   - All other entries (http/https/ftp/file) are downloaded to a temporary
//     directory and hashed with SHA-256.
//
// The rewrite preserves the original formatting exactly: single-line vs
// multi-line, quote characters (' or "), indentation, and spacing.
// Only the hash values themselves are replaced; everything else is untouched.
package gensum

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	mvdanshell "mvdan.cc/sh/v3/shell"

	"github.com/M0Rf30/yap/v2/pkg/constants"
	"github.com/M0Rf30/yap/v2/pkg/download"
	"github.com/M0Rf30/yap/v2/pkg/errors"
	"github.com/M0Rf30/yap/v2/pkg/logger"
	"github.com/M0Rf30/yap/v2/pkg/shell"
)

// sourceArrayRe matches any source=() / source_<arch>=() declaration.
var sourceArrayRe = regexp.MustCompile(`^(source(_\w+)?)\s*=\s*\(`)

// scalarVarRe matches simple scalar assignments like pkgver=1.2.3 or pkgname="foo".
// It does NOT match array assignments (those start with a parenthesis after =).
var scalarVarRe = regexp.MustCompile(`(?m)^(\w+)=([^(\n][^\n]*)`)

// unquoteShellValue strips an optional surrounding quote pair (' or ") and
// truncates any inline comment (# …) that follows the value.
// Examples:
//
//	`"1.30.1" # renovate: …`  →  `1.30.1`
//	`'foo'`                   →  `foo`
//	`plain`                   →  `plain`
func unquoteShellValue(raw string) string {
	s := strings.TrimSpace(raw)

	if s == "" {
		return s
	}

	// Quoted value: extract content between the first matching quote pair.
	if s[0] == '"' || s[0] == '\'' {
		q := s[0]
		end := strings.IndexByte(s[1:], q)

		if end >= 0 {
			return s[1 : end+1]
		}
		// Unmatched quote — fall through to comment stripping.
	}

	// Unquoted value: strip inline comment.
	if idx := strings.Index(s, " #"); idx >= 0 {
		s = strings.TrimSpace(s[:idx])
	}

	return s
}

// extractScalarVars parses simple scalar variable assignments from raw PKGBUILD
// content and returns a lookup function suitable for use with mvdanshell.Expand.
func extractScalarVars(content string) func(string) string {
	vars := make(map[string]string)

	for _, m := range scalarVarRe.FindAllStringSubmatch(content, -1) {
		name := m[1]
		val := unquoteShellValue(m[2])
		vars[name] = val
	}

	return func(name string) string {
		if v, ok := vars[name]; ok {
			return v
		}

		return os.Getenv(name)
	}
}

// hashValueRe matches a hash entry (or SKIP) inside an array line.
// It handles both multi-line form (value on its own line with leading whitespace)
// and single-line form (value inline after the opening paren).
// Captures: [1] everything before the hash, [2] the hash/SKIP, [3] closing quote.
var hashValueRe = regexp.MustCompile(`(['"])([0-9a-fA-F]{32,128}|SKIP)(['"])`)

// UpdateChecksums reads the PKGBUILD at pkgbuildDir, downloads every
// non-VCS source, computes its SHA-256 digest, and rewrites the checksum
// array(s) in the file preserving the original formatting.
func UpdateChecksums(pkgbuildDir string) error {
	pkgbuildPath := filepath.Join(pkgbuildDir, "PKGBUILD")

	raw, err := os.ReadFile(filepath.Clean(pkgbuildPath))
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to read PKGBUILD").
			WithOperation("UpdateChecksums")
	}

	content := string(raw)

	// Build a variable expander so that ${pkgver}, ${pkgname}, etc. in source
	// URLs are resolved before downloading.
	expandVar := extractScalarVars(content)

	// Extract all source blocks keyed by arch suffix ("" for base, "_x86_64", …).
	sourceBlocks := extractArrayBlocks(content, sourceArrayRe)
	if len(sourceBlocks) == 0 {
		logger.Info("gensum: no source arrays found, nothing to do")

		return nil
	}

	tmpDir, err := os.MkdirTemp("", "yap-gensum-*")
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to create temp dir").
			WithOperation("UpdateChecksums")
	}

	defer func() { _ = os.RemoveAll(tmpDir) }()

	// For each source block, compute new hashes and rewrite the matching
	// checksum block in the file content.
	result := content
	changed := false

	for suffix, srcBlock := range sourceBlocks {
		uris := parseArrayValues(srcBlock)
		if len(uris) == 0 {
			continue
		}

		newHashes, err := computeHashes(uris, tmpDir, pkgbuildDir, expandVar)
		if err != nil {
			return err
		}

		// Find the checksum block with the same arch suffix.
		checksumKey := "sha256sums" + suffix

		result, err = replaceChecksumValues(result, checksumKey, newHashes)
		if err != nil {
			return err
		}

		changed = true

		logger.Info("gensum: updated checksums",
			"field", checksumKey,
			"count", len(newHashes))
	}

	if !changed {
		logger.Info("gensum: nothing changed")

		return nil
	}

	if err := os.WriteFile(pkgbuildPath, []byte(result), 0o644); err != nil { //nolint:gosec
		return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to write PKGBUILD").
			WithOperation("UpdateChecksums")
	}

	return nil
}

// extractArrayBlocks scans content line by line and returns a map from arch
// suffix (e.g. "", "_x86_64") to the raw text of the matching array block.
func extractArrayBlocks(content string, re *regexp.Regexp) map[string]string {
	blocks := make(map[string]string)
	lines := strings.Split(content, "\n")

	i := 0

	for i < len(lines) {
		line := lines[i]

		m := re.FindStringSubmatch(line)
		if m == nil {
			i++

			continue
		}

		suffix := m[2] // "" or "_x86_64" etc.

		// Collect the full block until the closing ")".
		var blockLines []string

		depth := strings.Count(line, "(") - strings.Count(line, ")")
		blockLines = append(blockLines, line)
		i++

		for i < len(lines) && depth > 0 {
			l := lines[i]
			depth += strings.Count(l, "(") - strings.Count(l, ")")
			blockLines = append(blockLines, l)
			i++
		}

		blocks[suffix] = strings.Join(blockLines, "\n")
	}

	return blocks
}

// parseArrayValues extracts the URI/value strings from a raw array block.
// It handles both single-line  source=('a' 'b')  and multi-line forms.
func parseArrayValues(block string) []string {
	// Strip the field name and outer parens.
	inner := block
	if idx := strings.Index(inner, "("); idx != -1 {
		inner = inner[idx+1:]
	}

	if idx := strings.LastIndex(inner, ")"); idx != -1 {
		inner = inner[:idx]
	}

	// Split on whitespace and strip quotes.
	var values []string

	for tok := range strings.FieldsSeq(inner) {
		tok = strings.Trim(tok, `'"`)
		if tok != "" {
			values = append(values, tok)
		}
	}

	return values
}

// computeHashes downloads each URI (unless it is a VCS source) and returns
// the SHA-256 hex digest.  VCS sources keep their existing value "SKIP".
// pkgbuildDir is used to resolve local file sources relative to the PKGBUILD.
// expandVar is used to substitute PKGBUILD variables (e.g. ${pkgver}) in URIs.
func computeHashes(uris []string, tmpDir, pkgbuildDir string, expandVar func(string) string) ([]string, error) {
	hashes := make([]string, len(uris))

	for i, rawURI := range uris {
		// Expand PKGBUILD variables (e.g. ${pkgver}, $pkgname) in the URI.
		expanded, _ := mvdanshell.Expand(rawURI, expandVar)
		uri := expanded

		// Strip custom name prefix: "name::url" → "url"
		if idx := strings.Index(uri, "::"); idx != -1 {
			uri = uri[idx+2:]
		}

		// Strip VCS fragment: "url#branch=foo" → "url"
		fragment := ""
		if idx := strings.Index(uri, "#"); idx != -1 {
			fragment = uri[idx+1:]
			uri = uri[:idx]
		}

		_ = fragment // kept for future commit-hash support

		// Git / VCS sources: keep SKIP.
		if strings.HasPrefix(uri, constants.Git+"+") ||
			strings.HasPrefix(uri, "git://") ||
			strings.HasPrefix(uri, "svn+") ||
			strings.HasPrefix(uri, "hg+") ||
			strings.HasPrefix(uri, "bzr+") {
			hashes[i] = "SKIP"

			logger.Info("gensum: skipping VCS source", "uri", rawURI)

			continue
		}

		h, err := downloadAndHash(expanded, uri, tmpDir, pkgbuildDir)
		if err != nil {
			return nil, err
		}

		logger.Info("gensum: hashed source",
			"file", filepath.Base(uri),
			"sha256", h[:16]+"…")

		hashes[i] = h
	}

	return hashes, nil
}

// downloadAndHash downloads uri to tmpDir and returns its SHA-256 hex digest.
// Local file sources (no scheme) are resolved relative to pkgbuildDir.
func downloadAndHash(rawURI, uri, tmpDir, pkgbuildDir string) (string, error) {
	if !strings.Contains(uri, "://") {
		localPath := uri
		if !filepath.IsAbs(localPath) {
			localPath = filepath.Join(pkgbuildDir, localPath)
		}

		return hashFile(localPath)
	}

	destName := filepath.Base(uri)
	if destName == "" || destName == "." {
		destName = "source"
	}

	destPath := filepath.Join(tmpDir, destName)

	_, err := shell.MultiPrinter.Start()
	if err != nil {
		return "", errors.Wrap(err, errors.ErrTypeBuild, "failed to start printer").
			WithOperation("downloadAndHash")
	}

	err = download.WithResumeContext(
		destPath,
		uri,
		3,
		"gensum",
		filepath.Base(rawURI),
		shell.MultiPrinter.Writer)
	if err != nil {
		return "", errors.Wrap(err, errors.ErrTypeBuild, "failed to download source").
			WithOperation("downloadAndHash").
			WithContext("uri", uri)
	}

	return hashFile(destPath)
}

// hashFile computes the SHA-256 digest of the file at path.
func hashFile(path string) (string, error) {
	f, err := os.Open(filepath.Clean(path))
	if err != nil {
		return "", errors.Wrap(err, errors.ErrTypeFileSystem, "failed to open file for hashing").
			WithOperation("hashFile").
			WithContext("path", path)
	}

	defer func() { _ = f.Close() }()

	h := sha256.New()

	if _, err := io.Copy(h, f); err != nil {
		return "", errors.Wrap(err, errors.ErrTypeFileSystem, "failed to hash file").
			WithOperation("hashFile").
			WithContext("path", path)
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

// replaceChecksumValues finds the checksum array named fieldName in content
// and replaces each hash value in-place, preserving all formatting.
//
// Strategy: locate the array block, then walk its lines replacing only the
// hash token on each value line while keeping quote style, indentation, and
// any trailing comments untouched.
func replaceChecksumValues(content, fieldName string, newHashes []string) (string, error) {
	// Build a regex that matches exactly this field name (not a prefix of another).
	// Use Compile (not MustCompile) so invalid UTF-8 in fieldName returns an error
	// instead of panicking.
	fieldRe, err := regexp.Compile(`(?m)^` + regexp.QuoteMeta(fieldName) + `\s*=\s*\(`)
	if err != nil {
		return "", errors.Wrap(err, errors.ErrTypeValidation, "invalid checksum field name").
			WithOperation("replaceChecksumValues").
			WithContext("field", fieldName)
	}

	loc := fieldRe.FindStringIndex(content)
	if loc == nil {
		// Field not present — append a new sha256sums block at the end.
		block := buildNewBlock(fieldName, newHashes)

		if !strings.HasSuffix(content, "\n") {
			content += "\n"
		}

		return content + block + "\n", nil
	}

	// Find the closing ")" of this array.
	start := loc[0]
	depth := 0
	end := -1

	for i := loc[0]; i < len(content); i++ {
		switch content[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				end = i
			}
		}

		if end != -1 {
			break
		}
	}

	if end == -1 {
		return "", errors.New(errors.ErrTypeValidation,
			fmt.Sprintf("unclosed array for field %q", fieldName)).
			WithOperation("replaceChecksumValues")
	}

	blockText := content[start : end+1]

	newBlock, err := replaceHashesInBlock(blockText, newHashes)
	if err != nil {
		return "", err
	}

	return content[:start] + newBlock + content[end+1:], nil
}

// replaceHashesInBlock rewrites the hash values inside a raw array block
// string, preserving all formatting.  It replaces values sequentially —
// the nth hash-looking token (quoted hex string or SKIP) gets newHashes[n].
//
// Works for both single-line  sha256sums=('h1' 'h2')  and multi-line forms.
func replaceHashesInBlock(block string, newHashes []string) (string, error) {
	idx := 0

	result := hashValueRe.ReplaceAllStringFunc(block, func(match string) string {
		if idx >= len(newHashes) {
			return match // more slots than expected — leave untouched
		}

		// Preserve the surrounding quote characters.
		m := hashValueRe.FindStringSubmatch(match)
		if m == nil {
			return match
		}

		replaced := m[1] + newHashes[idx] + m[3]
		idx++

		return replaced
	})

	if idx != len(newHashes) {
		return "", errors.New(errors.ErrTypeValidation,
			fmt.Sprintf("expected %d hash values in block, found %d replaceable slots",
				len(newHashes), idx)).
			WithOperation("replaceHashesInBlock")
	}

	return result, nil
}

// buildNewBlock renders a new sha256sums=(...) block in multi-line style.
// Used when the field is absent from the PKGBUILD entirely.
func buildNewBlock(fieldName string, hashes []string) string {
	var sb strings.Builder

	sb.WriteString(fieldName)
	sb.WriteString("=(\n")

	for _, h := range hashes {
		sb.WriteString("  '")
		sb.WriteString(h)
		sb.WriteString("'\n")
	}

	sb.WriteString(")")

	return sb.String()
}
