# yap-mcp — Model Context Protocol server for yap

`yap-mcp` exposes yap's package-build pipeline to any
[MCP](https://modelcontextprotocol.io)-compatible client (Claude Desktop,
opencode, Cursor, Zed, Continue, Goose, …) so a language model can validate
PKGBUILDs, run builds, fetch artifacts, sign and SBOM-tag releases — all over
a single stdio JSON-RPC stream.

It ships as a separate binary in the same release as `yap`. Communication is
in-process (no shell-out), so MCP errors carry yap's typed error payloads and
versioning is lockstep with the `yap` library.

---

## 30-second quickstart

```sh
# 1. install both yap and yap-mcp
curl -fsSL https://raw.githubusercontent.com/M0Rf30/yap/main/scripts/install.sh | sh

# 2. confirm they're on PATH
which yap-mcp                # → /usr/local/bin/yap-mcp or ~/.local/bin/yap-mcp

# 3. wire one of the clients below

# 4. smoke test from your LLM client:
#    "Validate /path/to/PKGBUILD using yap-mcp."
```

Pin a version: `sh -s -- --version v2.1.3`. Install only one tool:
`sh -s -- --tool yap-mcp`. Custom prefix: `YAP_INSTALL_DIR=$HOME/bin sh`.

---

## Wire it into your client

All clients launch `yap-mcp` over stdio. If `yap-mcp` is on `PATH`, the bare
binary name works; otherwise use the absolute path printed by `which yap-mcp`.

### Claude Desktop

`~/Library/Application Support/Claude/claude_desktop_config.json` (macOS) or
`%APPDATA%\Claude\claude_desktop_config.json` (Windows):

```json
{
  "mcpServers": {
    "yap": { "command": "yap-mcp" }
  }
}
```

### opencode

`~/.config/opencode/opencode.json` (or per-project `./opencode.json`):

```json
{
  "$schema": "https://opencode.ai/config.json",
  "mcp": {
    "yap": {
      "type": "local",
      "command": ["yap-mcp"],
      "enabled": true
    }
  }
}
```

The skill card lives at
[`skills/yap/SKILL.md`](skills/yap/SKILL.md) (Anthropic Agent Skills
layout). Clients that want auto-discovery can symlink it into their own
skill directory (e.g. `ln -s "$PWD/skills/yap" .opencode/skills/yap`).

### Cursor

`~/.cursor/mcp.json` (global) or `.cursor/mcp.json` (per project):

```json
{
  "mcpServers": {
    "yap": { "command": "yap-mcp" }
  }
}
```

### Zed

`~/.config/zed/settings.json`:

```json
{
  "context_servers": {
    "yap": {
      "command": { "path": "yap-mcp", "args": [] }
    }
  }
}
```

### Continue (VS Code / JetBrains)

`~/.continue/config.yaml`:

```yaml
mcpServers:
  - name: yap
    command: yap-mcp
```

### Goose

`~/.config/goose/config.yaml`:

```yaml
extensions:
  yap:
    type: stdio
    cmd: yap-mcp
    enabled: true
```

### Anything else

If your client speaks MCP, the recipe is always: `command = yap-mcp`,
transport = stdio, no args required. `yap-mcp` writes JSON-RPC frames to
stdout and structured logs to stderr — never the other way around — so it is
safe to run from any spawn-based supervisor.

---

## Tool surface

All tool names below are the **literal** names exposed over MCP. No prefix.
Tool annotations (`ReadOnlyHint`, `IdempotentHint`, `DestructiveHint`) are
attached so safety-aware clients can cache or sandbox automatically.

### Read-only / pure

| Tool              | Purpose                                              |
| ----------------- | ---------------------------------------------------- |
| `status`          | yap version, build metadata, container detection.    |
| `list_distros`    | Every supported distro + its package manager.        |
| `list_images`     | Pre-built container image tags available to pull.    |
| `resolve_distro`  | Auto-detect distro/release from `/etc/os-release`.   |
| `parse_pkgbuild`  | Parse PKGBUILD → structured JSON.                    |
| `validate`        | Parse + mandatory-field + general validation.        |
| `graph`           | Dependency graph (nodes + edges) for a project.      |
| `inspect`         | Format, size, SBOM presence, signature presence.     |

### Build pipeline

| Tool               | Purpose                                                                |
| ------------------ | ---------------------------------------------------------------------- |
| `prepare`          | Install toolchain + base makedeps for a distro.                        |
| `pull`             | Pull a yap container image via the detected runtime (podman/docker/rootless). |
| `build`            | **Async.** Start a build, return a `buildID`. Auto-dispatches into a container when a `distro` is supplied. |
| `build_status`     | Lightweight poll: state + log line/byte counts + container info (no log payload). |
| `build_wait`       | Block until the build reaches a terminal state, with an optional timeout. |
| `build_summary`    | One-shot diagnosis: state + duration + lastErrorLine + failedStep + keyword hints. ~1 KB. |
| `build_logs`       | Filtered log payload — `tail`, `since`, `grep`, `context`, `withLineNo`. Bounded at the last 256 KB. |
| `build_cancel`     | Cancel a running build by buildID.                                     |
| `list_artifacts`   | Enumerate produced `.deb`/`.rpm`/`.apk`/`.pkg.tar.*` files for a buildID or path. |

### Destructive

| Tool             | Purpose                                                       |
| ---------------- | ------------------------------------------------------------- |
| `install`        | Install a built artifact on the host. Requires `confirm: true`. |
| `zap`            | Deep-clean a project's build env + artifacts. Requires `confirm: true`. |

### Resources

| URI               | Description                                          |
| ----------------- | ---------------------------------------------------- |
| `yap://distros`   | Distro/release matrix paired with package managers.  |

Parameterised lookups (e.g. PKGBUILD parsing) are intentionally exposed as
**tools** instead of resource templates because URL-encoded filesystem paths
in URIs are awkward for LLMs to construct correctly. Use `parse_pkgbuild`.

### Prompts

`yap-mcp` ships named prompts you can list via `prompts/list` and inline via
`prompts/get`. These are LLM-facing workflow templates — pick one when you
want a guaranteed-correct call sequence:

| Prompt              | Walks the client through                                       |
| ------------------- | -------------------------------------------------------------- |
| `build_single_pkg`  | validate → build → wait → summary/logs on failure → list_artifacts → inspect. Set `sign=true, sbom=true` on the build call to also produce signed + SBOM-tagged release artifacts. |
| `cross_compile`     | Same flow but with `targetArch`, container distro guidance, and cross-specific log greps. |

---

## Canonical workflow

```
validate(path)
   ↓
build(path, distro?, [sign, sbom, targetArch, ...])   →  {buildID, inContainer, ...}
   ↓
build_wait(buildID, timeoutSec)
   ↓
if state != succeeded:
  build_summary(buildID)                 # one-shot diagnosis, cheap
  # only if more detail needed:
  build_logs(buildID, tail=200, grep="ERROR|FAIL",
             context=3, withLineNo=true)
else:
  list_artifacts(buildID)
  for each artifact: inspect(artifact)
```

### Token-cost guide for log/status calls

| Tool             | Returns                                              |
| ---------------- | ---------------------------------------------------- |
| `build_logs`     | Filtered log payload (up to 256 KB)                  |
| `build_summary`  | State + lastErrorLine + failedStep + hints (~1 KB)   |
| `build_status`   | State + log line/byte counts + container info (no payload) |

`build_status` is log-payload-free; call `build_logs` explicitly when the
actual text is needed.

### Container transparency

`build` and `build_status` expose where the work runs:

| Field              | Meaning                                              |
| ------------------ | ---------------------------------------------------- |
| `inContainer`      | `true` when dispatched into a yap container image.   |
| `containerRuntime` | Backend driving the build: `cli` (podman/docker) or `rootless`. |
| `containerImage`   | Resolved tag, e.g. `ubuntu-jammy`.                   |

Use these to disambiguate failures: a missing toolchain in a container build
points at the image; a missing toolchain in a native build points at the host.

---

## Async build state machine

`build` returns immediately with a `buildID`. The build runs in a goroutine
on the server. Poll lightly with `build_status`, block efficiently with
`build_wait`, diagnose with `build_summary`:

```
running ─┬─► succeeded
         ├─► failed       (Err field populated)
         └─► canceled     (build_cancel or server shutdown)
```

Build state is process-local: restarting `yap-mcp` loses in-flight builds.
The captured log is bounded at the last 256 KB per build to avoid memory
exhaustion from runaway output.

---

## Security model

- **Destructive tools** (`install`, `zap`, `build`) are flagged via
  `DestructiveHint`. `install` and `zap` additionally refuse to act unless
  `confirm: true` is set on the call.
- **Signing passphrases** are forwarded to container builds via the
  `YAP_SIGN_PASSPHRASE` env var, **not** as CLI arguments — they never
  appear in `ps aux`. Native (in-process) builds read the passphrase
  directly without touching a shell. Either way, passing `signPassphrase`
  to the `build` tool is safe.
- **No interactive prompts.** The server never asks for input over stdio
  (that would corrupt the JSON-RPC stream). Pass secrets via tool args
  or pre-set env vars.
- Key resolution order for signing: CLI `signKey` arg → format env
  (`YAP_DEB_KEY`, `YAP_APK_KEY`, …) → global env (`YAP_SIGN_KEY`) →
  `yap.json` `signing.keyPath` → `~/.config/yap/keys/<format>.{rsa,gpg}`.

---

## Gotchas

- **Arch aliases.** Pass `arm64`/`amd64`; yap normalises to
  `aarch64`/`x86_64`. `armhf` → `armv7`, `i386` → `i686`.
- **Path forms.** `path` accepts the yap.json file, the PKGBUILD file, *or*
  the directory containing either. All three resolve to the project root.
- **Distro dispatch.** Omit `distro` for a native host build. Provide a
  distro tag (e.g. `ubuntu-jammy`) to dispatch into a matching container
  image — cross-arch builds almost always want this so the right
  cross-toolchain is in scope.
- **Bounded logs.** `build_logs` returns at most the last 256 KB of
  container stdout+stderr. Use `tail`, `since`, or `grep` to keep
  payloads small.
- **SBOMs.** `sbom=true` emits both CycloneDX 1.5 and SPDX 2.3 next to each
  artifact. Restrict with `sbomFormat: "cyclonedx" | "spdx" | "both"`.

---

## Smoke test (no client required)

```sh
{
  printf '%s\n' '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"smoke","version":"0"}}}'
  printf '%s\n' '{"jsonrpc":"2.0","method":"notifications/initialized"}'
  printf '%s\n' '{"jsonrpc":"2.0","id":2,"method":"tools/list"}'
} | yap-mcp
```

You should see a JSON-RPC response listing all tools above on stdout, and
structured slog output on stderr.

---

## Troubleshooting

| Symptom                                        | Likely cause / fix                                                                 |
| ---------------------------------------------- | ---------------------------------------------------------------------------------- |
| Client says "server not found"                 | `yap-mcp` not on PATH. Use `which yap-mcp` and put the absolute path in the config. |
| Client connects but lists 0 tools              | Stale client config; remove and re-add the entry, then restart the client.        |
| `build` succeeds instantly with empty artifacts | `noBuild: true` or `from`/`to` filtering excluded all packages. Drop the flag.    |
| Container build fails with "no such image"     | Run `pull(distro)` first or wait for the auto-pull; check `docker login` if private. |
| `build` returns `inContainer: false` unexpectedly | No container runtime detected. Install podman or docker, or run on Linux for the rootless fallback. |
| `signing failed: passphrase incorrect`         | Resolution order skipped your key. Confirm with `signKey` set explicitly or `YAP_SIGN_PASSPHRASE` in env. |
| stderr swamped with `mcp request done` logs    | Logger is at INFO; set `YAP_LOG_LEVEL=warn` (env) to quiet it.                    |

The `yap-mcp` server writes all logs to **stderr**; stdout is reserved for
the MCP JSON-RPC stream. If you see protocol corruption, something else in
your launch wrapper is writing to stdout — fix that first.

---

## Architecture

```
┌──────────────┐  stdio JSON-RPC  ┌──────────────────┐
│  MCP client  │ ◀──────────────▶ │   yap-mcp        │
└──────────────┘                  │   (cmd/yap-mcp)  │
                                  │                  │
                                  │  tools, resources,
                                  │  prompts, sessions
                                  └─────────┬────────┘
                                            │ in-process
                            ┌───────────────┴───────────────┐
                            │ pkg/project, pkg/parser,      │
                            │ pkg/graph, pkg/packer,        │
                            │ pkg/builders/*, pkg/sbom,     │
                            │ pkg/signing, pkg/container,   │
                            │ pkg/logger                    │
                            └───────────────────────────────┘
```

- `pkg/mcp/` is the library (tool/resource/prompt registrations).
- `cmd/yap-mcp/main.go` is a thin stdio entrypoint.
- Imports yap packages directly — no shell-out, no version skew.
- Container dispatch uses `pkg/container` (auto-detects podman → docker →
  built-in rootless runner) and captures container stdout+stderr into a
  per-session bounded buffer surfaced via `build_logs`.

---

## Roadmap

- slog → MCP `notifications/progress` bridge for streaming build logs.
- HTTP+SSE transport for remote use behind a bearer token.
- Repository signing surface (Release.gpg, repodata).
- Additional prompts: `sign_and_release`, `pkgbuild_skeleton`,
  `port_to_distro`.
