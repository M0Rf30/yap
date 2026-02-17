package project_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/M0Rf30/yap/pkg/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
}`), os.FileMode(0755)))

	defer os.Remove(packageRaw)

	err := os.MkdirAll(filepath.Dir(prj1), os.FileMode(0750))
	if err != nil {
		t.Error(err)
	}

	defer os.RemoveAll(filepath.Dir(prj1))

	err = os.MkdirAll(filepath.Dir(prj2), os.FileMode(0750))
	if err != nil {
		t.Error(err)
	}

	defer os.Remove(filepath.Dir(prj2))

	err = os.WriteFile(prj1, []byte(examplePkgbuild), os.FileMode(0750))
	if err != nil {
		t.Error(err)
	}

	defer os.Remove(prj1)

	err = os.WriteFile(prj2, []byte(examplePkgbuild), os.FileMode(0750))
	if err != nil {
		t.Error(err)
	}

	defer os.Remove(prj2)

	project.SkipSyncDeps = true

	var mpc = project.MultipleProject{}

	err = mpc.MultiProject("ubuntu", "", testDir)
	require.NoError(t, err)
}

func TestMultiProjectVariableIsolation(t *testing.T) {
	t.Parallel()

	testDir := t.TempDir()

	pkgbuild1 := `
pkgname="altermime"
pkgver="0.3.11"
pkgrel="1"
pkgdesc="Altermime utility"
maintainer="Test <test@test.org>"
arch=("x86_64")
license=("GPL-3.0-only")
section="utils"
priority="optional"
url="https://example.com/${pkgname}"
source=("${url}/archive/${pkgver}.tar.gz")
sha256sums=("SKIP")

build() {
  echo "building ${pkgname} version ${pkgver}"
}

package() {
  mkdir -p "${pkgdir}/usr/bin"
}
`
	pkgbuild2 := `
pkgname="opendkim"
pkgver="2.11.0"
pkgrel="3"
pkgdesc="OpenDKIM utility"
maintainer="Test <test@test.org>"
arch=("x86_64")
license=("GPL-3.0-only")
section="utils"
priority="optional"
url="https://example.com/${pkgname}"
source=("${url}/archive/${pkgver}.tar.gz")
sha256sums=("SKIP")

build() {
  echo "building ${pkgname} version ${pkgver}"
}

package() {
  mkdir -p "${pkgdir}/usr/bin"
}
`

	packageRaw := filepath.Join(testDir, "yap.json")
	prj1Dir := filepath.Join(testDir, "altermime")
	prj2Dir := filepath.Join(testDir, "opendkim")

	require.NoError(t, os.WriteFile(packageRaw, fmt.Appendf(nil, `{
    "name": "test-isolation",
    "description": "Test variable isolation",
    "buildDir": "%s",
    "output": "%s",
    "projects": [
        {"name": "altermime", "install": false},
        {"name": "opendkim", "install": false}
    ]
}`, filepath.Join(testDir, "build"), filepath.Join(testDir, "output")), os.FileMode(0755)))

	require.NoError(t, os.MkdirAll(prj1Dir, os.FileMode(0750)))
	require.NoError(t, os.MkdirAll(prj2Dir, os.FileMode(0750)))
	require.NoError(t, os.WriteFile(filepath.Join(prj1Dir, "PKGBUILD"), []byte(pkgbuild1), os.FileMode(0750)))
	require.NoError(t, os.WriteFile(filepath.Join(prj2Dir, "PKGBUILD"), []byte(pkgbuild2), os.FileMode(0750)))

	project.SkipSyncDeps = true

	var mpc = project.MultipleProject{}

	err := mpc.MultiProject("ubuntu", "", testDir)
	require.NoError(t, err)

	// Verify that each project got its own pkgver, not leaked from the other.
	require.Len(t, mpc.Projects, 2)

	prj1 := mpc.Projects[0]
	prj2 := mpc.Projects[1]

	assert.Equal(t, "altermime", prj1.Builder.PKGBUILD.PkgName,
		"project1 pkgname should be altermime")
	assert.Equal(t, "0.3.11", prj1.Builder.PKGBUILD.PkgVer,
		"project1 pkgver should be 0.3.11")
	assert.Equal(t, "1", prj1.Builder.PKGBUILD.PkgRel,
		"project1 pkgrel should be 1")

	assert.Equal(t, "opendkim", prj2.Builder.PKGBUILD.PkgName,
		"project2 pkgname should be opendkim")
	assert.Equal(t, "2.11.0", prj2.Builder.PKGBUILD.PkgVer,
		"project2 pkgver should be 2.11.0")
	assert.Equal(t, "3", prj2.Builder.PKGBUILD.PkgRel,
		"project2 pkgrel should be 3")

	// Verify that the environment doesn't retain values from the last parsed project.
	// After parsing both projects, the env should have been cleaned between them.
	// The env will still have project2's values (the last one parsed), which is expected.
	// What matters is that project1's struct fields were NOT polluted by project2.
}

func TestMultiProjectCustomVariableIsolation(t *testing.T) {
	t.Parallel()

	testDir := t.TempDir()

	// Project 1 defines a custom variable "custom_flag" that project 2 does NOT define.
	// Without env cleanup, project 2's function bodies could inherit project 1's custom_flag.
	pkgbuild1 := `
pkgname="projA"
pkgver="1.0.0"
pkgrel="1"
pkgdesc="Project A"
maintainer="Test <test@test.org>"
arch=("x86_64")
license=("GPL-3.0-only")
section="utils"
priority="optional"
url="https://example.com"
custom_flag="from-projA"
source=("${url}/archive/${pkgver}.tar.gz")
sha256sums=("SKIP")

build() {
  echo "custom_flag is ${custom_flag}"
}

package() {
  mkdir -p "${pkgdir}/usr/bin"
}
`
	pkgbuild2 := `
pkgname="projB"
pkgver="2.0.0"
pkgrel="1"
pkgdesc="Project B"
maintainer="Test <test@test.org>"
arch=("x86_64")
license=("GPL-3.0-only")
section="utils"
priority="optional"
url="https://example.com"
source=("${url}/archive/${pkgver}.tar.gz")
sha256sums=("SKIP")

build() {
  echo "custom_flag is ${custom_flag}"
}

package() {
  mkdir -p "${pkgdir}/usr/bin"
}
`

	packageRaw := filepath.Join(testDir, "yap.json")
	prj1Dir := filepath.Join(testDir, "projA")
	prj2Dir := filepath.Join(testDir, "projB")

	require.NoError(t, os.WriteFile(packageRaw, fmt.Appendf(nil, `{
    "name": "test-custom-isolation",
    "description": "Test custom variable isolation",
    "buildDir": "%s",
    "output": "%s",
    "projects": [
        {"name": "projA", "install": false},
        {"name": "projB", "install": false}
    ]
}`, filepath.Join(testDir, "build"), filepath.Join(testDir, "output")), os.FileMode(0755)))

	require.NoError(t, os.MkdirAll(prj1Dir, os.FileMode(0750)))
	require.NoError(t, os.MkdirAll(prj2Dir, os.FileMode(0750)))
	require.NoError(t, os.WriteFile(filepath.Join(prj1Dir, "PKGBUILD"), []byte(pkgbuild1), os.FileMode(0750)))
	require.NoError(t, os.WriteFile(filepath.Join(prj2Dir, "PKGBUILD"), []byte(pkgbuild2), os.FileMode(0750)))

	project.SkipSyncDeps = true

	var mpc = project.MultipleProject{}

	err := mpc.MultiProject("ubuntu", "", testDir)
	require.NoError(t, err)

	require.Len(t, mpc.Projects, 2)

	prj2 := mpc.Projects[1]

	// Project B does NOT define custom_flag. Without env cleanup, its build()
	// body would contain "from-projA" baked in by os.ExpandEnv at parse time.
	// With the fix, custom_flag should be empty in project B's build body.
	assert.NotContains(t, prj2.Builder.PKGBUILD.Build, "from-projA",
		"project B's build() should not contain project A's custom_flag value")
}
