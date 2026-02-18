# YAP Agent Guidelines

## Project Overview

**YAP (Yet Another Packager)** is a modern, cross-distribution package building tool that creates native packages for multiple GNU/Linux distributions from a single PKGBUILD specification. The project is written in Go and uses OCI containers (Docker/Podman) for isolated, reproducible builds across 16+ supported distributions.

### Key Components
- **CLI Interface** (`cmd/yap/`) - Cobra-based command-line interface
- **Package Building** (`pkg/builders/`) - Distribution-specific package builders (APK, DEB, RPM, Pacman)
- **Container Management** - OCI container orchestration for isolated builds
- **Dependency Resolution** (`pkg/graph/`) - Build order calculation and dependency management
- **Source Handling** (`pkg/source/`) - Download and validation of source files
- **PKGBUILD Parsing** (`pkg/pkgbuild/`, `pkg/parser/`) - Extended PKGBUILD format support
- **Custom Libraries** - Fork of archives library with APK-specific enhancements

### Architecture
```
┌─────────────────┐    ┌──────────────────┐    ┌─────────────────┐
│   CLI Layer     │───▶│  Core Engine     │───▶│  Builders       │
│   (cmd/yap)     │    │  (pkg/core)      │    │  (pkg/builders) │
└─────────────────┘    └──────────────────┘    └─────────────────┘
                              │
                              ▼
┌─────────────────┐    ┌──────────────────┐    ┌─────────────────┐
│   Containers    │◀───│  Graph/Deps      │───▶│  Source Mgmt    │
│   (Docker/Pod)  │    │  (pkg/graph)     │    │  (pkg/source)   │
└─────────────────┘    └──────────────────┘    └─────────────────┘
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
yap build <project-path>                       # Build for all distributions

# Project management
yap zap <project-path>                         # Clean build environment
yap list-distros                               # List supported distributions
yap graph <project-path>                       # Show dependency graph

# Package operations  
yap build --skip-sync <project-path>           # Skip dependency sync (faster)
yap build --cleanbuild <project-path>          # Clean source before build
yap build --parallel <project-path>            # Enable parallel dep-aware topo-sort (opt-in)
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
- `make doc-package PKG=./pkg/specific` - View specific package documentation
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
- Include component and operation context
- Use appropriate log levels (DEBUG, INFO, WARN, ERROR)

```go
logger.Info("Building package").
    WithField("package", pkgName).
    WithField("distribution", distro).
    WithComponent("builder")
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
```go
type Builder interface {
    Build(ctx context.Context, pkg *pkgbuild.Package) (*BuildResult, error)
    Prepare(ctx context.Context, distro string) error
    Clean(ctx context.Context) error
}
```

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
cd test-apk/
go run tar_format.go                           # Generate test packages
../scripts/test_apk_format.sh yap-auto.apk     # Quick format check
../scripts/compare_apk.sh official.apk yap.apk # Detailed comparison

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
- Tests run on Go 1.24.6 (as specified in go.mod)
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

The APK builder (`pkg/builders/apk/`) is under active development for full Alpine Linux compliance. Current status:

#### ✅ Completed Features
- **Tar Format**: Uses PAX format (POSIX.1-2001) matching Alpine's `abuild-tar`
- **Basic Package Structure**: `.PKGINFO`, `data.tar.gz` generation
- **Dependency Handling**: Package dependencies and conflicts
- **Metadata**: Basic package information (name, version, arch, description)

#### 🚧 In Progress / Planned Features
1. **3-Stream Gzip Format** (Critical)
   - Alpine APK uses concatenated gzip streams: `.SIGN` + `.CONTROL` + `data.tar.gz`
   - Current YAP: Single-stream gzip
   - Required for `apk add` compatibility
   
2. **Package Signing** (High Priority)
   - RSA signature support (`.SIGN.RSA.*` entries)
   - Integration with Alpine signing keys
   - Optional for development, required for production

3. **PAX Extended Headers** (Medium Priority)
   - `APK-TOOLS.checksum.SHA1` headers for file integrity
   - Used by `apk audit` and verification tools
   - Currently missing in YAP output

4. **Extended Metadata** (Low Priority)
   - `origin`: Source package name
   - `commit`: Git commit hash
   - `builddate`: Build timestamp  
   - `datahash`: Additional integrity check

#### Investigation Tools & Scripts
- `test-apk/tar_format.go` - Test different tar format outputs
- `scripts/test_apk_format.sh` - Quick tar format verification
- `scripts/compare_apk.sh` - Detailed APK comparison tool
- `test-apk/manual/.PKGINFO` - Manual APK structure reference

#### Key Findings Documentation
- `APK_TAR_FORMAT_FINAL_ANALYSIS.md` - Authoritative tar format analysis
- `APK_COMPLIANCE_SUMMARY.md` - Overall compliance status
- `APK_TAR_FORMAT_TEST_RESULTS.md` - Test results and comparisons

#### Testing APK Packages
```bash
# Download official Alpine package for comparison
wget http://dl-cdn.alpinelinux.org/alpine/v3.22/main/x86_64/busybox-1.37.0-r19.apk

# Build YAP APK package
yap build alpine examples/yap

# Compare tar formats
gunzip -c package.apk | dd bs=1 skip=257 count=8 2>/dev/null | od -A n -t x1z
# Expected: 75 73 74 61 72 00 30 30 (PAX format)

# Inspect APK structure
tar -tzf <(gunzip -c package.apk)

# Test with Alpine tools (in Alpine container)
apk add --allow-untrusted ./package.apk
apk info -L package-name
```

#### Dependencies
- **Custom Archive Library**: `github.com/M0Rf30/archives` (morfeo branch)
  - Fork of go-libpack/archives with APK-specific enhancements
  - PAX format support for extended attributes
  - Replace in `go.mod` before APK development work

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
1. **APK Format Compliance** - Bringing APK builder to full Alpine Linux compatibility
2. **Multi-Stream Gzip Support** - Critical for APK package installation
3. **Package Signing Infrastructure** - RSA signature integration

### Recent Achievements
- ✅ Verified tar format compliance (PAX matches Alpine)
- ✅ Created comprehensive APK testing infrastructure
- ✅ Integrated custom archives library for APK support
- ✅ Documented APK format requirements and gaps
- ✅ **Consolidated architecture handling and cross-compilation logging (2025-11-14)**
- ✅ **Sequential build as default; `--parallel` / `-P` flag for opt-in parallel dep resolution (2026-02-18)**

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
- **APK Packages**: Not yet installable with `apk add` (3-stream format required)
- **APK Signing**: No signature support (development builds only)
- **PAX Checksums**: Missing APK-TOOLS.checksum.SHA1 headers

### Development Priorities
1. Implement 3-stream gzip format for APK
2. Add RSA signing support
3. Integrate PAX extended attributes for checksums
4. Expand test coverage for APK builder (current: ~70%, target: 85%)

## Agent-Specific Context

### For Code Changes
- **Always check**: Existing code patterns before adding new features
- **Never assume**: Library availability - verify in `go.mod` first
- **Format validation**: Run `make fmt lint` before committing
- **Test requirements**: Sequential execution (`-p 1`) for all tests
- **APK work**: Requires `github.com/M0Rf30/archives` (morfeo branch)

### For Documentation
- Keep `AGENTS.md` updated with current project status
- Document investigation findings in dedicated analysis files
- Use markdown lint rules (`.markdownlint.yml`)
- Include command examples for all workflows

### For Testing
- Use real Alpine packages as reference (`dl-cdn.alpinelinux.org`)
- Test APK changes in Alpine containers
- Compare YAP output with official packages byte-by-byte
- Document test procedures in `test-apk/` directory

### For Debugging
- Enable verbose logging for build issues
- Use container inspection for isolation problems
- Check binary magic bytes for format validation
- Compare with working examples before assuming bugs
