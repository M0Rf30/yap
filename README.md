# Yap

![yap-logo](assets/images/logo.png)

[![report card](https://img.shields.io/badge/report%20card-a%2B-ff3333.svg?style=flat-square)](http://goreportcard.com/report/M0Rf30/yap)
[![View examples](https://img.shields.io/badge/learn%20by-examples-0077b3.svg?style=flat-square)](examples)

## Introduction

Yap is a versatile tool designed to simplify the process of building packages
for multiple GNU/Linux distributions. It provides a consistent package
specification format, reducing the complexity typically associated with
multi-distribution package building.

## Key Features

- **OCI Container Builds:** Yap conducts builds on OCI containers, eliminating
  the need for setting up any virtual machines or installing any software other
  than Docker/Podman.
- **Simple Format:** Yap uses a simple format that is similar to [PKGBUILD](https://wiki.archlinux.org/index.php/PKGBUILD) from Arch Linux, making it easy to use and understand.
- **Consistent Build Process:** Though each Linux distribution requires different build instructions, Yap ensures a consistent build process and format across all builds.

## Quick start

To install latest release, follow the steps below:

```sh
# First, download the latest version of the software from the yap
# archives
wget https://github.com/M0Rf30/yap/releases/latest/download/yap_Linux_x86_64.tar.gz

# Next, extract the downloaded archive
tar -xvf yap_Linux_x86_64.tar.gz

# Move the extracted files to a directory in your PATH
sudo mv yap /usr/local/bin/

# Verify the installation
yap version
```

## Documentation

Detailed documentation and guidelines on how to use Yap are available on our
[wiki](https://github.com/M0Rf30/yap/wiki).

## Examples

To get a better understanding of how Yap works, you can refer to the examples
provided in the [examples](examples) folder. Here you'll find:
- [the project definition](examples/yap.json)
- [the spec file](examples/yap/PKGBUILD)

## License

Yap is licensed under the terms mentioned in the [LICENSE](LICENSE.md) file.

## Credits

We would like to express our gratitude to
[Zachary Huff](https://github.com/zachhuff386) for his significant contributions
to Pacur, the project on which Yap is based.