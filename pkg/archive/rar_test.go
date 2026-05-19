//nolint:testpackage // tests cover unexported detectKind / classify helpers
package archive

import (
	"strings"
	"testing"
)

// TestClassify_RarMagic verifies that the rar magic-byte signatures
// (v1.5+ and v5) are detected by classify(). We exercise the helper
// directly to avoid shipping a real .rar fixture as a binary blob.
func TestClassify_RarMagic(t *testing.T) {
	cases := []struct {
		name   string
		header []byte
	}{
		{
			"rar v1.5",
			[]byte{'R', 'a', 'r', '!', 0x1A, 0x07, 0x00},
		},
		{
			"rar v5",
			[]byte{'R', 'a', 'r', '!', 0x1A, 0x07, 0x01, 0x00},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Pad header to sniffSize so classify() behaves like detectKind would.
			padded := append([]byte{}, tc.header...)
			padded = append(padded, make([]byte, sniffSize-len(padded))...)

			if got := classify(padded, "x.rar"); got != kindRar {
				t.Fatalf("classify(rar magic) = %d, want kindRar (%d)", got, kindRar)
			}
		})
	}
}

// TestClassify_SevenZipMagic verifies that the 7z magic byte sequence
// "7z" + 0xBC 0xAF 0x27 0x1C is detected as kind7z.
func TestClassify_SevenZipMagic(t *testing.T) {
	header := []byte{'7', 'z', 0xBC, 0xAF, 0x27, 0x1C}
	padded := append([]byte{}, header...)
	padded = append(padded, make([]byte, sniffSize-len(padded))...)

	if got := classify(padded, "x.7z"); got != kind7z {
		t.Fatalf("classify(7z magic) = %d, want kind7z (%d)", got, kind7z)
	}
}

// TestClassify_SuffixFallbackRestricted verifies that suffix-based
// classification is restricted to plain .tar — a file whose name ends in
// .tar.gz / .zip / .7z but contains no magic bytes is reported as
// kindUnknown, not silently misclassified.
func TestClassify_SuffixFallbackRestricted(t *testing.T) {
	plainText := []byte(strings.Repeat("x", sniffSize))

	cases := []struct {
		name string
		want archiveKind
	}{
		{"x.tar", kindTar},
		{"x.tar.gz", kindUnknown},
		{"x.zip", kindUnknown},
		{"x.7z", kindUnknown},
		{"x.rar", kindUnknown},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := classify(plainText, tc.name); got != tc.want {
				t.Fatalf("classify(plain, %q) = %d, want %d", tc.name, got, tc.want)
			}
		})
	}
}
