// Package yap provides a powerful, container-based package building system
// that creates packages for multiple GNU/Linux distributions from a single
// PKGBUILD-like specification format.
//
// YAP (Yet Another Packager) eliminates the need to learn multiple packaging
// formats by providing a unified interface based on the familiar PKGBUILD
// format from Arch Linux, extended with multi-distribution support and
// modern container-based builds.
//
// # Key Features
//
//   - Multi-format package support: RPM, DEB, APK, TAR.ZST packages
//   - Container-based isolation for clean, reproducible builds
//   - Dependency-aware build orchestration with project management
//   - PKGBUILD compatibility with enhanced parsing capabilities
//   - Component-aware structured logging system
//
// # Supported Distributions
//
// YAP supports building packages for Alpine, Arch, CentOS, Debian, Fedora,
// OpenSUSE, Rocky, Ubuntu, and more.
//
// # Usage
//
// The primary way to use YAP is through its command-line interface:
//
//	// Build for current system distribution
//	yap build .
//
//	// Build for specific distribution
//	yap build ubuntu-jammy /path/to/project
//
//	// Prepare build environment
//	yap prepare fedora-38
//
// # Package Structure
//
// YAP is organized into several key packages:
//
//   - pkg/project: Project configuration and orchestration
//   - pkg/parser: PKGBUILD parsing and processing
//   - pkg/packer: Package format implementations (RPM, DEB, APK, etc.)
//   - pkg/source: Source code retrieval and management
//   - pkg/builder: Container-based build execution
//   - pkg/logger: Component-aware structured logging
//   - pkg/osutils: Operating system utilities and helpers
//
// For detailed documentation and examples, visit https://github.com/M0Rf30/yap
package yap
