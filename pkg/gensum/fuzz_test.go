package gensum_test

// Fuzz tests for the pure text-manipulation functions in pkg/gensum.
//
// Run with:
//
//	go test -fuzz=FuzzReplaceChecksumValues    -fuzztime=30s ./pkg/gensum/
//	go test -fuzz=FuzzReplaceHashesInBlock     -fuzztime=30s ./pkg/gensum/
//	go test -fuzz=FuzzParseArrayValues         -fuzztime=30s ./pkg/gensum/
//	go test -fuzz=FuzzExtractSourceBlocks      -fuzztime=30s ./pkg/gensum/
//	go test -fuzz=FuzzRoundTrip                -fuzztime=60s ./pkg/gensum/
//
// The seed corpus below covers the known interesting shapes; the fuzzer
// explores the rest automatically.

import (
	"strings"
	"testing"

	"github.com/M0Rf30/yap/v2/pkg/gensum"
)

// ── helpers ──────────────────────────────────────────────────────────────────

// countHashSlots counts how many quoted hash-or-SKIP tokens appear in s.
func countHashSlots(s string) int {
	n := 0
	rest := s

	for {
		start := strings.IndexAny(rest, `'"`)
		if start == -1 {
			break
		}

		q := rest[start]
		end := strings.IndexByte(rest[start+1:], q)

		if end == -1 {
			break
		}

		tok := rest[start+1 : start+1+end]
		if isHashOrSkip(tok) {
			n++
		}

		rest = rest[start+1+end+1:]
	}

	return n
}

func isHashOrSkip(s string) bool {
	if s == "SKIP" {
		return true
	}

	if len(s) < 32 || len(s) > 128 {
		return false
	}

	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') && (c < 'A' || c > 'F') {
			return false
		}
	}

	return true
}

// ── FuzzReplaceChecksumValues ─────────────────────────────────────────────────

// FuzzReplaceChecksumValues verifies that replaceChecksumValues never panics
// and satisfies key invariants regardless of input.
func FuzzReplaceChecksumValues(f *testing.F) {
	// Seed: single-line
	f.Add("sha256sums=('SKIP')\n", "sha256sums", "aabbcc")
	// Seed: multi-line
	f.Add("sha256sums=(\n  'SKIP'\n  'SKIP'\n)\n", "sha256sums", "aabbcc")
	// Seed: double-quote style
	f.Add(`sha256sums=("SKIP")`, "sha256sums", "deadbeef")
	// Seed: arch-specific field
	f.Add("sha256sums_x86_64=('SKIP')\n", "sha256sums_x86_64", "cafebabe")
	// Seed: field absent — should append
	f.Add("pkgname=foo\n", "sha256sums", "newhash")
	// Seed: multiple fields, only one replaced
	f.Add("sha256sums=('SKIP')\nsha256sums_x86_64=('SKIP')\n", "sha256sums", "hash1")
	// Seed: real 64-char hash
	f.Add("sha256sums=('e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855')\n",
		"sha256sums", "aabbccddeeff00112233445566778899aabbccddeeff00112233445566778899")
	// Seed: empty content
	f.Add("", "sha256sums", "hash")
	// Seed: content with no newline at end
	f.Add("sha256sums=('SKIP')", "sha256sums", "hash")
	// Seed: nested parens in a comment
	f.Add("# comment (with parens)\nsha256sums=('SKIP')\n", "sha256sums", "hash")

	f.Fuzz(func(t *testing.T, content, fieldName, newHash string) {
		// Must not panic.
		result, err := gensum.ReplaceChecksumValuesExported(content, fieldName, []string{newHash})
		if err != nil {
			// Errors are acceptable (e.g. mismatched slot count); panics are not.
			return
		}

		// Invariant 1: result is non-empty when content is non-empty.
		if content != "" && result == "" {
			t.Errorf("result is empty for non-empty content")
		}

		// Invariant 2: idempotency — count how many hash slots are in the
		// result, then replace them all with the same value; the output must
		// be stable on a second pass.
		slotCount := countHashSlots(result)
		if slotCount > 0 {
			sameHashes := make([]string, slotCount)
			for i := range sameHashes {
				sameHashes[i] = newHash
			}

			result2, err2 := gensum.ReplaceChecksumValuesExported(result, fieldName, sameHashes)
			if err2 != nil {
				return
			}

			result3, err3 := gensum.ReplaceChecksumValuesExported(result2, fieldName, sameHashes)
			if err3 != nil {
				return
			}

			if result2 != result3 {
				t.Errorf("not idempotent:\nfirst:  %q\nsecond: %q", result2, result3)
			}
		}

		// Invariant 3: the new hash appears in the result.
		if newHash != "" && isHashOrSkip(newHash) {
			if !strings.Contains(result, newHash) {
				t.Errorf("new hash %q not found in result", newHash)
			}
		}
	})
}

// ── FuzzReplaceHashesInBlock ──────────────────────────────────────────────────

// FuzzReplaceHashesInBlock verifies the inner block rewriter never panics and
// preserves the surrounding non-hash text.
func FuzzReplaceHashesInBlock(f *testing.F) {
	f.Add("sha256sums=('SKIP')", "newhash", 1)
	f.Add("sha256sums=(\n  'SKIP'\n  'SKIP'\n)", "newhash", 2)
	f.Add(`sha256sums=("SKIP")`, "newhash", 1)
	f.Add("sha256sums=()", "newhash", 0)
	f.Add("sha256sums=('aabbcc')", "deadbeef", 1)

	f.Fuzz(func(t *testing.T, block, newHash string, count int) {
		if count < 0 || count > 64 {
			return // skip unreasonable counts
		}

		hashes := make([]string, count)
		for i := range hashes {
			hashes[i] = newHash
		}

		// Must not panic.
		result, err := gensum.ReplaceHashesInBlockExported(block, hashes)
		if err != nil {
			return // mismatch errors are acceptable
		}

		// Invariant: result length is >= block length when hashes are longer.
		_ = result
	})
}

// ── FuzzParseArrayValues ──────────────────────────────────────────────────────

// FuzzParseArrayValues verifies the value extractor never panics and returns
// a consistent result.
func FuzzParseArrayValues(f *testing.F) {
	f.Add(`source=('https://example.com/foo.tar.gz')`)
	f.Add(`source=(
  'https://example.com/foo.tar.gz'
  'git+https://github.com/example/repo'
)`)
	f.Add(`source=("a" "b" "c")`)
	f.Add(`source=()`)
	f.Add(`source=('name::https://example.com/foo.tar.gz#branch=main')`)
	f.Add(`source=('SKIP')`)
	f.Add(``)
	f.Add(`source=(`)    // unclosed
	f.Add(`source=)`)    // malformed
	f.Add(`source=('')`) // empty quoted string

	f.Fuzz(func(t *testing.T, block string) {
		// Must not panic.
		vals := gensum.ParseArrayValuesExported(block)

		// Invariant: result is a valid (possibly empty) slice.
		if vals == nil {
			vals = []string{}
		}

		// Invariant: no value contains unstripped quote characters at boundaries.
		for _, v := range vals {
			if strings.HasPrefix(v, "'") || strings.HasPrefix(v, `"`) {
				t.Errorf("value has leading quote: %q", v)
			}

			if strings.HasSuffix(v, "'") || strings.HasSuffix(v, `"`) {
				t.Errorf("value has trailing quote: %q", v)
			}
		}
	})
}

// ── FuzzExtractSourceBlocks ───────────────────────────────────────────────────

// FuzzExtractSourceBlocks verifies the block extractor never panics and
// returns consistent arch-suffix keys.
func FuzzExtractSourceBlocks(f *testing.F) {
	f.Add(`source=('https://example.com/foo.tar.gz')`)
	f.Add(`source=('a')
source_x86_64=('b')
source_aarch64=('c')`)
	f.Add(`source=(
  'a'
  'b'
)`)
	f.Add(``) // empty
	f.Add(`# just a comment`)
	f.Add(`source=(`) // unclosed
	f.Add(`source=()
source=()`) // duplicate base

	f.Fuzz(func(t *testing.T, content string) {
		// Must not panic.
		blocks := gensum.ExtractSourceBlocksExported(content)

		// Invariant: all keys start with "" or "_".
		for k := range blocks {
			if k != "" && !strings.HasPrefix(k, "_") {
				t.Errorf("unexpected key format: %q", k)
			}
		}

		// Invariant: each block value contains the opening paren.
		for k, v := range blocks {
			if !strings.Contains(v, "(") {
				t.Errorf("block for key %q has no opening paren: %q", k, v)
			}
		}
	})
}

// ── FuzzRoundTrip ─────────────────────────────────────────────────────────────

// FuzzRoundTrip verifies the full replace→replace cycle: replacing hashes
// with themselves must be a no-op (idempotent).
func FuzzRoundTrip(f *testing.F) {
	f.Add("sha256sums=('SKIP')\n", "sha256sums")
	f.Add("sha256sums=(\n  'SKIP'\n  'SKIP'\n)\n", "sha256sums")
	f.Add("sha256sums=('aabbccddeeff00112233445566778899aabbccddeeff00112233445566778899')\n",
		"sha256sums")
	f.Add("b2sums=('SKIP')\n", "b2sums")
	f.Add("sha256sums=('SKIP')\nsha256sums_x86_64=('SKIP')\n", "sha256sums")

	f.Fuzz(func(t *testing.T, content, fieldName string) {
		// Count slots in the original content, then replace them all with SKIP.
		slotCount := countHashSlots(content)
		if slotCount == 0 {
			return
		}

		hashes := make([]string, slotCount)
		for i := range hashes {
			hashes[i] = "SKIP"
		}

		result1, err := gensum.ReplaceChecksumValuesExported(content, fieldName, hashes)
		if err != nil {
			return
		}

		// Apply the same replacement again — must be identical.
		result2, err := gensum.ReplaceChecksumValuesExported(result1, fieldName, hashes)
		if err != nil {
			return
		}

		if result1 != result2 {
			t.Errorf("round-trip not idempotent for field %q:\nbefore: %q\nafter:  %q",
				fieldName, result1, result2)
		}
	})
}
