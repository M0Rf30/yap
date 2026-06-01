# YAP Agent Guidelines

## Project overview

**YAP (Yet Another Packager)** builds native packages for multiple GNU/Linux distributions from a single PKGBUILD specification. Written in Go; uses OCI containers (Docker/Podman) for isolated, reproducible builds across 15 supported distributions.

### Key packages

| Package | Role |
|---------|------|
| `cmd/yap/` | Cobra CLI |
| `cmd/yap-mcp/` | Stdio entrypoint for the Model Context Protocol server |
| `pkg/mcp/` | MCP tool/resource/prompt registrations; thin wrappers over yap pkgs |
| `pkg/project/` | `MultipleProject`, `BuildAll`, multi-package orchestration |
| `pkg/core/` | Project config struct (`yap.json`) |
| `pkg/builders/{apk,deb,rpm,pacman}` | Format-specific builders |
| `pkg/builders/common` | `BaseBuilder`, shared helpers, cross-compilation |
| `pkg/packer/` | Selects the right builder per distro/package manager |
| `pkg/graph/` | Build order / dependency resolution |
| `pkg/source/` | Source download and validation |
| `pkg/pkgbuild/`, `pkg/parser/` | Extended PKGBUILD format |
| `pkg/aptcache/` | deb822 parser for apt/dpkg metadata |
| `pkg/aptrepo/` | `apt-get update` + OpenPGP InRelease verification |
| `pkg/aptinstall/` | `apt-get install` (transitive dep resolution, scriptlets) |
| `pkg/apkindex/` | `apk update` + `apk add` |
| `pkg/pacmandb/` | `pacman -Sy` |
| `pkg/rpmdb/` | RPM SQLite reader + optional writer (Fedora 33+, RHEL 9+) |
| `pkg/dnfinstall/` | RPM install (GPG verify → CPIO extract → scriptlets → yapdb) |
| `pkg/yapdb/` | YAP-internal SQLite state DB for installed packages (cross-format) |
| `pkg/signing/` | APK RSA + DEB/RPM/Pacman GPG signing |
| `pkg/sbom/` | CycloneDX 1.5 + SPDX 2.3 generation |
| `pkg/color/` | Zero-dependency ANSI color helpers |
| `pkg/logger/` | slog-based structured logger with tree rendering |
| `pkg/buildinfo/` | Build-time metadata (Version, Commit, BuildTime) via ldflags |

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

## Build/test commands

```bash
make all              # clean, deps, fmt, lint, test, doc, build
make build            # build yap binary with version info
make build-all        # linux/darwin/windows, amd64/arm64
make clean
make deps
make fmt
make lint
make lint-md
make test             # -p 1 -v (sequential required)
make test-coverage    # generates coverage.html
make release

make doc-serve        # pkgsite on localhost:8080
make doc-generate     # static docs in docs/api/
make doc-package PKG=./pkg/builders/apk

make docker-build DISTRO=ubuntu
make docker-build-all
```

### Package-specific tests

```bash
go test ./pkg/source -v
go test ./pkg/builders/deb -v
go test ./pkg/graph -v
go test ./cmd/yap/command -v
go test -race ./pkg/builders/...
go test -timeout 30s ./pkg/download/...
```

### YAP CLI quick reference

```bash
yap build [distro[-release]] <path>    # distro auto-detected from /etc/os-release if omitted
yap zap [distro] <path>
yap prepare [distro[-release]]
yap pull <distro>
yap install <artifact-file>
yap list-distros
yap status
yap graph <path>

# Key build flags
--skip-sync --cleanbuild --parallel --sign --sbom
--compression-deb zstd|gzip|xz
--compression-rpm zstd|gzip|xz
--target-arch <arch>
```

## Code style

- **Module**: `github.com/M0Rf30/yap/v2`
- **Import grouping**: stdlib → third-party → local (goimports, local prefix `github.com/M0Rf30/yap/v2`)
- **Line length**: 100 chars max
- **Error handling**: `pkg/errors` typed errors with context
- **Naming**: exported PascalCase, private camelCase
- **Complexity**: max cyclomatic 15

### Error handling

```go
if err := operation(); err != nil {
    return errors.Wrap(err, errors.ErrTypeBuild, "failed to perform operation").
        WithOperation("BuildPackage").
        WithContext("config_path", configPath)
}

return errors.New(errors.ErrTypeConfiguration, "invalid configuration").
    WithContext("config_path", configPath).
    WithOperation("LoadConfig")
```

### Logging

```go
logger.Info("Building package", "package", pkgName, "distro", distro)
logger.Warn("Skipping strip", "binary", path, "reason", "foreign arch")
logger.Error("Build failed", "error", err, "duration", elapsed)
```

Flat variadic k/v pairs after the message. Long lines auto-wrap into `├`/`└` tree layout.

## Package builder interface

```go
type Packer interface {
    PrepareFakeroot(artifactsPath, targetArch string) error
    BuildPackage(artifactsPath, targetArch string) error  // returns (string, error)
}
```

`BuildPackage` returns the artifact path so the post-build pipeline can sign and generate SBOMs.

### Build pipeline

```
PrepareFakeroot → BuildPackage (returns artifactPath)
                       ↓
                  signArtifact (if proj.Signing.Enabled)
                       ↓
                  generateSBOM (if mpc.SBOM)
```

### Why certain methods stay format-specific

**DEB**: `getRelease()` (codename), `addScriptlets()`, `createConfFiles()`, `createDebconfFile()`  
**RPM**: `getRelease()` (distro mapping e.g. `.fc39`), `getGroup()`, `addScriptlets()`, `processDepends()`  
**APK**: `createAPKPackage()` (two-stream gzip), `createPkgInfo()`, `isControlFile()`, `writeFileWithChecksum()` (PAX + SHA1)  
**Pacman**: `renderMtree()`, `createMTREEGzip()`

These reflect genuine format requirements, not duplication.

## Signing subsystem (`pkg/signing/`)

| Format | Algorithm | Output |
|--------|-----------|--------|
| APK | RSA PKCS#1 v1.5 SHA1 | `.SIGN.RSA.<keyname>.rsa.pub` prepended as third gzip stream |
| DEB | OpenPGP | `<package>.deb.asc` ASCII-armored detached |
| RPM | OpenPGP | `<package>.rpm.asc` + optional in-RPM via rpmpack |
| Pacman | OpenPGP | `<package>.pkg.tar.zst.sig` binary detached |

Key resolution priority (highest first):
1. `--sign-key` CLI flag
2. Format env: `YAP_DEB_KEY`, `YAP_APK_KEY`, `YAP_RPM_KEY`, `YAP_PACMAN_KEY`
3. Global env: `YAP_SIGN_KEY`
4. `yap.json` `signing.keyPath`
5. `~/.config/yap/keys/<format>.{rsa,gpg}` → `~/.config/yap/keys/default.{rsa,gpg}`

Passphrase mirrors key resolution with `_PASSPHRASE` suffix.

Files: `signing.go` (enums, Config, Signer interface), `resolve.go`, `factory.go`, `rsa.go` (APK), `gpg.go` (DEB/RPM/Pacman).

## SBOM subsystem (`pkg/sbom/`)

- **CycloneDX 1.5** (`<artifact>.cdx.json`)
- **SPDX 2.3** (`<artifact>.spdx.json`)
- Selection: `--sbom-format cyclonedx|spdx|both` (default: both)
- Data source: PKGBUILD fields (name, version, license, deps, sources, checksums)
- Hand-rolled, no external SBOM library

## APK builder compliance (`pkg/builders/apk/`)

### Completed
- PAX tar format (POSIX.1-2001) matching Alpine's `abuild-tar`
- Two-stream concatenated gzip: `control.tar.gz` + `data.tar.gz`
- PAX extended headers: `APK-TOOLS.checksum.SHA1` per file
- `.PKGINFO` metadata: name, version, arch, desc, license, url, deps, conflicts, `size`, `origin`, `builddate`, `datahash`
- RSA signature stream (`.SIGN.RSA.<keyname>.rsa.pub`): PKCS#1 v1.5 SHA1 over control.tar.gz

### Planned
- `commit` field in `.PKGINFO` (low priority)
- Per-file ACL/xattr headers (rare in practice)
- Test coverage expansion (current ~70%, target 85%)

### APK testing

```bash
# Verify PAX magic bytes
gunzip -c package.apk | dd bs=1 skip=257 count=8 2>/dev/null | od -A n -t x1z
# Expected: 75 73 74 61 72 00 30 30 ("ustar\000" + "00")

# Test in Alpine container
apk add --allow-untrusted ./package.apk
apk info -L package-name
```

Dependency: `github.com/mholt/archives` (pinned in `go.mod`).

### RPM E2E testing

```bash
# Run RPM install E2E test on Rocky Linux 8
make test-e2e-rpm
```

The E2E test validates the complete RPM install pipeline:
- **Pipeline**: GPG verify → CPIO extract → scriptlets → yapdb state write
- **State storage**: `/var/lib/yap/installed.db` (SQLite), NOT `/var/lib/rpm/` (system rpmdb)
- **Test package**: `tree` (small, real Rocky 8 package from official repos)
- **Assertions**:
  1. Binary installed and functional (`/usr/bin/tree`)
  2. YAP state DB created with package metadata and file list
  3. System rpmdb untouched (no subprocess)
- **Requirements**: Docker or Podman; auto-detects container runtime
- **Duration**: ~2–3 minutes (includes `dnf install golang make sqlite` inside container)

Test script: `scripts/e2e-rpm.sh` (runs inside Rocky 8 container)

## Current state

### Implemented (stable)
- apt/dpkg metadata — `pkg/aptcache` (replaces `apt-cache`/`dpkg` subprocesses)
- `apt-get update` — `pkg/aptrepo` (InRelease/Release.gpg + OpenPGP verification)
- `apt-get install` — `pkg/aptinstall` (transitive deps, scriptlets, yapdb state by default)
- `apk update` + `apk add` — `pkg/apkindex`
- `pacman -Sy` — `pkg/pacmandb`
- RPM SQLite reader — `pkg/rpmdb` (Fedora 33+, RHEL 9+; subprocess fallback for legacy BDB)
- RPM install — `pkg/dnfinstall` (GPG verify → CPIO extract → scriptlets → yapdb); replaces `dnf install` for both `yap install <pkg.rpm>` and build-time makedepends on RPM distros
- YAP-internal package state — `pkg/yapdb` (cross-format SQLite registry at `<rootDir>/var/lib/yap/installed.db`); decouples YAP from system dpkg/rpm DBs (ephemeral build container friendly)
- Transitive cross-build dep extraction — `pkg/builders/common/cross.go` `DownloadClosure`
- Package signing — APK RSA + DEB/RPM/Pacman GPG
- SBOM — CycloneDX 1.5 + SPDX 2.3
- Per-format compression — DEB/RPM
- Pacman `.INSTALL` scriptlets — 6-hook parity
- Changelog support — DEB (Lintian-compliant), Pacman; RPM deferred
- Builder interface unification — `BuildPackage` returns `(string, error)`
- Sequential build default; `--parallel` / `-P` opt-in
- `pkg/color` + `pkg/logger` — zero external UI deps (pterm removed)
- Structured log tree rendering
- `/etc/os-release` auto-detection — `ResolveDistroRelease` helper

### Known limitations
- RPM changelog: no-op pending rpmpack API support
- `pacman -S` (install): still subprocess due to alpm hook complexity
- `pkg/dnfinstall` does NOT update `/var/lib/rpm/` by default (state lives in yapdb); set `Options.WriteSystemRpmdb=true` to also write to SQLite rpmdb (Fedora 33+/RHEL 9+/Rocky 9+ only — BDB hosts skip with warn)
- `pkg/aptinstall` does NOT update `/var/lib/dpkg/status` by default (state lives in yapdb); set `Options.WriteDpkgStatus=true` for legacy behavior
- `pkg/dnfinstall` coverage at ~41% (rpmpack→rpmutils interop blocks happy-path integration tests); pure-unit coverage of helpers is strong, full pipeline validated via `make test-e2e-rpm` on Rocky 8

### MCP surface (`pkg/mcp/`, `cmd/yap-mcp/`)

- Tools (19): `validate`, `parse_pkgbuild`, `graph`, `build` (async + buildID), `build_status`, `build_wait`, `build_summary`, `build_logs` (tail/since/grep), `build_cancel`, `list_artifacts`, `inspect`, `install`, `prepare`, `pull`, `zap`, `list_distros`, `list_images`, `resolve_distro`, `status`.
- Prompts (2): `build_single_pkg`, `cross_compile`.
- Resources (1): `yap://distros`.
- The exact tool/prompt/resource sets are enforced by `pkg/mcp/surface_test.go` (Test{Tool,Prompt,Resource}SurfaceExact). Adding or removing one fails CI until this list, `skills/yap/SKILL.md`, and the test's expected set are all updated together.
- `docs/mcp-surface.md` is the generated source-of-truth inventory (names + annotations + descriptions); regenerate with `go generate ./pkg/mcp/...` (runs `cmd/mcp-surface`) after any surface change.
- Handlers are intentionally thin wrappers over the existing `pkg/project`, `pkg/parser`, `pkg/packer`, `pkg/container`, `pkg/signing`, `pkg/dnfinstall` packages — do not duplicate build logic in `pkg/mcp`.
- `BuildSession.Done()` channel is closed by `Finish`; use it instead of polling for new wait-style tools.
- Tool annotations: `ReadOnlyHint` + `IdempotentHint` for inspectors; `DestructiveHint` for `build`, `install`, `prepare`, `zap`.
- Skill card lives at `skills/yap/SKILL.md`; keep it in sync when adding tools/prompts (enforced by `pkg/mcp/surface_test.go`).
- Install script: `scripts/install.sh` (curl|sh; Linux/Darwin × amd64/arm64).

### Development priorities
1. Wire RPM changelog when rpmpack adds the API
2. Expand test coverage: `pkg/signing` and `pkg/sbom` targets 85%
3. `yap keygen` CLI subcommand for signing key generation
4. `yap pubkey` for public-key extraction
5. Repository signing (Release.gpg, repodata) — separate from package signing

## Agent-specific rules

### For code changes
- Check existing patterns before adding features
- Verify library availability in `go.mod` before importing
- Run `make fmt lint` before committing
- Tests require `-p 1` (sequential)
- APK work requires `github.com/M0Rf30/archives` (morfeo branch)
- ldflags inject into `pkg/buildinfo` vars, not `main.*`
- Use `logger.Info/Warn/Error/Debug(msg, "key", val, ...)` — flat variadic k/v
- Use `pkg/color` helpers — never import external color/UI libraries
- Use `pkg/aptcache.Load()` instead of spawning `apt-cache`/`dpkg` subprocesses

### For documentation
- Keep this file updated with current project state
- Use `.markdownlint.yml` rules
- Include command examples for all workflows

### For testing
- Use real Alpine packages as reference (`dl-cdn.alpinelinux.org`)
- Test APK changes in Alpine containers
- Compare YAP output with official packages byte-by-byte
- Minimum 70% coverage for new code; 85%+ for builders and parsers

### For debugging
- Enable verbose logging for build issues
- Use container inspection for isolation problems
- Check binary magic bytes for format validation
- Compare with working examples before assuming bugs
