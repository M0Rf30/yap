# YAP Agent Guidelines

## Project Overview

**YAP (Yet Another Packager)** is a modern, cross-distribution package building tool that creates native packages for multiple GNU/Linux distributions from a single PKGBUILD specification. The project is written in Go and uses OCI containers (Docker/Podman) for isolated, reproducible builds across 15 supported distributions.

### Key Components
- **CLI Interface** (`cmd/yap/`) - Cobra-based command-line interface
- **Project Orchestration** (`pkg/project/`) - `MultipleProject`, `BuildAll`, multi-package coordination
- **Project Configuration** (`pkg/core/`) - Project config struct (`yap.json`)
- **Package Building** (`pkg/builders/`, `pkg/builders/common`) - Distribution-specific builders (APK, DEB, RPM, Pacman) and shared builder helpers
- **Packer Dispatch** (`pkg/packer/`) - Selects the right builder per distro/package manager
- **Container Management** - OCI container orchestration for isolated builds
- **Dependency Resolution** (`pkg/graph/`) - Build order calculation and dependency management
- **Source Handling** (`pkg/source/`) - Download and validation of source files
- **PKGBUILD Parsing** (`pkg/pkgbuild/`, `pkg/parser/`) - Extended PKGBUILD format support
- **APT Cache** (`pkg/aptcache/`) - Pure-Go deb822 parser for `/var/lib/apt/lists` and `/var/lib/dpkg/status`; replaces `apt-cache`/`dpkg` subprocess calls during cross-compilation dep partitioning
- **Color** (`pkg/color/`) - Zero-dependency ANSI color helpers used by logger, progress bar, and CLI output
- **Build Info** (`pkg/buildinfo/`) - Build-time metadata (Version, Commit, BuildTime) injected via `-ldflags`

### Architecture
```
┌─────────────────┐    ┌────────────────────────┐    ┌─────────────────┐
│   CLI Layer     │───▶│  Project Orchestration │───▶│  Builders       │
│   (cmd/yap)     │    │  (pkg/project,         │    │  (pkg/builders, │
│                 │    │   pkg/core,            │    │   pkg/packer)   │
│                 │    │   pkg/builder)         │    │                 │
└─────────────────┘    └────────────────────────┘    └─────────────────┘
                                  │
                                  ▼
┌─────────────────┐    ┌────────────────────────┐    ┌─────────────────┐
│   Containers    │◀───│  Graph / Deps          │───▶│  Source Mgmt    │
│   (Docker/Pod)  │    │  (pkg/graph)           │    │  (pkg/source)   │
└─────────────────┘    └────────────────────────┘    └─────────────────┘
```

## Build/Test Commands

### Primary Commands
- `make all` - Complete workflow: clean, deps, fmt, lint, test, doc, build
- `make build` - Build the yap binary with version info and optimizations
- `make build-all` - Build for multiple architectures (linux, darwin, windows)
- `make clean` - Clean build artifacts and temporary files
- `make deps` - Download and tidy Go module dependencies
- `make fmt` - Format code with gofmt
- `make lint` - Run golangci-lint with comprehensive checks
- `make lint-md` - Lint markdown files
- `make release` - Create release packages with full validation
- `make run` - Build and run yap with current changes
- `make test` - Run all tests with `-p 1 -v` flags (sequential execution required)
- `make test-coverage` - Run tests with coverage report (generates coverage.html)

### YAP CLI Commands
```bash
# Build packages
yap build <distro> <project-path>              # Build for specific distribution
yap build <project-path>                       # Auto-detect host distro from /etc/os-release

# Project management
yap zap [distro] <project-path>                # Clean build environment (distro optional)
yap prepare [distro[-release]]                 # Prepare host build env (distro optional)
yap pull <distro>                              # Pull pre-built container images
yap install <artifact-file>                    # Install a built artifact (.deb/.rpm/.apk/.pkg.tar.*)
yap list-distros                               # List supported distributions
yap status                                     # Show host status and runtime detection
yap graph <project-path>                       # Show dependency graph

# Package operations
yap build --skip-sync <project-path>           # Skip dependency sync (faster)
yap build --cleanbuild <project-path>          # Clean source before build
yap build --parallel <project-path>            # Enable parallel dep-aware topo-sort (opt-in)

# Signing
yap build --sign <project>                     # Enable artifact signing (key resolved from flags/env/config/default)
yap build --sign-key /path/to/private.key # Specify private key path
yap build --sign-passphrase pass          # Passphrase (use env vars in CI: YAP_SIGN_PASSPHRASE)
yap build --sign-key-name mykey           # APK key name (.SIGN.RSA.<keyname>.rsa.pub)

# SBOM (Software Bill of Materials)
yap build --sbom                          # Generate SBOM sidecars next to artifacts
yap build --sbom-format cyclonedx         # Only CycloneDX 1.5 (default: both)
yap build --sbom-format spdx              # Only SPDX 2.3
yap build --sbom-format both              # Both formats (default)

# Compression overrides
yap build --compression-deb gzip          # DEB compression: zstd|gzip|xz (default: zstd)
yap build --compression-rpm xz            # RPM compression: zstd|gzip|xz (default: zstd)
```


### Package-Specific Testing
```bash
# Test specific packages
go test ./pkg/source -v                    # Source handling tests
go test ./pkg/builders/deb -v              # Debian package builder tests
go test ./pkg/graph -v                     # Dependency graph tests
go test ./cmd/yap/command -v               # CLI command tests

# Test with race detection
go test -race ./pkg/builders/...

# Test with timeout
go test -timeout 30s ./pkg/download/...
```

### Documentation Commands
- `make doc` - View all package documentation
- `make doc-deps` - Install documentation dependencies (pkgsite)
- `make doc-generate` - Generate static documentation files in docs/api/
- `make doc-package PKG=./pkg/builders/apk` - View specific package documentation
- `make doc-serve` - Start documentation server on localhost:8080
- `make doc-serve-static` - Serve static documentation files on localhost:8081

## Code Style

### Module and Imports
- **Module**: `github.com/M0Rf30/yap/v2` 
- **Import grouping**: Standard → Third-party → Local (with goimports)
- **Local prefix**: `github.com/M0Rf30/yap/v2`

### Code Standards
- **Line length**: Max 100 characters (enforced by golangci-lint)
- **Error handling**: Use custom `pkg/errors` package with typed errors and context
- **Types**: Use proper Go types, prefer `any` over `interface{}`
- **Naming**: Follow Go conventions - exported PascalCase, private camelCase
- **Comments**: Package comments required, all exported functions documented
- **Complexity**: Max cyclomatic complexity of 15 (monitored by gocyclo)

### Error Handling Patterns
```go
// Use typed errors from pkg/errors
if err := operation(); err != nil {
    return errors.Wrap(err, errors.ErrTypeBuild, "failed to perform operation").
        WithOperation("BuildPackage").
        WithContext("config_path", configPath)
}

// Add context to errors
return errors.New(errors.ErrTypeConfiguration, "invalid configuration").
    WithContext("config_path", configPath).
    WithOperation("LoadConfig")
```

### Struct Validation
```go
type Config struct {
    Name        string `json:"name" validate:"required"`
    BuildDir    string `json:"buildDir" validate:"required,dir"`
    Projects    []Project `json:"projects" validate:"required,min=1"`
}
```

## Key Patterns

### Logging
- Use structured logging with `pkg/logger`
- Pass key-value pairs as flat variadic args after the message
- Use appropriate log levels (DEBUG, INFO, WARN, ERROR)
- Long lines auto-wrap into a tree layout (├/└) at terminal width

```go
logger.Info("Building package", "package", pkgName, "distro", distro)
logger.Warn("Skipping strip", "binary", path, "reason", "foreign arch")
logger.Error("Build failed", "error", err, "duration", elapsed)
```

### Error Wrapping
- Wrap errors with context using `errors.Wrap()` and `.WithOperation()`
- Add relevant fields for debugging
- Maintain error chain for troubleshooting

### Platform Abstraction
- Use `pkg/platform` for OS-specific operations
- Handle file permissions, ownership, and path separators appropriately
- Test on multiple platforms when possible

### Package Builder Interface
Each format (APK, DEB, RPM, Pacman) exposes the same two-method surface invoked by `pkg/packer`:

```go
// Implemented by pkg/builders/{apk,deb,rpm,pacman}
type Packer interface {
    PrepareFakeroot(artifactsPath, targetArch string) error
    BuildPackage(artifactsPath, targetArch string) error
}
```

Shared helpers live in `pkg/builders/common` (`BaseBuilder`, metadata/scriptlet helpers).
See `pkg/builders/apk/apk.go`, `pkg/builders/deb/deb.go`, `pkg/builders/rpm/rpm.go`,
`pkg/builders/pacman/pacman.go` for the concrete implementations.

### Container Patterns
- Use OCI-compliant containers for all builds
- Mount source and build directories appropriately
- Handle container lifecycle (create, start, copy, cleanup)
- Support both Docker and Podman runtimes

## Development Workflow

### 1. Setup Development Environment
```bash
# Clone and setup
git clone https://github.com/M0Rf30/yap.git
cd yap
make deps

# Install development tools
make doc-deps
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
```

### 2. Making Changes
```bash
# Format and lint before committing
make fmt lint

# Run tests
make test

# Test specific functionality
go test ./pkg/builders/deb -run TestDebBuilder
```

### 3. Testing Changes
```bash
# Test with example projects
yap build examples/yap
yap build examples/dependency-orchestration

# Test specific distributions (use available distributions)
yap build ubuntu examples/yap
yap build fedora examples/dependency-orchestration
yap build alpine examples/yap
```

### 4. Debugging
```bash
# Enable build logging (use -v for more details during builds)
yap build alpine examples/yap

# Clean and retry
yap zap examples/yap

# Check available distributions
yap list-distros
```

## Docker/Container Development

### Building Container Images
```bash
# List available distributions
make docker-list-distros

# Build specific distribution
make docker-build DISTRO=ubuntu
make docker-build DISTRO=fedora

# Build all distributions
make docker-build-all
```

### Container Testing
```bash
# Test container functionality
docker run --rm yap:ubuntu-latest /bin/bash
docker run --rm yap:fedora-latest /bin/bash

# Debug container builds
docker build --progress=plain --no-cache -f build/deploy/alpine/Dockerfile .
```

### Container Patterns in Code
- Always use context.Context for cancellation
- Implement proper cleanup with defer statements
- Handle both Docker and Podman APIs
- Use volume mounts for source code and build artifacts

## Testing Strategies

### Test Categories
1. **Unit Tests** - Individual package functionality
2. **Integration Tests** - Multi-package interactions
3. **Container Tests** - Build environment validation
4. **End-to-End Tests** - Complete package building workflows
5. **Format Compliance Tests** - Verify package format against distribution standards

### Coverage Requirements
- Minimum 70% coverage for new code
- Critical paths (builders, parsers) should have 85%+ coverage
- Use `make test-coverage` to generate reports

### Test Data
- Use `examples/` directory for realistic test scenarios
- Mock external dependencies (downloads, containers) in unit tests
- Use temporary directories for file system tests
- Compare with official distribution packages for format validation

### Format Validation Testing
```bash
# APK Format Testing
yap build alpine examples/yap                  # Build a YAP-produced APK
gunzip -c package.apk | dd bs=1 skip=257 count=8 2>/dev/null | od -A n -t x1z
                                               # Expect: 75 73 74 61 72 00 30 30 (PAX/ustar)

# DEB Format Testing
dpkg-deb --info package.deb                    # Validate metadata
dpkg-deb --contents package.deb                # Check file structure

# RPM Format Testing
rpm -qip package.rpm                           # Query package info
rpm2cpio package.rpm | cpio -tv                # Extract and list

# Pacman Format Testing
tar -tzf package.pkg.tar.zst                   # List package contents
pacman -Qip package.pkg.tar.zst                # Query package
```

### Continuous Integration
- Tests run on Go 1.26.1 (per `go.mod` — `go 1.26.0` directive, `toolchain go1.26.1`)
- Multi-architecture builds (amd64, arm64) on Ubuntu runners
- Integration with GitHub Actions
- Format compliance checks for all package types

## Package Builder Development

### Adding New Distribution Support
1. Create builder in `pkg/builders/newformat/`
2. Implement the Builder interface
3. Add container configuration in `build/deploy/newdistro/`
4. Update distribution constants in `pkg/constants/`
5. Add tests in `pkg/builders/newformat/`

### Builder Implementation Checklist
- [ ] Implement all Builder interface methods
- [ ] Handle distribution-specific variables (e.g., `pkgdesc__newdistro`)
- [ ] Support architecture-specific overrides
- [ ] Validate package metadata
- [ ] Generate correct package format
- [ ] Write comprehensive tests
- [ ] Document any special requirements

### APK Builder Compliance Status

The APK builder (`pkg/builders/apk/`) targets Alpine Linux APK compatibility. Current status reflects the actual code in `pkg/builders/apk/apk.go`:

#### ✅ Completed Features
- **Tar Format**: PAX format (POSIX.1-2001) matching Alpine's `abuild-tar`
- **Two-Stream Concatenated Gzip**: `control.tar.gz` + `data.tar.gz` produced by `createTarGzWithChecksums` — installable with `apk add --allow-untrusted`
- **PAX Extended Headers**: `APK-TOOLS.checksum.SHA1` per-file headers (`writeFileWithChecksum`) consumed by `apk audit`
- **`.PKGINFO` Metadata**: name, version, arch, description, license, url, dependencies, conflicts, `size`, `origin`, `builddate`, `datahash` (SHA-256 of `data.tar.gz`)
- **Dependency Handling**: depends, makedepends, conflicts, provides
- **RSA Signature Stream** (`.SIGN.RSA.<keyname>.rsa.pub`): PKCS#1 v1.5 SHA1 signature over control.tar.gz, prepended as third concatenated gzip stream — enables `apk add` without `--allow-untrusted`. Implementation: `pkg/signing/rsa.go`.

#### 🚧 In Progress / Planned Features
1. **Optional Extended Metadata** — Low Priority
   - `commit` (Git commit hash) — currently not emitted
   - Per-file ACL / xattr headers (rare in practice)
2. **Test Coverage Expansion** — Ongoing
   - Current: ~70%, target: 85%

#### Testing APK Packages
```bash
# Download an official Alpine package for byte-level comparison
wget http://dl-cdn.alpinelinux.org/alpine/v3.22/main/x86_64/busybox-1.37.0-r19.apk

# Build YAP APK package
yap build alpine examples/yap

# Verify PAX tar format magic bytes
gunzip -c package.apk | dd bs=1 skip=257 count=8 2>/dev/null | od -A n -t x1z
# Expected: 75 73 74 61 72 00 30 30 ("ustar\0" + "00")

# Inspect APK structure (control + data streams)
tar -tzf <(gunzip -c package.apk)

# Test with Alpine tools (in an Alpine container)
apk add --allow-untrusted ./package.apk
apk info -L package-name
```

#### Dependencies
- **Archive Library**: `github.com/mholt/archives` (pinned in `go.mod`, see `pkg/builders/apk/apk.go` imports). No `replace` directive is currently required.

## Release and Deployment

### Release Process
```bash
# Full release workflow
make release

# Manual release steps
make clean deps fmt lint lint-md test doc build-all

# Create release artifacts
ls releases/
# yap-linux-amd64.tar.gz
# yap-linux-arm64.tar.gz  
# yap-darwin-amd64.tar.gz
# yap-darwin-arm64.tar.gz
# yap-windows-amd64.zip
```

### Version Management
- Use semantic versioning (MAJOR.MINOR.PATCH)
- Git tags trigger release builds
- Version info embedded in binary via ldflags

### Distribution Testing
Before release, test on multiple distributions:
```bash
# Test major distribution families
yap build ubuntu examples/yap         # Debian family
yap build fedora examples/yap         # Red Hat family  
yap build alpine examples/yap         # Alpine
yap build arch examples/yap           # Arch Linux
```

## Troubleshooting

### Common Development Issues
1. **Test failures** - Check `-p 1` flag (tests must run sequentially)
2. **Container permission issues** - Ensure user is in docker group
3. **Build failures** - Clean build directory and retry
4. **Dependency issues** - Run `make deps` to refresh modules

### Debugging Tools
- `yap list-distros` - List available distributions
- `yap zap <project>` - Clean build environment for specific project
- `make test-coverage` - Identify untested code paths
- Container logs for build debugging

### Performance Considerations
- Use `--skip-sync` for faster development builds
- Container image caching improves build times
- Sequential builds are the default; use `--parallel` (`-P`) to enable dependency-aware topo-sort with worker pools
- Use `--cleanbuild` flag to clean source directory before builds

## Current Development Focus

### Active Investigations
1. **RPM Changelog** — Deferred pending rpmpack API support
2. **Test Coverage Expansion** — `pkg/signing` and `pkg/sbom` targets 85%

### Recent Achievements
- ✅ **OpenPGP verification of apt InRelease / Release.gpg (2026-05-21)** — `pkg/aptrepo/verify.go` verifies the Debian / Ubuntu repository trust chain in pure Go via `github.com/ProtonMail/go-crypto/openpgp`. Behaviour:
  - `Signed-By:` directives resolve to a specific keyring file (binary `.gpg` or ASCII-armored `.asc`).
  - Sources without `Signed-By:` use the union of the standard apt trust paths (`/etc/apt/trusted.gpg.d/*.gpg`, `/usr/share/keyrings/*.gpg`, `/etc/apt/keyrings/*.gpg`, `/etc/apt/trusted.gpg`), mirroring apt's lenient default-trust behaviour.
  - Both inline clear-signed `InRelease` and detached `Release` + `Release.gpg` pairs are handled.
  - **A signature that exists and fails to verify is fatal regardless of `--allow-unverified-repos`** — forged signatures are strictly worse than no signature.
  - `--allow-unverified-repos` (alias env var `YAP_ALLOW_UNVERIFIED_REPOS=1`) and `aptrepo.SetAllowUnverifiedRepos(true)` only relax the *missing-trust-anchor* path: when no key in the trust set matches the source, the SHA-256 manifest is still used but signature checking is skipped with a Warn log.
  - End-to-end smoke against `docker.io/m0rf30/yap-ubuntu-noble:latest` confirms `archive.ubuntu.com` and `security.ubuntu.com` InRelease files verify against `/usr/share/keyrings/ubuntu-archive-keyring.gpg` with `signer="Ubuntu Archive Automatic Signing Key (2018) <ftpmaster@ubuntu.com>"`.
- ✅ **Transitive cross-build runtime dep extraction (2026-05-21)** — `pkg/builders/common/cross.go.DownloadAndExtractCrossDeps` now walks the full transitive Depends/Pre-Depends graph of each declared dep via the new `aptcache.Cache.DownloadClosure(ctx, destDir, seeds) ([]*PackageInfo, []string, error)` helper. Previously a PKGBUILD declaring only `carbonio-ffmpeg` would fail cross-link because `libavcodec.so` has DT_NEEDED entries for `libvpx.so.12`/`libx264.so.165` from sibling packages (`carbonio-libvpx`, `carbonio-x264`) that were not declared. The closure walker:
  - Reuses `ResolveDeps`' post-order DFS with cycle detection.
  - Skips packages already marked `Installed` in `/var/lib/dpkg/status` (their dep edges are still walked so transitive-only libs aren't missed).
  - Resolves virtual packages via the reverse-Provides index; takes the first concrete provider.
  - Logs `origin=direct|transitive` per package so CI can audit what was pulled in.
  - Unresolvable deps are surfaced as Warn (not fatal): typical with file-/SO-based deps that aren't in any apt index.
  Regression covered by `pkg/aptcache/closure_test.go` (carbonio-ffmpeg shape + diamond-dep dedup) using an httptest mirror.
- ✅ **Pure-Go `apt-get install` — `pkg/aptinstall` (2026-05-20)** — Full replacement for `apt-get install`: transitive dep resolution via `aptcache.ResolveDeps`, parallel `.deb` download via `aptcache.Download`, ar/control.tar/data.tar parsing, scriptlet execution via `mvdan.cc/sh/v3` (pure-Go bash), atomic `/var/lib/dpkg/status` updates with `/var/lib/dpkg/info/<pkg>.{preinst,postinst,prerm,postrm,md5sums,conffiles,list,triggers}` materialisation, conffile collision handling (`*.dpkg-new`), single `ldconfig` invocation at end of transaction. NO subprocess fallback — failures surface as errors.
- ✅ **Pure-Go `apt-get update` — `pkg/aptrepo` (2026-05-20)** — Parses `/etc/apt/sources.list` + `/etc/apt/sources.list.d/*.{list,sources}`, fetches `InRelease`/`Release` per source, verifies SHA-256 hashes of component indexes, writes to `/var/lib/apt/lists/` with apt-compatible filename encoding so `pkg/aptcache` reads them directly. Pure-Go via `net/http` + `crypto/sha256`. Triggers `aptcache.Reload()` after success.
- ✅ **Pure-Go `apk update` + `apk add` — `pkg/apkindex` (2026-05-20)** — Replaces both Alpine subprocess calls. Parses `/etc/apk/repositories`, fetches `APKINDEX.tar.gz` per repo, parses single-char-tag stanzas (P:/V:/A:/D:/p:/…), BFS transitive dep resolution, downloads `.apk` packages, extracts the 2-stream concatenated-gzip format (control + data), updates `/lib/apk/db/installed`. NO subprocess fallback for install or update.
- ✅ **Pure-Go `pacman -Sy` — `pkg/pacmandb` (2026-05-20)** — Parses `/etc/pacman.conf` with `Include` directives + mirrorlists, resolves `$repo`/`$arch` placeholders, multi-mirror failover, atomic `.db` writes to `/var/lib/pacman/sync/`. `pacman -S` (install) still subprocess due to alpm hook complexity.
- ✅ **Pure-Go RPM SQLite database reader — `pkg/rpmdb` (2026-05-20)** — Replaces `rpm -q <pkg>` subprocess calls with direct SQLite queries (Fedora 33+, RHEL 9+, Rocky 9+, AlmaLinux 9+, openSUSE 15.5+). Uses `modernc.org/sqlite` (no CGO) and `sqlc` code generation. Falls back to subprocess for legacy BerkeleyDB hosts. O(1) indexed lookups; single DB-open, N queries much faster than N subprocesses.
- ✅ **Pure-Go `dpkg --add-architecture` + `apt-cache show` + `dpkg -s` (2026-05-20)** — All replaced by direct file I/O against `/var/lib/dpkg/arch`, `/var/lib/apt/lists/*_Packages`, `/var/lib/dpkg/status` via `pkg/aptcache`. `apt-cache policy/showpkg` for virtual packages replaced by reverse-Provides index. `pkg/aptcache.PackageInfo` now stores `BaseURL`/`Filename`/`SHA256`/`Size`/`Depends`/`PreDepends`/`Name` parsed at load time (no post-hoc URL reconstruction).
- ✅ **Package signing infrastructure: APK RSA + DEB/RPM/Pacman GPG (2026-05-04)** — Pure-Go via ProtonMail/go-crypto. APK packages now installable without --allow-untrusted; DEB/RPM/Pacman emit detached signature sidecars (.deb.asc/.rpm.asc/.pkg.tar.zst.sig). RPM also supports in-package signatures via rpmpack.SetPGPSigner. Priority: CLI > format env > global env > yap.json > ~/.config/yap/keys/.
- ✅ **SBOM generation: CycloneDX 1.5 + SPDX 2.3 (2026-05-04)** — Opt-in via --sbom; emits .cdx.json and/or .spdx.json sidecars next to each built artifact. Hand-rolled, no external SBOM library.
- ✅ **Per-format compression for DEB/RPM (2026-05-04)** — --compression-deb and --compression-rpm flags accept zstd/gzip/xz. APK and Pacman remain format-locked.
- ✅ **Pacman .INSTALL scriptlets (2026-05-04)** — Full 6-hook parity with DEB/RPM (pre_install, post_install, pre_upgrade, post_upgrade, pre_remove, post_remove). .install file conditionally emitted only when scriptlets are present.
- ✅ **Changelog support (2026-05-04)** — New PKGBUILD `changelog` field. DEB emits Lintian-compliant `usr/share/doc/<pkg>/changelog.Debian.gz`; Pacman emits `.CHANGELOG`. RPM deferred (rpmpack API gap).
- ✅ **Builder interface unification (2026-05-04)** — BuildPackage now returns (string, error) across all four builders, enabling post-build pipeline hooks for signing and SBOM.
- ✅ Verified tar format compliance (PAX matches Alpine)
- ✅ Created comprehensive APK testing infrastructure
- ✅ Integrated custom archives library for APK support
- ✅ Documented APK format requirements and gaps
- ✅ **Consolidated architecture handling and cross-compilation logging (2025-11-14)**
- ✅ **Sequential build as default; `--parallel` / `-P` flag for opt-in parallel dep resolution (2026-02-18)**
- ✅ **Code quality pass 2: fixed malformed nolint, migrated ~30 fmt.Errorf to pkg/errors, threaded context.Context through archive/shell/download APIs (2026-02-19)**
- ✅ **Unified `/etc/os-release` auto-detection across `build`, `zap`, and `prepare` (`prepare` distro arg now optional, supports `distro-release` form) — `ResolveDistroRelease` helper (2026-05-04)**
- ✅ **Pure-Go apt/dpkg metadata parser — `pkg/aptcache` (2026-05-17)** — Replaces `apt-cache show` + `dpkg -s` subprocess calls in cross-compilation dep partitioning with a single in-process deb822 index scan. O(1) lookups per package; also handles virtual package resolution (`apt-cache policy` + `showpkg` replaced by reverse-provides index).
- ✅ **pterm removed — zero external UI dependencies (2026-05-17)** — Replaced `github.com/pterm/pterm` (and 8 transitive deps: atomicgo/cursor, atomicgo/keyboard, atomicgo/schedule, containerd/console, gookit/color, lithammer/fuzzysearch, xo/terminfo, mattn/go-runewidth) with `pkg/color` (stdlib ANSI helpers) and `pkg/logger` (slog + custom handler). Progress bar replaced with 120-line stdlib implementation. Net: ~12 MB of source deps removed, 15 packages gone from module graph.
- ✅ **CLI output overhaul (2026-05-17)** — `version`/`status` use plain table output; `build --help` flags fully i18n'd; `-l`/`--language` flag now works for `--help` via pre-parse in `main()`; footer shown on root help only; emoji stripped from command Short descriptions; `pkg/buildinfo` wires ldflags correctly.
- ✅ **Structured log tree rendering (2026-05-17)** — Logger auto-wraps key-value pairs onto indented `├`/`└` tree lines when the full line exceeds terminal width, matching the previous pterm behaviour. Newlines in values collapsed to ` ↵ `.

### Architectural Decisions

#### Why Certain Methods Remain Format-Specific

The following methods **cannot** be consolidated into BaseBuilder:

**DEB-specific:**
- `getRelease()` - Debian uses codename directly (e.g., "bullseye")
- `addScriptlets()` - DEB maintainer scripts format
- `createConfFiles()` - Debian conffiles mechanism
- `createDebconfFile()` - Debian interactive configuration

**RPM-specific:**
- `getRelease()` - RPM uses distro mapping (e.g., ".fc39" for Fedora 39)
- `getGroup()` - RPM has specific group categories
- `addScriptlets()` - RPM supports pretrans/posttrans not in DEB
- `processDepends()` - Converts to rpmpack.Relations type

**APK-specific:**
- `createAPKPackage()` - Two-stream gzip format (control + data)
- `createPkgInfo()` - Alpine .PKGINFO format with datahash
- `isControlFile()` - APK control file detection logic
- `writeFileWithChecksum()` - PAX format with APK-TOOLS.checksum.SHA1

**Pacman-specific:**
- `renderMtree()` - Arch Linux mtree metadata format
- `createMTREEGzip()` - Mtree-specific compression

These differences reflect **genuine format requirements**, not duplication.

### Known Limitations
- **APK Trusted Installs**: Now supported via `pkg/signing/rsa.go` — packages can be signed with `--sign` for trusted installation
- **RPM Changelog**: Currently a no-op due to rpmpack v0.7.1 API gap. Tracked for future implementation when rpmpack adds changelog support

### Development Priorities
1. Wire RPM changelog when rpmpack adds the API
2. Expand test coverage for new packages (pkg/signing target 85%, pkg/sbom target 85%)
3. Add `yap keygen` CLI subcommand for generating signing keys
4. Add public-key extraction CLI (`yap pubkey`)
5. Repository signing (Release.gpg, repodata signing) — separate concern from package signing

## Signing & SBOM Subsystems

### Signing (`pkg/signing/`)

YAP supports cryptographic signing of built artifacts. The signing subsystem is format-aware and pluggable.

**Format → Algorithm matrix:**

| Format | Algorithm | Output | Notes |
| ------ | --------- | ------ | ----- |
| APK | RSA PKCS#1 v1.5 SHA1 | `.SIGN.RSA.<keyname>.rsa.pub` prepended as third gzip stream | Enables `apk add` without `--allow-untrusted` |
| DEB | OpenPGP (GPG) | `<package>.deb.asc` ASCII-armored detached | Verify with `gpg --verify package.deb.asc package.deb` |
| RPM | OpenPGP (GPG) | `<package>.rpm.asc` + optional in-RPM via `rpmpack.SetPGPSigner` | In-RPM preferred when supported |
| Pacman | OpenPGP (GPG) | `<package>.pkg.tar.zst.sig` binary detached | Matches `makepkg --sign` convention |

**Key resolution priority (highest to lowest):**
1. CLI flag: `--sign-key /path/to/key`
2. Format-specific env: `YAP_DEB_KEY`, `YAP_APK_KEY`, `YAP_RPM_KEY`, `YAP_PACMAN_KEY`
3. Global env: `YAP_SIGN_KEY`
4. yap.json: `signing.keyPath`
5. Default search: `~/.config/yap/keys/<format>.{rsa,gpg}` then `~/.config/yap/keys/default.{rsa,gpg}`

Passphrase resolution mirrors key resolution with `_PASSPHRASE` suffix.

**Implementation:**
- `pkg/signing/signing.go` — Format/Algorithm enums, Config, Signer interface
- `pkg/signing/resolve.go` — Priority-based config resolution
- `pkg/signing/factory.go` — `NewSigner(format, cfg) Signer`
- `pkg/signing/rsa.go` — APK RSA signer (RSASigner)
- `pkg/signing/gpg.go` — DEB/RPM/Pacman GPG signer (GPGSigner)

**Pure-Go**: Uses `github.com/ProtonMail/go-crypto/openpgp` and stdlib `crypto/rsa`. No external `gpg` binary required — container-friendly.

### SBOM (`pkg/sbom/`)

Software Bill of Materials generation, opt-in via `--sbom`.

**Formats:**
- **CycloneDX 1.5** (`<artifact>.cdx.json`) — components with hashes, licenses, external refs
- **SPDX 2.3** (`<artifact>.spdx.json`) — packages with DESCRIBES + DEPENDS_ON relationships

**Selection:** `--sbom-format cyclonedx`, `--sbom-format spdx`, or `--sbom-format both` (default).

**Data source:** PKGBUILD fields (name, version, license, depends, makedepends, sources, sha256/sha512 sums).

**Implementation:** Hand-rolled to specification, no external SBOM library — keeps dependency surface minimal.

### Build pipeline integration (`pkg/project/project.go`)

```
PrepareFakeroot → BuildPackage (returns artifactPath)
                       ↓
                  signArtifact (if proj.Signing.Enabled)
                       ↓
                  generateSBOM (if mpc.SBOM)
```

The `Builder.BuildPackage` interface returns `(string, error)` so the pipeline can address the resulting artifact for post-build hooks.

## Agent-Specific Context

### For Code Changes
- **Always check**: Existing code patterns before adding new features
- **Never assume**: Library availability - verify in `go.mod` first
- **Format validation**: Run `make fmt lint` before committing
- **Test requirements**: Sequential execution (`-p 1`) for all tests
- **APK work**: Requires `github.com/M0Rf30/archives` (morfeo branch)
- **ldflags**: Version/Commit/BuildTime are injected into `pkg/buildinfo` vars, not `main.*`
- **Logging**: Use `logger.Info/Warn/Error/Debug(msg, "key", val, ...)` — flat variadic k/v pairs
- **Colors**: Use `pkg/color` helpers — never import external color/UI libraries
- **apt metadata**: Use `pkg/aptcache.Load()` instead of spawning `apt-cache`/`dpkg` subprocesses

### For Documentation
- Keep `AGENTS.md` updated with current project status
- Document investigation findings in dedicated analysis files
- Use markdown lint rules (`.markdownlint.yml`)
- Include command examples for all workflows

### For Testing
- Use real Alpine packages as reference (`dl-cdn.alpinelinux.org`)
- Test APK changes in Alpine containers
- Compare YAP output with official packages byte-by-byte

### For Debugging
- Enable verbose logging for build issues
- Use container inspection for isolation problems
- Check binary magic bytes for format validation
- Compare with working examples before assuming bugs
