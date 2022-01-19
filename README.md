# Yap: Simple Packaging

![Arch Linux](https://img.shields.io/badge/docker-archlinux-1794d1.svg?style=flat "Arch Linux")

![Amazon Linux 1](https://img.shields.io/badge/docker-amazonlinux--1-faaf34.svg?style=flat "Amazon Linux 1")
![Amazon Linux 2](https://img.shields.io/badge/docker-amazonlinux--2-faaf34.svg?style=flat "Amazon Linux 2")

![CentOS 8](https://img.shields.io/badge/docker-centos--8-10233f.svg?style=flat "CentOS 8")

![Debian Jessie](https://img.shields.io/badge/docker-debian--jessie-d70a53.svg?style=flat "Debian Jessie")
![Debian Stretch](https://img.shields.io/badge/docker-debian--stretch-d70a53.svg?style=flat "Debian Stretch")
![Debian Buster](https://img.shields.io/badge/docker-debian--buster-d70a53.svg?style=flat "Debian Buster")

![Oracle Linux 8](https://img.shields.io/badge/docker-oraclelinux--8-f82200.svg?style=flat "Oracle Linux 8")

![Fedora 33](https://img.shields.io/badge/docker-fedora--33-294172.svg?style=flat "Fedora 33")

![Ubuntu Xenial](https://img.shields.io/badge/docker-ubuntu--xenial-dd4814.svg?style=flat "Ubuntu Xenial")
![Ubuntu Bionic](https://img.shields.io/badge/docker-ubuntu--bionic-dd4814.svg?style=flat "Ubuntu Bionic")
![Ubuntu Focal](https://img.shields.io/badge/docker-ubuntu--focal-dd4814.svg?style=flat "Ubuntu Focal")

Yap allows building packages for multiple linux distributions with a
consistent package spec format. Currently `deb`, `rpm` and `pacman` packages
are available for several linux distributions. Builds are done on Docker
containers without needing to setup any virtual machines or install any
software other than Docker. All packages are built using a simple format that
is similar to [PKGBUILD](https://wiki.archlinux.org/index.php/PKGBUILD) from
ArchLinux. Each distribution is different and will still require different
build instructions, but a consistent build process and format can be used for
all builds. Docker only supports 64-bit containers, yap can't be used to
build packages 32-bit packages. Yap will also create a `deb`, `rpm` and
`pacman` signed repository that can be used on ArchLinux, CentOS, Fedora,
Debian and Ubuntu to distribute the packages.

## Initialize

It is recommended to build the docker images locally instead of pulling each
image from the Docker Hub. A script is located in the docker directory to
assist with this. Always run the `clean.sh` script to clear any existing yap
images. Building the images can take several hours.

```sh
cd ~/go/src/github.com/packagefoundation/yap/docker
sh clean.sh
sh build.sh
```

## Format

```sh
key="example string"
key=`example "quoted" string`
key=("list with one element")
key=(
    "list with"
    "multiple elements"
)
key="example ${variable} string"
key:ubuntu="this will apply only to ubuntu builds"
```

## Builtin Variables

| key | value |
| --- | ----- |
| `${srcdir}` | `Source` directory where all sources are downloaded and extracted |
| `${pkgdir}` | `Package` directory for the root of the package |

## Spec

| key | type | value |
| --- | ---- | ----- |
| `targets` | `list` | List of build targets only used for projects. Prefix a `!` to ignore target. |
| `pkgname` | `string` | Package name |
| `pkgver` | `string` | Package version |
| `pkgrel` | `string` | Package release number |
| `pkgdesc` | `string` | Short package description |
| `pkgdesclong` | `list` | List of lines for package description |
| `maintainer` | `string` | Package maintainer |
| `arch` | `string` | Package architecture, can be `all` or `amd64` |
| `license` | `list` | List of licenses for packaged software |
| `section` | `string` | Section for package. Built in sections available:<br> `admin`<br> `localization`<br> `mail`<br> `comm`<br> `math`<br> `database`<br> `misc`<br> `debug`<br> `net`<br> `news`<br> `devel`<br> `doc`<br> `editors`<br> `electronics`<br> `embedded`<br> `fonts`<br> `games`<br> `science`<br> `shells`<br> `sound`<br> `graphics`<br> `text`<br> `httpd`<br> `vcs`<br> `interpreters`<br> `video`<br> `web`<br> `kernel`<br> `x11`<br> `libdevel`<br> `libs` |
| `priority` | `string` | Package priority, only used for debian packages |
| `url` | `string` | Package url |
| `depends` | `list` | List of package dependencies |
| `optdepends` | `list` | List of package optional dependencies |
| `makedepends` | `list` | List of package build dependencies |
| `provides` | `list` | List of packages provided |
| `conflicts` | `list` | List of packages conflicts |
| `sources` | `list` | List of packages sources. Sources can be url or paths that are relative to the PKGBUILD |
| `debconf_config` | `string` | File used as debconf config, only used for debian packages |
| `debconf_template` | `string` | File used as debconf template, only used for debian packages |
| `hashsums` | `list` | List of `md5`/`sha1`/`sha256`/`sha512` hex hashes for sources, hash type is determined by the length of the hash. Use `skip` to ignore hash check |
| `backup` | `list` | List of config files that shouldn't be overwritten on upgrades |
| `build` | `func` | Function to build the source, starts in srcdir |
| `package` | `func` | Function to package the source into the pkgdir, starts in srcdir |
| `preinst` | `func` | Function to run before installing |
| `postinst` | `func` | Function to run after installing |
| `prerm` | `func` | Function to run before removing |
| `postrm` | `func` | Function to run after removing |

### build targets

| target | value |
| ------ | ----- |
| `arch` | All archlinux releases |
| `astra` | All astralinux releases |
| `amazon` | All amazonlinux releases |
| `centos` | All centos releases |
| `debian` | All debian releases |
| `fedora` | All fedora releases |
| `oracle` | All oraclelinux releases |
| `ubuntu` | All ubuntu releases |
| `amazon-1` | Amazonlinux 1 |
| `amazon-2` | Amazonlinux 2 |
| `centos-7` | Centos 7 |
| `centos-8` | Centos 8 |
| `debian-jessie` | Debian jessie |
| `debian-stretch` | Debian stretch |
| `debian-buster` | Debian buster |
| `fedora-32` | Fedora 32 |
| `fedora-33` | Fedora 33 |
| `oracle-7` | Oraclelinux 7 |
| `rocky-8` | Rockylinux 8 |
| `oracle-8` | Oraclelinux 8 |
| `ubuntu-bionic` | Ubuntu bionic |
| `ubuntu-focal` | Ubuntu focal |

### directives

| directive | value |
| --------- | ----- |
| `apt` | All deb packages |
| `pacman` | All pkg packages |
| `yum` | All rpm packages |
| `arch` | All archlinux releases |
| `amazonlinux` | All amazonlinux releases |
| `centos` | All centos releases |
| `debian` | All debian releases |
| `fedora` | All fedora releases |
| `oracle` | All oraclelinux releases |
| `ubuntu` | All ubuntu releases |
| `amazon-1` | Amazonlinux 1 |
| `amazon-2` | Amazonlinux 2 |
| `centos-8` | Centos 8 |
| `debian-jessie` | Debian jessie |
| `debian-stretch` | Debian stretch |
| `debian-buster` | Debian buster |
| `fedora-32` | Fedora 32 |
| `fedora-33` | Fedora 33 |
| `oracle-8` | Oraclelinux 8 |
| `rocky-8` | Rockylinux 8 |
| `ubuntu-xenial` | Ubuntu xenial |
| `ubuntu-bionic` | Ubuntu bionic |
| `ubuntu-focal` | Ubuntu focal |

Directives are used to specify variables that only apply to a limited set of
build targets. All variables can use directives including user defined
variables. To use directives include the directive after a
variable separated by a colon such as
`pkgdesc:ubuntu="This description will only apply to Ubuntu packages"`.
The directives above are sorted from lowest to the highest priority.

## Example

First create a directory for the PKGBUILD file. This directory should only
contain the PKGBUILD file and any other files needed such as patches. Then
create a PKGBUILD the package directory. After creating the PKGBUILD build
the package with docker.

```sh
mkdir httpserver
cd httpserver
nano PKGBUILD
docker run --rm -t -v $(pwd):/yap yap/ubuntu-bionic
```

```sh
targets=(
    "arch"
    "centos"
    "debian"
    "ubuntu"
)
pkgname="httpserver"
pkgver="1.0"
pkgrel="1"
pkgdesc="Http file server written with Go"
pkgdesc:centos="Http file server written with Go for CentOS"
pkgdesc:debian="Http file server written with Go for Debian"
pkgdesc:fedora="Http file server written with Go for Fedora"
pkgdesc:ubuntu="Http file server written with Go for Ubuntu"
pkgdesclong=(
    "Quick http file server written with Go"
    "using directory listing similar to apache"
)
maintainer="example <example@packagefoundation.org>"
arch="all"
license=("GPLv3")
section="utils"
priority="optional"
url="https://github.com/yap/${pkgname}"
sources=(
    "${url}/archive/${pkgver}.tar.gz"
)
hashsums=(
    "3548e1263a931b27970e190f04b74623"
)

build() {
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
```

## Project Example

A project can be created with the cli tools which can be installed using
go get. The packages can be built and added to the repo. An example project is
available in the example directory. The `pull` command should be run before
all builds to update the docker images used for builds.

```sh
go get github.com/packagefoundation/yap
yap build ubuntu example
```
