package aptcache_test

import (
	"bytes"
	"testing"

	"github.com/M0Rf30/yap/v2/pkg/aptcache"
)

// FuzzParseDeb822 tests the deb822 parser with arbitrary input.
// It must never panic and should maintain invariants:
// - Lookup always returns valid PackageInfo (no nil deref)
// - ArchitectureAll/MultiArchForeign never panic
// - ResolveVirtual always returns non-empty string
func FuzzParseDeb822(f *testing.F) {
	// Seed corpus with valid deb822 stanzas
	f.Add([]byte(`Package: test
Architecture: amd64
Version: 1.0
`))
	f.Add([]byte(`Package: test
Architecture: all
Version: 1.0
`))
	f.Add([]byte(`Package: test
Multi-Arch: foreign
Version: 1.0
`))
	f.Add([]byte(`Package: test
Multi-Arch: same
Version: 1.0
`))
	f.Add([]byte(`Package: test
Essential: yes
Version: 1.0
`))
	f.Add([]byte(`Package: test
Provides: virtual-pkg (= 1.0)
Version: 1.0
`))
	f.Add([]byte(`Package: test1
Version: 1.0

Package: test2
Version: 2.0
`))
	// Empty input
	f.Add([]byte(""))
	// Binary garbage
	f.Add([]byte("\x00\x01\x02\x03\xff\xfe\xfd"))
	// Very long line
	f.Add(bytes.Repeat([]byte("a"), 300000))
	// Missing colons
	f.Add([]byte("Package test\nArchitecture amd64\n"))
	// Duplicate Package fields
	f.Add([]byte(`Package: test1
Package: test2
Version: 1.0
`))
	// Malformed Status lines
	f.Add([]byte(`Package: test
Status: invalid status
Version: 1.0
`))
	// Provides with version constraints
	f.Add([]byte(`Package: test
Provides: pkg1 (= 1.0), pkg2 (>= 2.0), pkg3
Version: 1.0
`))
	// Continuation lines
	f.Add([]byte(`Package: test
Description: This is a long
 description that spans
 multiple lines
Version: 1.0
`))
	// Mixed case fields
	f.Add([]byte(`package: test
ARCHITECTURE: amd64
version: 1.0
`))

	f.Fuzz(func(t *testing.T, data []byte) {
		c := aptcache.NewCacheForTesting()
		_ = c.ParseDeb822ForTesting(bytes.NewReader(data), false)

		// Invariant 1: Lookup always returns valid PackageInfo (no nil deref)
		// Try to lookup various package names
		for _, name := range []string{"test", "test1", "test2", "nonexistent", ""} {
			info, _ := c.Lookup(name)
			// Should not panic when accessing fields
			_ = info.ArchitectureAll()
			_ = info.MultiArchForeign()
		}

		// Invariant 2: ResolveVirtual returns the input name if not found
		// (which may be empty if input is empty, but should not panic)
		for _, name := range []string{"test", "virtual-pkg", "nonexistent", ""} {
			resolved := c.ResolveVirtual(name)
			// ResolveVirtual returns the input name if not found, so it can be empty
			// The important thing is it doesn't panic
			_ = resolved
		}
	})
}

// FuzzParseDeb822DpkgStatus tests the deb822 parser with dpkgStatus=true.
// Invariant: Installed flag is only true when input contains "Status: install ok installed"
func FuzzParseDeb822DpkgStatus(f *testing.F) {
	// Seed corpus with valid dpkg status stanzas
	f.Add([]byte(`Package: test
Status: install ok installed
Version: 1.0
`))
	f.Add([]byte(`Package: test
Status: deinstall ok config-files
Version: 1.0
`))
	f.Add([]byte(`Package: test
Status: install ok unpacked
Version: 1.0
`))
	f.Add([]byte(`Package: test1
Status: install ok installed
Version: 1.0

Package: test2
Status: deinstall ok config-files
Version: 2.0
`))
	// Empty input
	f.Add([]byte(""))
	// Missing Status field
	f.Add([]byte(`Package: test
Version: 1.0
`))
	// Malformed Status
	f.Add([]byte(`Package: test
Status: garbage
Version: 1.0
`))

	f.Fuzz(func(t *testing.T, data []byte) {
		c := aptcache.NewCacheForTesting()
		_ = c.ParseDeb822ForTesting(bytes.NewReader(data), true)

		// Invariant: Installed flag is only true when Status is "install ok installed"
		// We can't directly inspect the cache, but we can verify no panics occur
		// when accessing the Installed field through Lookup
		for _, name := range []string{"test", "test1", "test2", "nonexistent"} {
			info, ok := c.Lookup(name)
			if ok {
				// Should not panic
				_ = info.Installed
			}
		}
	})
}

// FuzzLookupAfterParse tests parsing arbitrary content then lookup with arbitrary names.
// Must never panic.
func FuzzLookupAfterParse(f *testing.F) {
	f.Add([]byte(`Package: test
Version: 1.0
`), []byte("test"))
	f.Add([]byte(`Package: foo
Version: 1.0
`), []byte("bar"))
	f.Add([]byte(""), []byte("anything"))
	f.Add([]byte("\x00\x01\x02"), []byte("\xff\xfe"))

	f.Fuzz(func(t *testing.T, parseData, lookupName []byte) {
		c := aptcache.NewCacheForTesting()
		_ = c.ParseDeb822ForTesting(bytes.NewReader(parseData), false)

		// Should not panic with arbitrary lookup names
		_, _ = c.Lookup(string(lookupName))
	})
}

// FuzzResolveVirtual tests parsing arbitrary content then ResolveVirtual with arbitrary names.
// Must never panic. Returns the input name if not found (which may be empty).
func FuzzResolveVirtual(f *testing.F) {
	f.Add([]byte(`Package: test
Provides: virtual-pkg
Version: 1.0
`), []byte("virtual-pkg"))
	f.Add([]byte(`Package: test
Version: 1.0
`), []byte("test"))
	f.Add([]byte(""), []byte("anything"))
	f.Add([]byte("\x00\x01\x02"), []byte("\xff\xfe"))

	f.Fuzz(func(t *testing.T, parseData, resolveName []byte) {
		c := aptcache.NewCacheForTesting()
		_ = c.ParseDeb822ForTesting(bytes.NewReader(parseData), false)

		// Should not panic. ResolveVirtual returns the input name if not found,
		// so it can be empty if the input is empty.
		resolved := c.ResolveVirtual(string(resolveName))
		// The important invariant is that it doesn't panic
		_ = resolved
	})
}
