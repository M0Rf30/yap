package aptcache_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/M0Rf30/yap/v2/pkg/aptcache"
)

// samplePackagesStanza is a minimal deb822 stanza as found in
// /var/lib/apt/lists/*_Packages files.
const samplePackagesStanza = `Package: libssl-dev
Architecture: amd64
Multi-Arch: same
Version: 3.0.2-0ubuntu1
Description: Secure Sockets Layer toolkit - development files
 This package is part of the OpenSSL project.

Package: make
Architecture: all
Version: 4.3-4.1build1
Description: utility for directing compilation

Package: bash
Architecture: amd64
Essential: yes
Version: 5.1-6ubuntu1
Description: GNU Bourne Again SHell

Package: libgcc-s1
Architecture: amd64
Multi-Arch: foreign
Version: 12.3.0-1ubuntu1
Description: GCC support library

`

// sampleDpkgStatus is a minimal deb822 stanza as found in /var/lib/dpkg/status.
const sampleDpkgStatus = `Package: libssl-dev
Status: install ok installed
Architecture: amd64
Multi-Arch: same
Version: 3.0.2-0ubuntu1
Description: Secure Sockets Layer toolkit - development files

Package: make
Status: install ok installed
Architecture: all
Version: 4.3-4.1build1
Description: utility for directing compilation

Package: libfoo
Status: deinstall ok config-files
Architecture: amd64
Version: 1.0
Description: a removed package

`

// sampleVirtualPackages tests Provides / ResolveVirtual.
const sampleVirtualPackages = `Package: default-jre
Architecture: amd64
Version: 2:1.11-72
Provides: java-runtime (= 11), java11-runtime
Description: Standard Java runtime

Package: openjdk-11-jre
Architecture: amd64
Version: 11.0.20+8-1ubuntu1
Provides: java-runtime (= 11), java11-runtime
Description: OpenJDK Java runtime

`

func newCacheFromStrings(t *testing.T, aptContent, dpkgContent string) *aptcache.Cache {
	t.Helper()

	c := aptcache.NewCacheForTesting()

	if aptContent != "" {
		err := c.ParseDeb822ForTesting(strings.NewReader(aptContent), false)
		require.NoError(t, err)
	}

	if dpkgContent != "" {
		err := c.ParseDeb822ForTesting(strings.NewReader(dpkgContent), true)
		require.NoError(t, err)
	}

	return c
}

func TestLookup_AptIndex(t *testing.T) {
	c := newCacheFromStrings(t, samplePackagesStanza, "")

	t.Run("arch-specific package", func(t *testing.T) {
		info, ok := c.Lookup("libssl-dev")
		require.True(t, ok)
		assert.Equal(t, "amd64", info.Architecture)
		assert.Equal(t, "same", info.MultiArch)
		assert.False(t, info.Essential)
		assert.False(t, info.Installed)
		assert.False(t, info.ArchitectureAll())
		assert.False(t, info.MultiArchForeign())
		assert.True(t, info.MultiArchSame())
	})

	t.Run("arch-all package", func(t *testing.T) {
		info, ok := c.Lookup("make")
		require.True(t, ok)
		assert.Equal(t, "all", info.Architecture)
		assert.True(t, info.ArchitectureAll())
		assert.False(t, info.MultiArchForeign())
	})

	t.Run("essential package", func(t *testing.T) {
		info, ok := c.Lookup("bash")
		require.True(t, ok)
		assert.True(t, info.Essential)
		assert.False(t, info.ArchitectureAll())
	})

	t.Run("multi-arch foreign", func(t *testing.T) {
		info, ok := c.Lookup("libgcc-s1")
		require.True(t, ok)
		assert.Equal(t, "foreign", info.MultiArch)
		assert.True(t, info.MultiArchForeign())
	})

	t.Run("unknown package", func(t *testing.T) {
		_, ok := c.Lookup("nonexistent-pkg")
		assert.False(t, ok)
	})
}

func TestLookup_DpkgStatus(t *testing.T) {
	c := newCacheFromStrings(t, "", sampleDpkgStatus)

	t.Run("installed package", func(t *testing.T) {
		info, ok := c.Lookup("libssl-dev")
		require.True(t, ok)
		assert.True(t, info.Installed)
	})

	t.Run("arch-all installed", func(t *testing.T) {
		info, ok := c.Lookup("make")
		require.True(t, ok)
		assert.True(t, info.Installed)
		assert.True(t, info.ArchitectureAll())
	})

	t.Run("deinstalled package not marked installed", func(t *testing.T) {
		info, ok := c.Lookup("libfoo")
		require.True(t, ok)
		assert.False(t, info.Installed)
	})
}

func TestMerge_AptThenDpkg(t *testing.T) {
	// Simulate the real load order: apt index first, dpkg status overlays.
	c := newCacheFromStrings(t, samplePackagesStanza, sampleDpkgStatus)

	info, ok := c.Lookup("libssl-dev")
	require.True(t, ok)
	// Architecture and Multi-Arch come from apt index.
	assert.Equal(t, "amd64", info.Architecture)
	assert.Equal(t, "same", info.MultiArch)
	// Installed flag comes from dpkg status overlay.
	assert.True(t, info.Installed)
}

func TestResolveVirtual(t *testing.T) {
	c := newCacheFromStrings(t, sampleVirtualPackages, "")

	t.Run("real package returns itself", func(t *testing.T) {
		assert.Equal(t, "default-jre", c.ResolveVirtual("default-jre"))
	})

	t.Run("virtual name resolves to first provider", func(t *testing.T) {
		// "java-runtime" is provided by default-jre and openjdk-11-jre.
		// First provider encountered in the index is returned.
		resolved := c.ResolveVirtual("java-runtime")
		assert.NotEqual(t, "java-runtime", resolved, "should resolve to a concrete package")
	})

	t.Run("unknown name returns itself", func(t *testing.T) {
		assert.Equal(t, "no-such-pkg", c.ResolveVirtual("no-such-pkg"))
	})
}

func TestPackageInfo_MultiArchForeign(t *testing.T) {
	cases := []struct {
		multiArch string
		want      bool
	}{
		{"foreign", true},
		{"Foreign", true}, // case-insensitive
		{"allowed", true},
		{"same", false}, // same = dev lib, not foreign/allowed
		{"no", false},
		{"", false},
	}

	for _, tc := range cases {
		p := aptcache.PackageInfo{MultiArch: tc.multiArch}
		assert.Equal(t, tc.want, p.MultiArchForeign(), "MultiArch=%q", tc.multiArch)
	}
}

// TestMultiArchClassification verifies that the Multi-Arch field correctly
// classifies packages that were previously handled by the hardcoded
// isHostOnlyPackage() list. Every entry here must map to the expected
// MultiArchForeign() / MultiArchSame() result so that partitionArchAllDeps
// keeps host tools unqualified and qualifies dev libraries with :arm64.
func TestMultiArchClassification(t *testing.T) {
	// Data derived from `apt-cache show` on Ubuntu Jammy.
	cases := []struct {
		pkg         string
		arch        string
		multiArch   string
		wantForeign bool // should be kept unqualified (host tool)
		wantSame    bool // should be qualified :arm64 (dev lib)
	}{
		// Build tools — Multi-Arch: foreign or allowed → host-only
		{"make", "amd64", "allowed", true, false},
		{"cmake", "amd64", "foreign", true, false},
		{"git", "amd64", "foreign", true, false},
		{"bison", "amd64", "foreign", true, false},
		{"flex", "amd64", "foreign", true, false},
		{"autoconf", "all", "foreign", true, false},
		{"automake", "all", "foreign", true, false},
		{"libtool", "all", "foreign", true, false},
		{"pkg-config", "amd64", "foreign", true, false},
		{"patch", "amd64", "foreign", true, false},
		{"m4", "amd64", "foreign", true, false},
		{"groff-base", "amd64", "foreign", true, false},
		{"systemd", "amd64", "foreign", true, false},
		{"perl", "amd64", "allowed", true, false},
		{"python3", "amd64", "allowed", true, false},
		// Dev libraries — Multi-Arch: same → qualify with :arm64
		{"libssl-dev", "amd64", "same", false, true},
		{"libsystemd-dev", "amd64", "same", false, true},
		{"libbz2-dev", "amd64", "same", false, true},
		{"zlib1g-dev", "amd64", "same", false, true},
		{"libpcre2-dev", "amd64", "same", false, true},
	}

	for _, tc := range cases {
		t.Run(tc.pkg, func(t *testing.T) {
			p := aptcache.PackageInfo{
				Architecture: tc.arch,
				MultiArch:    tc.multiArch,
			}
			assert.Equal(t, tc.wantForeign, p.MultiArchForeign(),
				"%s: MultiArchForeign() (Multi-Arch=%q)", tc.pkg, tc.multiArch)
			assert.Equal(t, tc.wantSame, p.MultiArchSame(),
				"%s: MultiArchSame() (Multi-Arch=%q)", tc.pkg, tc.multiArch)
		})
	}
}

func TestEmptyInput(t *testing.T) {
	c := newCacheFromStrings(t, "", "")
	_, ok := c.Lookup("anything")
	assert.False(t, ok)
}

func TestNoTrailingNewline(t *testing.T) {
	// File without trailing blank line — last stanza must still be flushed.
	c := aptcache.NewCacheForTesting()
	err := c.ParseDeb822ForTesting(strings.NewReader("Package: curl\nArchitecture: amd64\nVersion: 7.81.0"), false)
	require.NoError(t, err)

	info, ok := c.Lookup("curl")
	require.True(t, ok)
	assert.Equal(t, "amd64", info.Architecture)
}
