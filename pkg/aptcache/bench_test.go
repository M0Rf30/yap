package aptcache_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/M0Rf30/yap/v2/pkg/aptcache"
)

// makePackagesStanza builds a synthetic deb822 Packages stanza body
// containing n stanzas. Each stanza carries the fields the parser
// actually reads (Name, Architecture, Version, Depends, Filename, SHA256,
// Size). Used to simulate the parse cost of a real Ubuntu Packages.xz
// file (~60k stanzas in production; smaller multiples here keep
// benchmarks under a couple seconds).
func makePackagesStanza(n int) string {
	var b strings.Builder

	b.Grow(n * 220)

	for i := range n {
		fmt.Fprintf(&b,
			"Package: pkg-%06d\n"+
				"Architecture: amd64\n"+
				"Version: 1.0.%d\n"+
				"Multi-Arch: same\n"+
				"Filename: pool/main/p/pkg-%06d/pkg-%06d_1.0.%d_amd64.deb\n"+
				"Size: %d\n"+
				"SHA256: 0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef\n"+
				"Depends: libfoo, libbar (>= 1.0), libbaz | libquux\n"+
				"Description: synthetic package %d\n"+
				" extended description line\n"+
				"\n",
			i, i, i, i, i, 1000+i, i,
		)
	}

	return b.String()
}

// BenchmarkParseDeb822Serial parses N synthetic Packages files
// sequentially — the pre-parallel loadAptLists behaviour.
func BenchmarkParseDeb822Serial(b *testing.B) {
	const numFiles = 16

	stanza := makePackagesStanza(2_000)

	b.ResetTimer()
	b.ReportAllocs()

	for range b.N {
		for range numFiles {
			c := aptcache.NewCacheForTesting()
			if err := c.ParseDeb822ForTesting(strings.NewReader(stanza), false); err != nil {
				b.Fatal(err)
			}
		}
	}
}

// BenchmarkParseDeb822Parallel mirrors Serial but parses concurrently,
// the way the new loadAptLists does. Speedup vs Serial should approach
// min(GOMAXPROCS, 8) on a multi-core host.
func BenchmarkParseDeb822Parallel(b *testing.B) {
	const numFiles = 16

	stanza := makePackagesStanza(2_000)

	b.ResetTimer()
	b.ReportAllocs()

	for range b.N {
		var wg sync.WaitGroup

		wg.Add(numFiles)

		for range numFiles {
			go func() {
				defer wg.Done()

				c := aptcache.NewCacheForTesting()
				_ = c.ParseDeb822ForTesting(strings.NewReader(stanza), false)
			}()
		}

		wg.Wait()
	}
}

// BenchmarkLoadAptListsParallel exercises the real loadAptLists code
// path against a temp dir full of synthetic Packages files. Includes
// file I/O, the worker pool, and the mergeFrom step — the most
// representative end-to-end measurement.
func BenchmarkLoadAptListsParallel(b *testing.B) {
	const numFiles = 16

	stanza := makePackagesStanza(2_000)

	dir := b.TempDir()

	for f := range numFiles {
		path := filepath.Join(dir,
			fmt.Sprintf("repo%02d.example.com_dists_jammy_main_binary-amd64_Packages", f))
		if err := os.WriteFile(path, []byte(stanza), 0o600); err != nil {
			b.Fatal(err)
		}
	}

	b.ResetTimer()
	b.ReportAllocs()

	for range b.N {
		c := aptcache.NewCacheForTesting()
		if err := aptcache.LoadAptListsForTesting(c, dir); err != nil {
			b.Fatal(err)
		}
	}
}
