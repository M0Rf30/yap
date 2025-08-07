package project_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/M0Rf30/yap/pkg/project"
)

const examplePkgbuild = `
pkgname="httpserver"
pkgver="1.0"
pkgrel="1"
pkgdesc="Http file server written with Go"
pkgdesc__debian="Http file server written with Go for Debian"
pkgdesc__fedora="Http file server written with Go for Fedora"
pkgdesc__rocky="Http file server written with Go for Rocky"
pkgdesc__ubuntu="Http file server written with Go for Ubuntu"
maintainer="Example <example@yap.org>"
arch=("x86_64")
license=("GPL-3.0-only")
section="utils"
priority="optional"
url="https://github.com/M0Rf30/${pkgname}"
source=(
  "${url}/archive/${pkgver}.tar.gz"
)
sha256sums=(
  "SKIP"
)

build() {
  export GO111MODULE=off
  mkdir -p "go/src"
  export GOPATH="${srcdir}/go"
  mv "${pkgname}-${pkgver}" "go/src"
  cd "go/src/${pkgname}-${pkgver}"
  go get
  go build cmd
}

package() {
  cd "${srcdir}/go/src/${pkgname}-${pkgver}"
  mkdir -p "${pkgdir}/usr/bin"
  cp ${pkgname}-${pkgver} ${pkgdir}/usr/bin/${pkgname}
}
`

func TestBuildMultipleProjectFromJSON(t *testing.T) {
	t.Parallel()

	testDir := t.TempDir()

	packageRaw := filepath.Join(testDir, "yap.json")
	prj1 := filepath.Join(testDir, "project1", "PKGBUILD")
	prj2 := filepath.Join(testDir, "project2", "PKGBUILD")

	require.NoError(t, os.WriteFile(packageRaw, []byte(`{
    "name": "A test",
    "description": "The test description",
	"buildDir": "/tmp/",
	"output": "/tmp/fake-path/",
    "projects": [
        {
            "name": "project1",
			"install": true
        },
        {
            "name": "project2",
			"install": false
        }
    ]
}`), os.FileMode(0o755)))

	defer func() {
		err := os.Remove(packageRaw)
		if err != nil {
			t.Logf("Failed to remove file %s: %v", packageRaw, err)
		}
	}()

	err := os.MkdirAll(filepath.Dir(prj1), os.FileMode(0o750))
	if err != nil {
		t.Error(err)
	}

	defer func() {
		err := os.RemoveAll(filepath.Dir(prj1))
		if err != nil {
			t.Logf("Failed to remove directory %s: %v", filepath.Dir(prj1), err)
		}
	}()

	err = os.MkdirAll(filepath.Dir(prj2), os.FileMode(0o750))
	if err != nil {
		t.Error(err)
	}

	defer func() {
		err := os.Remove(filepath.Dir(prj2))
		if err != nil {
			t.Logf("Failed to remove directory %s: %v", filepath.Dir(prj2), err)
		}
	}()

	err = os.WriteFile(prj1, []byte(examplePkgbuild), os.FileMode(0o750))
	if err != nil {
		t.Error(err)
	}

	defer func() {
		err := os.Remove(prj1)
		if err != nil {
			t.Logf("Failed to remove file %s: %v", prj1, err)
		}
	}()

	err = os.WriteFile(prj2, []byte(examplePkgbuild), os.FileMode(0o750))
	if err != nil {
		t.Error(err)
	}

	defer func() {
		err := os.Remove(prj2)
		if err != nil {
			t.Logf("Failed to remove file %s: %v", prj2, err)
		}
	}()

	project.SkipSyncDeps = true

	mpc := project.MultipleProject{}

	err = mpc.MultiProject("ubuntu", "", testDir)
	require.NoError(t, err)
}
