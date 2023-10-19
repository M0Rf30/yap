# Yap

![yap-logo](https://raw.githubusercontent.com/M0Rf30/yap/main/images/yap.png)

[![report card](https://img.shields.io/badge/report%20card-a%2B-ff3333.svg?style=flat-square)](http://goreportcard.com/report/M0Rf30/yap)
[![View examples](https://img.shields.io/badge/learn%20by-examples-0077b3.svg?style=flat-square)](https://github.com/M0Rf30/yap/tree/main/examples)

Yap allows building packages for multiple GNU/Linux distributions with a
consistent package spec format.

Builds are done on OCI containers without needing to setup any virtual
machines or install any software other than Docker/Podman.

All packages are built using a simple format that is similar to
[PKGBUILD](https://wiki.archlinux.org/index.php/PKGBUILD) from Arch Linux.

Each distribution is different and will still require different build
instructions, but a consistent build process and format can be used for all
builds.

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
key__ubuntu="this will apply only to Ubuntu  builds"
```

## Builtin Variables

| key         | value                                                             |
| ----------- | ----------------------------------------------------------------- |
| `${srcdir}` | `Source` directory where all sources are downloaded and extracted |
| `${pkgdir}` | `Package` directory for the root of the package                   |

## Spec file - the PKGBUILD

| key                | type     | value                                                                                                                                                                                                                                                                                                                          |
|--------------------|----------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `pkgname`          | `string` | Package name                                                                                                                                                                                                                                                                                                                   |
| `epoch`            | `string` | Package epoch                                                                                                                                                                                                                                                                                                                  |
| `pkgver`           | `string` | Package version                                                                                                                                                                                                                                                                                                                |
| `pkgrel`           | `string` | Package release number                                                                                                                                                                                                                                                                                                         |
| `pkgdesc`          | `string` | Short package description                                                                                                                                                                                                                                                                                                      |
| `maintainer`       | `string` | Package maintainer                                                                                                                                                                                                                                                                                                             |
| `arch`             | `list`   | Package architecture, can be `any` or `x86_64`                                                                                                                                                                                                                                                                                 |
| `license`          | `list`   | List of licenses for packaged software                                                                                                                                                                                                                                                                                         |
| `section`          | `string` | Section for package. Built in sections available: `admin` `localization` `mail` `comm` `math` `database` `misc` `debug` `net` `news` `devel` `doc` `editors` `electronics` `embedded` `fonts` `games` `science` `shells` `sound` `graphics` `text` `httpd` `vcs` `interpreters` `video` `web` `kernel` `x11` `libdevel` `libs` |
| `priority`         | `string` | Package priority, only used for Debian packages                                                                                                                                                                                                                                                                                |
| `url`              | `string` | Package url                                                                                                                                                                                                                                                                                                                    |
| `depends`          | `list`   | List of package dependencies                                                                                                                                                                                                                                                                                                   |
| `optdepends`       | `list`   | List of package optional dependencies                                                                                                                                                                                                                                                                                          |
| `makedepends`      | `list`   | List of package build dependencies                                                                                                                                                                                                                                                                                             |
| `provides`         | `list`   | List of packages provided                                                                                                                                                                                                                                                                                                      |
| `conflicts`        | `list`   | List of packages conflicts                                                                                                                                                                                                                                                                                                     |
| `source`           | `list`   | List of packages sources. Sources can be url or paths that are relative to the PKGBUILD                                                                                                                                                                                                                                        |
| `debconf_config`   | `string` | File used as debconf config, only used for Debian packages                                                                                                                                                                                                                                                                     |
| `debconf_template` | `string` | File used as debconf template, only used for Debian packages                                                                                                                                                                                                                                                                   |
| `hashsums`         | `list`   | List of `sha256`/`sha512` hex hashes for sources, hash type is determined by the length of the hash. Use `SKIP` to ignore hash check                                                                                                                                                                                           |
| `backup`           | `list`   | List of config files that shouldn't be overwritten on upgrades                                                                                                                                                                                                                                                                 |
| `build`            | `func`   | Function to build the source, starts in srcdir                                                                                                                                                                                                                                                                                 |
| `package`          | `func`   | Function to package the source into the pkgdir, starts in srcdir                                                                                                                                                                                                                                                               |
| `preinst`          | `func`   | Function to run before installing                                                                                                                                                                                                                                                                                              |
| `postinst`         | `func`   | Function to run after installing                                                                                                                                                                                                                                                                                               |
| `prerm`            | `func`   | Function to run before removing                                                                                                                                                                                                                                                                                                |
| `postrm`           | `func`   | Function to run after removing                                                                                                                                                                                                                                                                                                 |

### Build targets

| target           | value                     |
|------------------|---------------------------|
| `alpine`         | all Alpine Linux releases |
| `arch`           | all Arch Linux releases   |
| `amazon`         | all Amazon Linux releases |
| `centos`         | all CentOS releases       |
| `debian`         | all Debian releases       |
| `fedora`         | all Fedora releases       |
| `oracle`         | all Oracle Linux releases |
| `ubuntu`         | all Ubuntu releases       |
| `amazon-1`       | Amazon Linux 1            |
| `amazon-2`       | Amazon Linux 2            |
| `debian-jessie`  | Debian Jessie             |
| `debian-stretch` | Debian Stretch            |
| `debian-buster`  | Debian Buster             |
| `fedora-38`      | Fedora 38                 |
| `rocky-8`        | Rocky Linux 8             |
| `rocky-9`        | Rocky Linux 9             |
| `ubuntu-bionic`  | Ubuntu Bionic             |
| `ubuntu-focal`   | Ubuntu Focal              |
| `ubuntu-jammy`   | Ubuntu Jammy              |

### Directives

Directives are used to specify variables that only apply to a limited set of
build targets.

All variables can use directives including user defined variables.

To use directives include the directive after a variable separated by a colon
such as `pkgdesc__ubuntu="This description will only apply to Ubuntu packages"`.

The directives above are sorted from lowest to the highest priority.

| directive        | value                     |
|------------------|---------------------------|
| `apk`            | all apk packages          |
| `apt`            | all deb packages          |
| `pacman`         | all pkg packages          |
| `yum`            | all yum rpm packages      |
| `alpine`         | all Alpine Linux packages |
| `arch`           | all Arch Linux releases   |
| `amazon`         | all Amazon Linux releases |
| `centos`         | all CentOS releases       |
| `debian`         | all Debian releases       |
| `fedora`         | all Fedora releases       |
| `oracle`         | all Oracle Linux releases |
| `ubuntu`         | all Ubuntu releases       |
| `amazon_1`       | Amazon Linux 1            |
| `amazon_2`       | Amazon Linux 2            |
| `debian_jessie`  | Debian Jessie             |
| `debian_stretch` | Debian Stretch            |
| `debian_buster`  | Debian Buster             |
| `fedora_38`      | Fedora 38                 |
| `rocky_8`        | Rocky Linux 8             |
| `rocky_9`        | Rocky Linux 9             |
| `ubuntu_bionic`  | Ubuntu Bionic             |
| `ubuntu_focal`   | Ubuntu Focal              |
| `ubuntu_jammy`   | Ubuntu Jammy              |

## Examples

Please have a look under the `examples` folder.

You'll find:

- [the project definition](examples/yap.json)
- [the spec file](examples/yap/PKGBUILD)

## License

See [LICENSE](LICENSE.md) file for details.

## Credits

[Zachary Huff](https://github.com/zachhuff386), for his work on Pacur, on which
Yap is based on.
