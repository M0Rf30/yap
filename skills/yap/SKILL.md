---
name: yap
description: |
  Build native Linux packages (.deb / .rpm / .apk / .pkg.tar.zst) from a
  PKGBUILD or yap.json project using the yap-mcp Model Context Protocol
  server. Use this skill whenever the user asks to "build a package",
  "make a deb/rpm", "cross-compile for arm64/aarch64", "sign a release",
  generate SBOMs, install a built artifact, or diagnose why a yap build
  failed. The yap MCP server exposes async builds with buildID polling,
  validation, dependency-graph inspection, and artifact discovery.
---

# Driving yap-mcp

> The authoritative, generated tool/prompt/resource inventory (with
> annotations) lives in `docs/mcp-surface.md` (in the yap repo root),
> produced by `go generate ./pkg/mcp/...`. This card is the curated guide;
> that file is the source of truth.

## Picking the right tool

| Want to                                  | Use                          |
| ---------------------------------------- | ---------------------------- |
| Check a PKGBUILD before building         | `validate(path)`             |
| Parse a PKGBUILD into structured JSON    | `parse_pkgbuild(path)`       |
| See the project's dependency graph       | `graph(path)`                |
| List supported distros + pkg managers    | `list_distros()`             |
| Normalize/auto-detect distro + release   | `resolve_distro(distro?, release?)` |
| yap version + runtime/container info     | `status()`                   |
| Find prebuilt container image tags       | `list_images()`              |
| Pre-fetch one image for offline use      | `pull(distro)`               |
| Prepare the host (toolchain + makedeps)  | `prepare(distro?)`           |
| **Start a build (async, returns ID)**    | `build(path, ...)`           |
| Wait for a build to finish               | `build_wait(buildID)`        |
| Poll a build's state (lightweight)       | `build_status(buildID)`      |
| One-shot failure diagnosis               | `build_summary(buildID)`     |
| Fetch logs (tail / since / grep / ctx)   | `build_logs(buildID, ...)`   |
| Cancel a running build                   | `build_cancel(buildID)`      |
| List produced artifacts                  | `list_artifacts(buildID)`    |
| Inspect a single artifact                | `inspect(artifact)`          |
| Install an artifact (DESTRUCTIVE)        | `install(artifact, confirm: true)` |
| Clean a project's build dirs             | `zap(path, confirm: true)`   |

`list_images` and `pull` differ: `list_images` is a read-only inventory of
tags you can pass as `distro` to `build`. `pull` actually downloads a tag into
the local container store — only needed for offline use or to pre-warm a
specific image; `build` auto-pulls anyway.

## Canonical workflow

```
validate(path)
   ↓
build(path, distro?, [sign, sbom, targetArch, ...])   →  {buildID}
   ↓
build_wait(buildID, timeoutSec)
   ↓
if state != succeeded:
  build_summary(buildID)                  # one-shot diagnosis, cheap
  # only if more detail needed:
  build_logs(buildID, tail=200, grep="ERROR|FAIL",
             context=3, withLineNo=true)
else:
  list_artifacts(buildID)
  for each artifact: inspect(artifact)
```

**Token-cost guide for log/status calls (largest → cheapest):**

| Tool             | Returns                                           |
| ---------------- | ------------------------------------------------- |
| `build_logs`     | Filtered raw log payload (up to 256 KB)           |
| `build_summary`  | State + lastErrorLine + failedStep + hints (~1 KB) |
| `build_status`   | State + `logBytes`/`logLines` only (no payload)   |

`build_status` is now log-payload-free; call `build_logs` explicitly when
the actual text is needed.

The MCP server also ships two named **prompts**: `build_single_pkg`,
`cross_compile`. Use `prompts/list` then `prompts/get` to inline the right
one. `build_single_pkg` already covers sign+sbom flows when you set
`sign=true, sbom=true` on the build call.

## Container transparency

`build` and `build_status` return `inContainer`, `containerRuntime`
(`cli` for podman/docker, `rootless` for the built-in runner), and
`containerImage` (the resolved tag, e.g. `ubuntu-jammy`). Use these to
disambiguate failures: a missing toolchain in a container build points at
the image; a missing toolchain in a native build points at the host.

## Signing secrets

`signPassphrase` is forwarded to container builds via the
`YAP_SIGN_PASSPHRASE` env var, not as a CLI argument — it does not appear
in `ps aux`. For native builds the passphrase is read in-process and never
hits a shell. Either way, you can pass it via the tool call without
worrying about argv leakage.

## Gotchas

- **Arch aliases**: `arm64` / `amd64` (Go-style) are accepted but normalised
  to `aarch64` / `x86_64`. `armhf` → `armv7`, `i386` → `i686`.
- **Path forms**: `path` accepts the yap.json file, the PKGBUILD file, *or*
  the directory containing either. All three resolve to the project root.
- **Distro dispatch**: omit the `distro` arg to build natively on the host.
  Provide a distro tag (e.g. `ubuntu-jammy`) to dispatch the build into a
  matching container image. Cross-arch builds usually need a distro arg so
  the right cross-toolchain is available.
- **Formats & package managers**: 15 distros across 5 package managers —
  `apt` (Debian/Ubuntu/Mint/Pop → `.deb`), `yum` and `zypper`
  (Fedora/RHEL/Rocky/Alma/Amazon/Oracle/CentOS **and** openSUSE Leap/Tumbleweed
  → `.rpm`), `apk` (Alpine → `.apk`), `pacman` (Arch → `.pkg.tar.zst`).
  openSUSE/`zypper` is RPM-format and uses the `rpm` builder — there is no
  separate zypper builder. Call `list_distros` for the authoritative set.
- **Async builds**: `build` returns a `buildID` instantly. The actual work
  runs in a goroutine. Always pair `build` with `build_wait` or
  `build_status` polling.
- **Logs are bounded**: `build_status.log` / `build_logs.log` contain at
  most the last 256 KB of container stdout+stderr. Use `tail`, `since`,
  or `grep` on `build_logs` to keep payloads small.
- **Destructive tools**: `install`, `zap`, and `build` are flagged
  destructive via tool annotations. `install` and `zap` require an
  explicit `confirm: true` arg.
- **Signing**: set `sign: true` and yap reads the key from `signKey`
  arg, then env vars (`YAP_DEB_KEY`, `YAP_SIGN_KEY`, …), then yap.json
  `signing.keyPath`, then `~/.config/yap/keys/{format,default}.{rsa,gpg}`.
- **SBOMs**: `sbom: true` emits CycloneDX 1.5 and SPDX 2.3 next to each
  artifact (`<artifact>.cdx.json`, `<artifact>.spdx.json`). Use
  `sbomFormat: "cyclonedx" | "spdx" | "both"` (default both).

## Resources

- `yap://distros` — JSON listing of supported distros.

(For PKGBUILDs use the `parse_pkgbuild` tool — it takes a plain path arg
instead of a URL-encoded URI.)

## Installing yap on a new machine

```sh
curl -fsSL https://raw.githubusercontent.com/M0Rf30/yap/main/scripts/install.sh | sh
```

Pin a version with `sh -s -- --version v2.1.3` or install only one tool
with `sh -s -- --tool yap-mcp`. Override the install dir via
`YAP_INSTALL_DIR=$HOME/bin`.
