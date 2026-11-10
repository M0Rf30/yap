# YAP Agent Guidelines

## Project overview

**YAP (Yet Another Packager)** builds native packages for multiple GNU/Linux distributions from a single PKGBUILD specification. Written in Go; uses OCI containers (Docker/Podman) for isolated, reproducible builds across 15 supported distributions and 5 package managers (apk, apt, pacman, yum, zypper).

### Key packages

| Package | Role |
|---------|------|
| `cmd/yap/` | Cobra CLI |
| `cmd/yap-mcp/` | Stdio entrypoint for the Model Context Protocol server |
| `pkg/mcp/` | MCP tool/resource/prompt registrations; thin wrappers over yap pkgs |
| `pkg/project/` | `MultipleProject`, `BuildAll`, multi-package orchestration |
| `pkg/core/` | Project config struct (`yap.json`) |
| `pkg/builders/{apk,deb,rpm,pacman}` | Format-specific builders: map the PKGBUILD to a `pkgformat` spec and write it (openSUSE/zypper uses the `rpm` builder) |
| `pkg/builders/common` | `BaseBuilder`, shared helpers, cross-compilation |
| `pkg/packer/` | Selects the right builder per distro/package manager (`yum`+`zypper` → RPM) |
| `pkg/graph/` | Build order / dependency resolution |
| `pkg/source/` | Source download and validation |
| `pkg/fetch/` | Source download: retry budget, progress bar and i18n over `pkgformat/download` |
| `pkg/pkgbuild/`, `pkg/parser/` | Extended PKGBUILD format; `filter_installed.go` skips installed makedepends through the `pkgformat` readers (`deb/apt` cache, `apk.OpenInstalled`, `pacman.OpenLocalDB`, `rpm/rpmdb`) |
| `pkg/apthost/` | Host adapter over `pkgformat/deb/apt`: dpkg/apt paths and keyrings, lazy process-wide cache, `apt-get update` |
| `pkg/aptinstall/` | `apt-get install` (transitive dep resolution, scriptlets); reads `/var/lib/dpkg/status` with `pkgformat/deb` `OpenStatus` |
| `pkg/apkindex/` | `apk update` + `apk add`; app-side adapter over `pkgformat/apk` (`Client`, `Index`, `OpenInstalled` for the installed set) with the trust decision |
| `pkg/pacmandb/` | `pacman -Sy`; thin adapter over `pkgformat/pacman` (`ParseConfigFile`, `Client.Sync`) |
| `pkg/rpmdb/` | Host RPM database queries (ListInstalled / ListInstalledProvides / FilterInstalled) via `pkgformat/rpm/rpmdb` (SQLite, ndb, BerkeleyDB) + optional SQLite writer (Fedora 33+, RHEL 9+) |
| `pkg/dnfinstall/` | RPM install (GPG verify → CPIO extract → scriptlets → yapdb); host adapter over `pkgformat/rpm/dnf` (`host.go`) and the `rpm --install` path (`rpmcli.go`) |
| `pkg/yapdb/` | YAP-internal SQLite state DB for installed packages (cross-format) |
| `pkg/signing/` | APK RSA (`APKSigner`, over `pkgformat/apk`) + DEB/RPM/Pacman GPG signing |
| `pkg/sbom/` | CycloneDX 1.5 + SPDX 2.3 generation |
| `pkg/publish/` | `yap publish`: upload deb/rpm (native APIs) and apk/pacman (raw repo) to OpenArtifact; Bearer token, retries, sha256 verification, idempotent on 409 |
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
make all              # clean, deps, fmt-check, lint, test, doc, build (non-mutating)
make build            # build yap binary with version info
make build-all        # linux/darwin, amd64/arm64 (windows unsupported: dnfinstall uses flock/chroot)
make fmt-check        # fail if gofmt/goimports would change files
make tidy             # go mod tidy (no longer run implicitly by all/release)
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
go test -timeout 30s ./pkg/fetch/...
```

### YAP CLI quick reference

```bash
yap build [distro[-release]] <path>    # distro auto-detected from /etc/os-release if omitted
yap zap [distro] <path>
yap prepare [distro[-release]]
yap pull <distro>
yap install <artifact-file>
yap publish [artifact-or-dir]...       # upload to OpenArtifact (YAP_OA_URL/YAP_OA_TOKEN/...)
yap list-distros
yap status
yap graph <path>

# Key build flags
--skip-sync --cleanbuild --parallel --sign --sbom --nocheck
--compression-deb zstd|gzip|xz
--compression-rpm zstd|gzip|xz
--rpm-format 4|6
--target-arch <arch>
--source-retries <n>
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
| APK | RSA PKCS#1 v1.5 SHA1 (`APKSigner`, `pkgapk.SigRSASHA1`) | `.SIGN.RSA.<keyname>.rsa.pub` prepended as first gzip stream by `pkgapk.SignPackage`; RSA keys of 2048–16384 bits only |
| DEB | OpenPGP | `<package>.deb.asc` ASCII-armored detached |
| RPM | OpenPGP | embedded via `pkgformat/rpm/pack` `RPM.SetPGPSigner` (v4 RSA+PGP tags, v6 `OPENPGP` tag) + `<package>.rpm.asc` |
| Pacman | OpenPGP | `<package>.pkg.tar.zst.sig` binary detached |

Key resolution priority (highest first):
1. `--sign-key` CLI flag
2. Format env: `YAP_DEB_KEY`, `YAP_APK_KEY`, `YAP_RPM_KEY`, `YAP_PACMAN_KEY`
3. Global env: `YAP_SIGN_KEY`
4. `yap.json` `signing.keyPath`
5. `~/.config/yap/keys/<format>.{rsa,gpg}` → `~/.config/yap/keys/default.{rsa,gpg}`

Format-specific resolution is applied in `signing.NewSigner` via `signing.ForFormat`:
implicit keys (global env / default files) are replaced by `YAP_<FMT>_KEY` or the
format default; if no key of the right type exists the artifact is left unsigned.
Explicit `--sign-key` / `yap.json` keys are never overridden.

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
- apt/dpkg metadata + `apt-get update` — `pkg/apthost` over `pkgformat/deb/apt` (deb822 parsing, InRelease/Release.gpg OpenPGP verification, http→https fallback, closure downloads); replaces `apt-cache`/`dpkg`/`apt-get update` subprocesses
- `apt-get install` — `pkg/aptinstall` (transitive deps, scriptlets, yapdb state by default)
- `apk update` + `apk add` — `pkg/apkindex` over `pkgformat/apk`; indexes are verified against `/etc/apk/keys`; with no matching key a privileged host installs unverified with a warning, an unprivileged one refuses. `epoch:` is ignored with a warning (Alpine has none)
- `pacman -Sy` — `pkg/pacmandb` over `pkgformat/pacman`
- Installed-package databases — dpkg `status` (`pkg/aptinstall`, `pkgformat/deb` `OpenStatus`; the apt cache of `pkg/apthost` reads the same file through it), apk `/lib/apk/db/installed` (`pkgformat/apk` `OpenInstalled`) and pacman `/var/lib/pacman/local` (`pkgformat/pacman` `OpenLocalDB`; a package is named by its `desc` `%NAME%`), all read-only; an unreadable database means "nothing installed" to the callers that filter makedepends. `pkg/aptinstall` keeps the writer (`writeDpkgStatus`) and encodes the library's logical field values back
- RPM database reader — `pkg/rpmdb`: wraps `github.com/M0Rf30/pkgformat/rpm/rpmdb` (`Find("/")` + `Open`), one code path for SQLite (Fedora 33+, RHEL 9+, Arch), ndb (openSUSE/SLE `Packages.db`) and BerkeleyDB (RHEL 8 and earlier); `ErrNoDatabase` when the host has none, and only then the batched `rpm -q` / `rpm -qa` subprocess fallback
- RPM install — `pkg/dnfinstall` (GPG verify → CPIO extract → scriptlets → yapdb); replaces `dnf install` for both `yap install <pkg.rpm>` and build-time makedepends on RPM distros
- YAP-internal package state — `pkg/yapdb` (cross-format SQLite registry at `<rootDir>/var/lib/yap/installed.db`); decouples YAP from system dpkg/rpm DBs (ephemeral build container friendly)
- DNF metadata + install resolution — `pkgformat/rpm/dnf` (repomd/primary parsing, module filtering, transitive dep resolution, checksum-verified resumable downloads, mirror failover with metadata and primary from the same mirror); yap wires it in `pkg/dnfinstall/host.go`. Packages without a SHA-256/512 checksum are refused
- Transient-failure retry — `pkgformat/httpclient` (retries transport errors, mid-body EOF, HTTP 408/429/5xx; never other 4xx/size-cap/ctx-cancel); used by every apt/dnf/apk/pacman client and `pkg/publish` (`publish.Config.Retry`)
- Source download retry (gensum / pkg/source / pkg/platform) — `pkg/fetch` over `pkgformat/download` (resume-capable, no grab); budget configurable via `--source-retries` / `YAP_SOURCE_MAX_RETRIES` (default 3 retries = 4 attempts), shared with `pkg/platform`'s toolchain fetch
- Extraction safety — `pkgformat/safepath` plus `pkg/archive`'s stricter `safeJoin`/`safeSymlinkTarget` (hostile `..`-bearing member names rejected even under root "/"); `.deb` and `.rpm` payloads are extracted by the pkgformat readers, confined with `os.Root`
- Transitive cross-build dep extraction — `pkg/builders/common/cross.go` `DownloadClosure`
- Package signing — APK RSA (`pkgapk.SignPackage`) + DEB/RPM/Pacman GPG
- SBOM — CycloneDX 1.5 + SPDX 2.3
- Per-format compression — DEB/RPM
- RPM v6 package format — opt-in via `--rpm-format 6` / yap.json `rpmFormat` / MCP `rpmFormat` (default 4). Writer: `pkgformat/rpm/pack` (`RPMMetaData.Format`; SHA256+SHA3-256 header digests, 64-bit sizes, stripped `07070X` cpio, `OPENPGP` signature tag). Reader: `pkgformat/rpm` (used by `pkg/dnfinstall` and `pkg/archive`). Validated against rpm 6.0.1 (`rpm -K`, `rpm -i`)
- Embedded RPM signatures — with `--sign`, RPM artifacts carry the signature in the package header (`prepareEmbeddedSigning` → `pack.RPM.SetPGPSigner`), required by rpm 6's default `%_pkgverify_level signature`
- Pacman `.INSTALL` scriptlets — 6-hook parity
- Changelog support — DEB (Lintian-compliant), RPM (native %changelog), Pacman
- Builder interface unification — `BuildPackage` returns `(string, error)`
- PKGBUILD `check()` function — runs between build and package (makepkg parity); skip with `--nocheck` (CLI), `noCheck` (MCP build tool)
- Implicit `cd "${srcdir}"` before every PKGBUILD function (prepare/build/check/package) — makepkg `run_function` parity
- Sequential build default; `--parallel` / `-P` opt-in
- `pkg/color` + `pkg/logger` — zero external UI deps (pterm removed)
- Structured log tree rendering
- `/etc/os-release` auto-detection — `ResolveDistroRelease` helper: an
  explicit bare distro family stays generic (`1ubuntu`, never host `1jammy`);
  `ResolveContainerImage` picks the build image separately, package identity untouched

### Known limitations
- `pacman -S` (install): still subprocess due to alpm hook complexity
- `pkg/dnfinstall` does NOT update `/var/lib/rpm/` by default (state lives in yapdb); set `Options.WriteSystemRpmdb=true` to also write to SQLite rpmdb (Fedora 33+/RHEL 9+/Rocky 9+ only — BDB hosts skip with warn)
- `pkg/aptinstall` does NOT update `/var/lib/dpkg/status` by default (state lives in yapdb); set `Options.WriteDpkgStatus=true` for legacy behavior
- `pkg/aptinstall` rejects `Options.RootDir` other than `/` (dpkg state + maintainer scripts are not yet re-rooted)
- Extraction into a non-`/` root (archive, aptinstall, dnfinstall, apkindex, rootless pull) resolves every path through `safepath.ResolveInRoot` (chroot-semantics symlink resolution); extraction into `/` keeps plain OS semantics
- `pkg/dnfinstall` hard-fails cryptographically invalid RPM signatures even with `AllowUnverifiedRPMs`; only missing keyring / unknown signer / unsigned are tolerated in that mode
- `pkg/dnfinstall` coverage at ~40% (pack→rpm interop blocks happy-path integration tests); pure-unit coverage of helpers is strong, full pipeline validated via `make test-e2e-rpm` on Rocky 8

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
1. Expand test coverage: `pkg/signing` and `pkg/sbom` targets 85%
2. `yap keygen` CLI subcommand for signing key generation
3. `yap pubkey` for public-key extraction
4. Repository signing (Release.gpg, repodata) — separate from package signing

## Agent-specific rules

### For code changes
- Check existing patterns before adding features
- Verify library availability in `go.mod` before importing
- Run `make fmt lint` before committing
- Tests require `-p 1` (sequential)
- Package formats (deb, rpm, apk, pacman writers and readers, apt/dnf/apk/pacman repository clients, safepath, httpclient, download) live in `github.com/M0Rf30/pkgformat`; do not add a second implementation to yap
- ldflags inject into `pkg/buildinfo` vars, not `main.*`
- Use `logger.Info/Warn/Error/Debug(msg, "key", val, ...)` — flat variadic k/v
- Use `pkg/color` helpers — never import external color/UI libraries
- Use `pkg/apthost.Cache()` instead of spawning `apt-cache`/`dpkg` subprocesses

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
