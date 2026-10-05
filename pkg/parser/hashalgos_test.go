package parser_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/M0Rf30/yap/v2/pkg/parser"
)

func TestParseFile_HashAlgosTracked(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	content := `pkgname="algo-test"
pkgver="1.0"
pkgrel="1"
pkgdesc="d"
arch=('x86_64')
license=('MIT')
source=("a.txt" "b.txt")
b2sums=('aaaa' 'bbbb')
package() {
  true
}
`

	if err := os.WriteFile(filepath.Join(dir, "PKGBUILD"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	pb, err := parser.ParseFile("ubuntu", "jammy", dir, dir, "x86_64")
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}

	if len(pb.HashSums) != 2 || pb.HashSums[0] != "aaaa" || pb.HashSums[1] != "bbbb" {
		t.Fatalf("HashSums = %v", pb.HashSums)
	}

	if len(pb.HashAlgos) != 2 || pb.HashAlgos[0] != "b2sums" || pb.HashAlgos[1] != "b2sums" {
		t.Errorf("HashAlgos = %v, want [b2sums b2sums]", pb.HashAlgos)
	}
}
