# Publishing YAP's MCP server & skill

This document covers how `yap-mcp` and the YAP agent skill are distributed to
the popular discovery channels.

## Artifacts

| Artifact | Source | Channel |
| --- | --- | --- |
| `yap-mcp` binary | goreleaser (`.goreleaser.yml`, id `yap-mcp`) | GitHub Releases |
| `ghcr.io/m0rf30/yap-mcp` OCI image | `build/mcp/Dockerfile` + `.github/workflows/mcp-publish.yml` | GHCR + MCP registry |
| `server.json` | repo root | MCP registry manifest |
| `skills/yap/SKILL.md` | repo | Anthropic Agent Skills layout |

## 1. MCP Registry (registry.modelcontextprotocol.io)

The official registry is the highest-signal channel. It cannot ingest a raw
`.tar.gz` release asset — `registryType` only accepts `npm | pypi | oci |
nuget | mcpb`. We publish the **OCI image** and reference it from
`server.json`.

Automated on every `v*.*.*` tag by `.github/workflows/mcp-publish.yml`:

1. Builds + pushes `ghcr.io/m0rf30/yap-mcp:{version,latest}` (amd64 + arm64).
2. Syncs the version into `server.json`.
3. Authenticates via **GitHub OIDC** (`mcp-publisher login github-oidc`).
4. `mcp-publisher publish`.

Manual publish:

```sh
# from repo root, server.json present
mcp-publisher login github-oidc
mcp-publisher publish
```

Namespace `io.github.M0Rf30/*` is owned automatically via the GitHub OIDC
identity (the repo is under the `M0Rf30` GitHub account).

### Naming

- Server name: `io.github.M0Rf30/yap` (reverse-DNS, exactly one `/`). The
  namespace is **case-sensitive** and must match the GitHub username casing
  (`M0Rf30`). The GHCR image path stays lowercase (OCI requirement).
- OCI image carries the `io.modelcontextprotocol.server.name` label so the
  registry can verify ownership of the image ↔ server mapping.

## 2. GitHub Releases (binaries)

Already handled by `.goreleaser.yml` (`yap-mcp` build + per-binary archive).
Users install via:

```sh
curl -fsSL https://raw.githubusercontent.com/M0Rf30/yap/main/scripts/install.sh | sh
```

This is the path for manual / direct client config (no container).

## 3. Aggregators & awesome-lists (PR-based discovery)

Submit once; they index from GitHub thereafter. Each wants the repo URL +
`server.json` (or their own manifest):

| Channel | How |
| --- | --- |
| `modelcontextprotocol/servers` (Community Servers) | PR adding a list entry |
| `punkpeye/awesome-mcp-servers` | PR adding a list entry |
| Glama (`glama.ai/mcp`) | auto-indexes public GitHub MCP repos |
| PulseMCP (`pulsemcp.com`) | submission form |
| mcp.so | submission form |
| Smithery (`smithery.ai`) | add `smithery.yaml` (optional, for hosted deploy) |

## 4. Agent Skill channels

`skills/yap/SKILL.md` already follows the Anthropic Agent Skills layout.

| Channel | How |
| --- | --- |
| This repo | canonical source (`skills/yap/`) |
| `anthropics/skills` | PR to contribute as a community skill |
| Claude Code plugin marketplaces | bundle skill in a plugin repo with `plugin.json` |
| awesome-claude-skills lists | PR adding an entry |

## Release checklist

1. Tag `vX.Y.Z` → `release.yml` builds binaries, `mcp-publish.yml` builds the
   OCI image and publishes to the MCP registry.
2. Verify the image: `docker run -i --rm ghcr.io/m0rf30/yap-mcp:latest` (should
   block on stdio).
3. Verify the registry entry at
   `https://registry.modelcontextprotocol.io/v0/servers?search=yap`.
4. (First release only) open the aggregator + skill PRs above.
