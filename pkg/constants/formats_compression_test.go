package constants

import "testing"

func TestIsSupportedCompression(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"", true}, // empty → default applied downstream
		{"zstd", true},
		{"gzip", true},
		{"xz", true},
		{"bzip2", false},
		{"lz4", false},
		{"ZSTD", false}, // case-sensitive
		{"none", false},
	}

	for _, c := range cases {
		if got := IsSupportedCompression(c.in); got != c.want {
			t.Errorf("IsSupportedCompression(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestSupportedCompressionsContents(t *testing.T) {
	want := map[string]bool{"zstd": true, "gzip": true, "xz": true}
	if len(SupportedCompressions) != len(want) {
		t.Fatalf("SupportedCompressions = %v, want keys %v", SupportedCompressions, want)
	}

	for _, c := range SupportedCompressions {
		if !want[c] {
			t.Errorf("unexpected compression %q in SupportedCompressions", c)
		}
	}
}
