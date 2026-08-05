# nfpm specfile support

YAP can build packages directly from an [nfpm](https://nfpm.goreleaser.com/)
`nfpm.yaml` specfile, in addition to its native extended-PKGBUILD dialect, and
can convert between the two formats.

## What is supported

- **`yap build [distro] <dir>`** — when `<dir>` contains a recognised nfpm
  spec file (`nfpm.yaml`, `nfpm.yml`, `.nfpm.yaml`, or `.nfpm.yml`) and no
  `PKGBUILD`, YAP parses it, converts it in memory into a `pkgbuild.PKGBUILD`,
  and runs it through the existing deb/rpm/apk/pacman builders unchanged —
  including signing (`--sign`) and SBOM generation (`--sbom`). If both a
  `PKGBUILD` and an nfpm spec exist in the same directory, `PKGBUILD` wins.
- **`yap convert <spec> [--to pkgbuild|nfpm] [--packager NAME] [-o out]`** —
  converts an nfpm spec into a PKGBUILD, or a PKGBUILD into an nfpm spec, in
  either direction. `--to` defaults to whichever dialect `<spec>` is not.
- **MCP `convert_spec` tool** — the same conversion, callable by an
  MCP-compatible client; see [`MCP.md`](../MCP.md).

Converting is lossy in both directions — see the tables below for exactly
what carries over and what does not.

## Field mapping: nfpm -> PKGBUILD

This is the mapping `pkg/nfpm.Config.ToPKGBUILD` applies (and
`pkg/nfpm.FromPKGBUILD` mirrors in reverse, where a reverse mapping exists).

| nfpm | PKGBUILD |
|---|---|
| `name` | `PkgName`, `PkgBase` |
| `archlinux.pkgbase` | `PkgBase` (archlinux packager only) |
| `version` (+`prerelease`/`version_metadata`) | `PkgVer` — see "Version semantics" below |
| `release` | `PkgRel` |
| `epoch` | `Epoch` |
| `description` | `PkgDesc` |
| `rpm.summary` | `PkgDesc` when packager is `rpm` and set |
| `section` | `Section` |
| `priority` | `Priority` |
| `maintainer` | `Maintainer` |
| `homepage` | `URL` |
| `license` | `License` (single-element slice) |
| `arch` (`all` -> `any`) | `Arch`, via `constants.NormalizeArchitecture` |
| `depends` | `Depends` |
| `recommends` | `OptDepends` (YAP renders these as Debian `Recommends:`) |
| `suggests` | `Suggests` |
| `conflicts` | `Conflicts` |
| `replaces` | `Replaces` |
| `provides` | `Provides` |
| `deb.breaks` | `Breaks` |
| `deb.predepends` | `PreDepends` |
| `deb.fields["Bugs"]` | `Bugs` |
| `deb.fields["Multi-Arch"]` | `MultiArch` |
| `deb.fields["Source"]` | `SourcePkg` |
| `deb.fields["Built-Using"]` | `BuiltUsing` (comma-split) |
| `rpm.group` | `Group` |
| `scripts.preinstall` / `postinstall` / `preremove` / `postremove` | `PreInst` / `PostInst` / `PreRm` / `PostRm` — file read, shebang and leading blank lines stripped, body indented by two spaces |
| `rpm.scripts.pretrans` / `posttrans` | `PreTrans` / `PostTrans` |
| `apk.scripts.preupgrade` / `postupgrade`, `archlinux.scripts.*` | `PreUpgrade` / `PostUpgrade` |
| `deb.scripts.templates` / `deb.scripts.config` | `DebTemplate` / `DebConfig` (paths, kept relative to the spec directory) |
| `contents[]` of type `config*` | `Backup` (destination with the leading `/` stripped, matching YAP convention) |
| `changelog` | `ChangelogData` (rendered; see "Changelog" below) |

`contents:` entries themselves become the synthesized `package()` function
body (see "Path semantics" below) — they have no single PKGBUILD field, since
PKGBUILD expresses installed files as shell commands, not declarative
manifest.

`SourceURI`/`HashSums` are always left empty: nfpm specs have no concept of
upstream sources to fetch and verify, only files already present on disk.
`Options` is likewise left empty, but with `StripEnabled`, `ZipManEnabled`,
and `DebugEnabled` all forced off for nfpm-sourced specs, so prebuilt
binaries referenced by `contents:` ship byte-identical to what nfpm would
have produced — YAP's builders must not strip or repack them.

### Round-trip limitations (PKGBUILD -> nfpm)

`pkg/nfpm.FromPKGBUILD` mirrors the table above where a PKGBUILD-side
directive exists to read back, but two rows in it are one-way in practice:

- **`changelog`** — nfpm's `changelog:` points at a goreleaser/chglog YAML
  *source* file; `PKGBUILD.ChangelogData`/`Changelog` carry already-rendered
  Debian/RPM changelog *text* with no chglog YAML to recover. Converting
  PKGBUILD -> nfpm therefore leaves `Config.Changelog` empty rather than
  guessing at a source document that was never captured.
- **`rpm.group`** — `PKGBUILD.Group` has no textual PKGBUILD directive (there
  is no `group=` keyword in the PKGBUILD parser's dispatch table, unlike
  `multiarch=`/`source_pkg=`/`builtusing=`, which do parse and do round-trip
  into `deb.fields`). `FromPKGBUILD` still reads `Group` off an in-memory
  `*pkgbuild.PKGBUILD` when one is available, but a value set only by writing
  `group=...` into PKGBUILD text and re-parsing it will not survive, since
  nothing populates that field from text in the first place.

## Deliberately unsupported nfpm features

These nfpm fields are recognised but dropped during conversion. Nothing
errors; each drop logs one warning (CLI) or is returned as one message per
feature in the `[]string` result of `ToPKGBUILD`/`FromPKGBUILD` (MCP surfaces
these too). The reasons are architectural, not oversights:

| nfpm feature | Why it is dropped |
|---|---|
| `*.signature.*` (deb/rpm/apk signature blocks) | YAP signs packages uniformly after the build via `--sign` / `yap.json` `signing` config (`pkg/signing`), independent of package format. A per-format signature block in the spec would fight that pipeline instead of feeding it. |
| `rpm.compression`, `deb.compression` | YAP already exposes compression as a build-time choice: `--compression-rpm` / `--compression-deb` (`zstd`/`gzip`/`xz`), applied uniformly across a build run, not pinned per spec file. |
| `deb.triggers.*` | YAP's deb builder has no dpkg trigger support. |
| `deb.scripts.rules` | YAP does not run `debian/rules` or any external build-system driver; builds happen through the PKGBUILD `build()`/`package()` shell functions. |
| `deb.arch_variant` | YAP does not track Debian architecture qualifiers (e.g. `armhf` vs `armel` variants) separately from `Arch`. |
| unrecognised `deb.fields` keys | Only `Bugs`, `Multi-Arch`, `Source`, and `Built-Using` have a PKGBUILD destination; arbitrary custom `debian/control` fields have none. |
| `rpm.buildhost` | YAP's RPM builder always derives build-host metadata at build time for reproducibility; there is no per-spec override. |
| `rpm.packager` | PKGBUILD already carries `maintainer`; RPM's separate "packager" field has no distinct YAP concept. |
| `rpm.prefixes` | Relocatable-RPM prefixes are not modeled by YAP's RPM builder. |
| `rpm.requires.post` | YAP has no equivalent of RPM's "post-transaction requires" distinct from ordinary `Depends`. |
| `rpm.ghost_files` | YAP's RPM builder has no ghost-file concept (files the package owns but does not install). |
| `rpm.scripts.verify` | YAP's RPM builder does not support `%verifyscript`. |
| `contents[]` of type `ghost` | Same reason as `rpm.ghost_files`. |
| `contents[]` of type `debian changelog` | YAP's changelog handling is driven entirely by the top-level `changelog:` field (see below), not by a synthetic contents entry. |
| `vendor` | No PKGBUILD field tracks upstream vendor attribution independent of `maintainer`. |
| the whole `ipk` section | YAP has no ipk (OpenWrt/opkg) builder, so nothing consumes it. |
| `file_info.lang` | YAP's builders have no per-locale doc-file segregation (RPM's `%lang` tag). |

## PKGBUILD -> nfpm: what does not carry over

Converting the other direction (`pkg/nfpm.FromPKGBUILD`) mirrors the table
above wherever a PKGBUILD directive has an nfpm destination — including
`multiarch=`, `source_pkg=`, and `builtusing=`, which round-trip into
`deb.fields["Multi-Arch"]`/`["Source"]`/`["Built-Using"]`. What is left
behind, one dropped-with-message entry each:

| PKGBUILD feature | Why it is dropped |
|---|---|
| `prepare()` / `build()` / `check()` function bodies | nfpm specs describe already-built artifacts; nothing in the schema executes a build step. |
| helper functions | Same reason — nfpm has no shell execution model at all outside `scripts.*`. |
| `source`/`sha*sums`/`b2sums`/`cksums` arrays | nfpm has no upstream-source-fetching concept; `contents:` only ever references files already on disk. |
| custom variables and custom arrays | Free-form PKGBUILD variables have no structured nfpm destination. |
| `makedepends` | nfpm has no build-time-dependency field — its `depends`/`recommends`/etc. are all runtime. |
| `options` (`!strip`, `!zipman`, ...) | nfpm has no per-package build-tuning option list. |
| arch-specific (`_x86_64`) and distro-specific (`__ubuntu`) directive variants | nfpm has one flat value per field, or `overrides:` per packager — neither models YAP's per-arch/per-distro variable suffixes. |
| `install=` (pacman `.install` script reference) | Superseded by fields nfpm does have (`scripts.*`, `apk.scripts.*`, `archlinux.scripts.*`) — the pacman `.install` *file* concept itself has no nfpm slot. |
| `copyright` | No nfpm field distinct from `license`/`maintainer`. |

`pkgname` **arrays** (split packages) are the one case that is not a soft
drop: `FromPKGBUILD` returns an error instead, since nfpm's schema has no
split-package concept at all — there would be no way to represent a second
`pkgname` in the emitted `nfpm.yaml`, and a silent partial conversion (first
sub-package only, say) would be worse than failing loudly.

## Version semantics

nfpm and YAP both compose a version string from several parts, but the two
package-format families that support build-metadata separators (`~`, `+`)
diverge from the two that don't:

- **`deb` and `rpm`**: `PkgVer` = `version` + (`~` + `prerelease` if set) +
  (`+` + `version_metadata` if set) — e.g. `version: 1.2.3`,
  `prerelease: beta.1` and `version_metadata: gitabc123` produce
  `1.2.3~beta.1+gitabc123`. Both dpkg and RPM version comparison understand
  `~` as "sorts before the release it qualifies" and accept `+` in general
  use, so the full nfpm version is preserved losslessly.
- **`archlinux` and `apk`**: `PkgVer` = `version` + (`_` + `prerelease` with
  every `-` replaced by `_`, if set). Neither pacman's nor apk's version
  comparator tolerates `~` or `+` in a version string the way dpkg/rpm do, so
  those separators are never emitted for these two families, and
  `version_metadata` has no representation at all in the composed version —
  it is silently dropped for `archlinux`/`apk` targets specifically (not
  logged separately from the rest of this table, since it is version-scoped
  rather than field-scoped).
- **`release`** maps straight to `PkgRel` and **`epoch`** straight to
  `Epoch` for every packager — no format-specific divergence there.

## Path semantics

`contents[].src`, `scripts.*`, `deb.scripts.templates`, and
`deb.scripts.config` are all resolved **relative to the directory containing
the nfpm spec file** (`Config.BaseDir()`, set by `nfpm.Load`/`SetBaseDir`) —
the same directory `yap build`/`yap convert` was pointed at.

This differs from upstream `nfpm package -f nfpm.yaml`, which resolves these
same relative paths against the **process's current working directory**
regardless of where `nfpm.yaml` lives. An nfpm spec that only ever gets built
from its own directory behaves identically either way; one built from
elsewhere (`nfpm package -f some/other/dir/nfpm.yaml` from a different cwd)
will not. Keep this in mind when porting an existing nfpm.yaml into YAP —
either build/convert from the spec's own directory, or check that all
relative paths in the spec still make sense.

## Changelog

nfpm's `changelog:` field does not point at a Debian changelog or an RPM
`%changelog` block directly — it points at a
[goreleaser/chglog](https://github.com/goreleaser/chglog) YAML document (a
list of version entries, each with a date, packager, and a list of
commit/note/author changes). YAP parses this file
(`pkg/nfpm.LoadChangelog`) and re-renders it itself:

- into **Debian changelog(5)** text for `deb` builds (`RenderDebian`)
- into the **RPM `%changelog`** dialect for every other builder — including
  Pacman and APK, which reuse YAP's existing RPM-style changelog renderer —
  via `RenderRPM`

The rendered bytes are attached to the synthesized PKGBUILD's new
`ChangelogData` field, which `pkgbuild.PKGBUILD.ReadChangelog` returns
verbatim instead of reading a changelog path from disk — nfpm-sourced specs
never need a native-format changelog file on disk.

### `yap convert` and the changelog

`yap convert nfpm.yaml -> PKGBUILD` renders the same chglog YAML with
`RenderRPM` and writes it to a `<name>.changelog` sidecar next to the
generated `PKGBUILD`, which carries a matching `changelog=` directive (the
text-render path has no `ChangelogData` field to attach the bytes to, so a
sidecar file is the only way to keep the two in sync). Converting to stdout
(`--output -`) has nowhere to put a sidecar: the `changelog=` directive is
still emitted, but the file is not written, and the conversion reports this
as a dropped-feature message.

## Worked example

[`examples/nfpm/`](../examples/nfpm) is a complete, buildable nfpm spec: a
file, a `config|noreplace` file, a directory, a symlink, a `postinstall`
script, and an `overrides.rpm.depends` override. See
[`examples/nfpm/README.md`](../examples/nfpm/README.md) for the exact build
and convert commands, summarized here:

```bash
# Build straight from the nfpm spec — no PKGBUILD involved.
yap build examples/nfpm
yap build fedora-38 examples/nfpm

# Convert it into an editable PKGBUILD.
yap convert examples/nfpm/nfpm.yaml --to pkgbuild -o examples/nfpm/PKGBUILD.generated

# Convert a PKGBUILD back into nfpm.yaml.
yap convert examples/nfpm/PKGBUILD.generated --to nfpm -o -
```
