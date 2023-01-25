package project_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/M0Rf30/yap/project"
	"github.com/stretchr/testify/assert"
)

const examplePkgbuild = `pkgname="httpserver"
pkgver="1.0"
pkgrel="1"
pkgdesc="Http file server written with Go"
pkgdesc_centos="Http file server written with Go for CentOS"
pkgdesc_debian="Http file server written with Go for Debian"
pkgdesc_fedora="Http file server written with Go for Fedora"
pkgdesc_ubuntu="Http file server written with Go for Ubuntu"
maintainer="Example <example@yap.org>"
arch=("all")
license=("GPLv3")
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
    go build -a
}

package() {
    cd "${srcdir}/go/src/${pkgname}-${pkgver}"
    mkdir -p "${pkgdir}/usr/bin"
    cp ${pkgname}-${pkgver} ${pkgdir}/usr/bin/${pkgname}
}
`

func TestBuildMultipleProjectFromJSON(t *testing.T) {
	t.Parallel()

	testDir, err := os.MkdirTemp("", "TestBuildProjectFromJSON")
	if err != nil {
		t.Fatal(err)
	}

	defer os.RemoveAll(testDir)
	packageRaw := filepath.Join(testDir, "yap.json")
	prj1 := filepath.Join(testDir, "project1/", "PKGBUILD")
	prj2 := filepath.Join(testDir, "project2/", "PKGBUILD")

	assert.NoError(t, os.WriteFile(packageRaw, []byte(`{
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
}`), os.FileMode(0755)))

	defer os.Remove(packageRaw)
	err = os.MkdirAll(filepath.Dir(prj1), os.FileMode(0777))

	if err != nil {
		t.Error(err)
	}

	defer os.RemoveAll(filepath.Dir(prj1))
	err = os.MkdirAll(filepath.Dir(prj2), os.FileMode(0777))

	if err != nil {
		t.Error(err)
	}

	defer os.Remove(filepath.Dir(prj2))

	err = os.WriteFile(prj1, []byte(examplePkgbuild), os.FileMode(0755))
	if err != nil {
		t.Error(err)
	}

	defer os.Remove(prj1)

	err = os.WriteFile(prj2, []byte(examplePkgbuild), os.FileMode(0755))
	if err != nil {
		t.Error(err)
	}

	defer os.Remove(prj2)

	_, err = project.MultiProject("ubuntu", "", testDir, true)
	assert.NoError(t, err)
}
