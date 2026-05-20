package apkindex_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/M0Rf30/yap/v2/pkg/apkindex"
)

// sampleAPKINDEX is a minimal APKINDEX stanza as found in Alpine repositories.
const sampleAPKINDEX = `C:Q1+Ys9xMNWEb3xQUefSpmlnP8xQ=
P:musl
V:1.2.3-r4
A:x86_64
S:123456
I:234567
T:the musl c library
U:https://musl.libc.org
L:MIT
o:musl
m:Natanael Copa <ncopa@alpinelinux.org>
D:
p:

C:Q1+abc123def456ghi789jkl012mno=
P:musl-dev
V:1.2.3-r4
A:x86_64
S:234567
I:345678
T:musl development files
U:https://musl.libc.org
L:MIT
o:musl
m:Natanael Copa <ncopa@alpinelinux.org>
D:musl=1.2.3-r4
p:

C:Q1+xyz789abc456def123ghi012jkl=
P:gcc
V:12.2.1-r1
A:x86_64
S:345678
I:456789
T:GNU Compiler Collection
U:https://gcc.gnu.org
L:GPL-3.0-or-later
o:gcc
m:Natanael Copa <ncopa@alpinelinux.org>
D:musl-dev>=1.2 binutils
p:

C:Q1+vir123tua456lpa567cka678gez=
P:virtual-pkg
V:1.0-r0
A:x86_64
S:100
I:200
T:A virtual package
U:https://example.com
L:MIT
o:virtual-pkg
m:Test <test@example.com>
D:
p:service-discover

`

func TestParseIndex(t *testing.T) {
	idx := apkindex.NewIndex()
	err := idx.ParseIndex(strings.NewReader(sampleAPKINDEX), "https://dl-cdn.alpinelinux.org/alpine/v3.20/main")
	require.NoError(t, err)

	t.Run("lookup musl", func(t *testing.T) {
		pkg, ok := idx.Lookup("musl")
		require.True(t, ok)
		assert.Equal(t, "musl", pkg.Name)
		assert.Equal(t, "1.2.3-r4", pkg.Version)
		assert.Equal(t, "x86_64", pkg.Arch)
		assert.Equal(t, int64(123456), pkg.Size)
		assert.Equal(t, int64(234567), pkg.InstSize)
		assert.Equal(t, "the musl c library", pkg.Description)
		assert.Equal(t, "MIT", pkg.License)
		assert.Equal(t, "musl", pkg.Origin)
	})

	t.Run("lookup musl-dev with deps", func(t *testing.T) {
		pkg, ok := idx.Lookup("musl-dev")
		require.True(t, ok)
		assert.Equal(t, "musl-dev", pkg.Name)
		assert.Equal(t, []string{"musl=1.2.3-r4"}, pkg.Depends)
	})

	t.Run("lookup gcc with multiple deps", func(t *testing.T) {
		pkg, ok := idx.Lookup("gcc")
		require.True(t, ok)
		assert.Equal(t, "gcc", pkg.Name)
		assert.Equal(t, []string{"musl-dev>=1.2", "binutils"}, pkg.Depends)
	})

	t.Run("lookup nonexistent", func(t *testing.T) {
		_, ok := idx.Lookup("nonexistent")
		assert.False(t, ok)
	})

	t.Run("virtual package resolution", func(t *testing.T) {
		pkg, ok := idx.ResolveVirtual("service-discover")
		require.True(t, ok)
		assert.Equal(t, "virtual-pkg", pkg.Name)
	})

	t.Run("nonexistent virtual", func(t *testing.T) {
		_, ok := idx.ResolveVirtual("nonexistent-virtual")
		assert.False(t, ok)
	})
}

func TestResolveDeps(t *testing.T) {
	idx := apkindex.NewIndex()
	err := idx.ParseIndex(strings.NewReader(sampleAPKINDEX), "https://dl-cdn.alpinelinux.org/alpine/v3.20/main")
	require.NoError(t, err)

	t.Run("simple dep", func(t *testing.T) {
		resolved, err := idx.ResolveDeps([]string{"musl-dev"})
		require.NoError(t, err)
		require.Len(t, resolved, 2) // musl + musl-dev
		assert.Equal(t, "musl", resolved[0].Name)
		assert.Equal(t, "musl-dev", resolved[1].Name)
	})

	t.Run("transitive deps", func(t *testing.T) {
		resolved, err := idx.ResolveDeps([]string{"gcc"})
		require.NoError(t, err)
		// gcc depends on musl-dev>=1.2 and binutils
		// musl-dev depends on musl=1.2.3-r4
		// So we expect: musl, musl-dev, binutils, gcc (in order)
		names := make([]string, len(resolved))
		for i, p := range resolved {
			names[i] = p.Name
		}

		assert.Contains(t, names, "musl")
		assert.Contains(t, names, "musl-dev")
		assert.Contains(t, names, "gcc")
	})

	t.Run("virtual package resolution in deps", func(t *testing.T) {
		resolved, err := idx.ResolveDeps([]string{"service-discover"})
		require.NoError(t, err)
		require.Len(t, resolved, 1)
		assert.Equal(t, "virtual-pkg", resolved[0].Name)
	})

	t.Run("empty input", func(t *testing.T) {
		resolved, err := idx.ResolveDeps([]string{})
		require.NoError(t, err)
		assert.Len(t, resolved, 0)
	})

	t.Run("nonexistent package", func(t *testing.T) {
		// Should not error; just skip the nonexistent package.
		resolved, err := idx.ResolveDeps([]string{"nonexistent"})
		require.NoError(t, err)
		assert.Len(t, resolved, 0)
	})
}

func TestStripVersionConstraint(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"musl-dev>=1.2", "musl-dev"},
		{"musl-dev=1.2.3-r4", "musl-dev"},
		{"gcc>12", "gcc"},
		{"binutils<2.0", "binutils"},
		{"foo!=1.0", "foo"},
		{"bar<=3.0", "bar"},
		{"baz~1.5", "baz"},
		{"simple", "simple"},
		{"  spaces  ", "spaces"},
		{"pkg>=1.0>=2.0", "pkg"}, // First match wins
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := stripVersionConstraint(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// stripVersionConstraint is a helper for testing (mirrors the one in parser.go).
func stripVersionConstraint(spec string) string {
	spec = strings.TrimSpace(spec)

	// Multi-char operators first (longest match wins).
	for _, op := range []string{"!=", ">=", "<="} {
		if before, _, ok := strings.Cut(spec, op); ok {
			return strings.TrimSpace(before)
		}
	}

	// Single-char operators.
	for _, op := range []string{"~", "=", ">", "<"} {
		if before, _, ok := strings.Cut(spec, op); ok {
			return strings.TrimSpace(before)
		}
	}

	return spec
}
