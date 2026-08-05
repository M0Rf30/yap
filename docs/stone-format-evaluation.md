# AerynOS `stone` format — evaluation

This is an **evaluation only**. No `stone`-related code ships in this change;
the nfpm work in this branch (`pkg/nfpm`, `yap convert`, the `convert_spec`
MCP tool) is unaffected by anything below. Everything here is grounded in a
research pass over `github.com/AerynOS/os-tools` (`crates/stone_recipe` and
`crates/stone`), captured 2026-08-04. Claims not directly confirmed against
that source are marked **[unverified]**; nothing here should be read as a
commitment to implement `stone` support.

AerynOS ("Serpent OS") uses two distinct formats that this document treats
separately: **`stone.yaml`**, the human-authored build recipe consumed by its
`boulder` builder (a specfile, analogous to `PKGBUILD`/`nfpm.yaml`), and
**`.stone`**, the binary package container `boulder` emits and `moss`
installs (analogous to a `.deb`/`.rpm`/`.apk` archive).

## Part A — the `stone.yaml` recipe

### Schema summary

Identity (required unless noted): `name` (string), `version` (string),
`release` (u64), `homepage` (string), `license` (`Vec<String>`, a single
scalar is also accepted), `summary` (optional string), `description`
(optional string).

Build phases, each an optional shell script body: `setup`, `build`,
`install`, `check`, `workload`, `environment`.

Dependencies: `builddeps`, `checkdeps`, `rundeps` (each `Vec<String>`,
also settable per-package — see below). Dependency strings support provider
forms beyond plain package names: `binary(name)`, `pkgconfig(name)`,
`soname(lib)`, `cmake(pkg)`, `python(mod)`.

Sources (`upstreams:`), a sequence of single-key maps:

- plain archive — key is the URL, value is `{hash, rename?, stripdirs?,
  unpack=true, unpackdir?}` (or a bare hash string)
- git — key is `git|<url>`, value is `{ref, clonedir?, staging?}`

Subpackages (`packages:`, a sequence of `{name: {...}}` maps), fields:
`summary`, `description`, `rundeps`, `rundeps-exclude`, `provides-exclude`,
`conflicts`, `paths` (a list of glob strings, or `{path: kind}` where `kind`
is `any` (default), `exe`, `symlink`, or `special`).

Tuning/options with no packaging-format analogue: `architectures`, `emul32`,
`mold`, `toolchain` (`llvm`/`gnu`), `debug`, `strip`, `lastrip`, `networking`,
`cspgo`, `samplepgo`, `compressman`, `tuning` (list of `{flag: enable |
disable | <config>}`), `profiles` (list of `{profile-name: {<build fields>}}`
for building N variants of one recipe).

Phase bodies can reference two flavours of macro, expanded by `boulder` from
its own data YAML before the shell body runs: definitions `%(name)` (e.g.
`%(prefix)`, `%(libdir)`, `%(bindir)`, `%(sysconfdir)`, `%(installroot)` —
the DESTDIR analogue, `%(cc)`, `%(cflags)`) and actions `%name` (e.g.
`%configure`, `%make`, `%make_install`, `%cmake_build`, `%meson_install`).
`boulder` runs each phase inside its own sandboxed rootfs (network off by
default); a bare shell interpreter cannot execute a `stone.yaml` phase body
verbatim without first expanding these macros.

### stone-key -> PKGBUILD-field mapping

This mirrors the nfpm mapping table in [`docs/nfpm-support.md`](nfpm-support.md)
in spirit — it is **not implemented**, only sketched to show recipe-level
`stone.yaml` support would slot into the same `pkg/parser.DetectSpec` seam
nfpm now uses.

| `stone.yaml` key | YAP PKGBUILD equivalent | Notes |
|---|---|---|
| `name` | `pkgname`, `pkgbase` | |
| `version` | `pkgver` | |
| `release` (u64) | `pkgrel` | |
| `homepage` | `url` | |
| `license` (`Vec<String>` or scalar) | `license` (array) | |
| `summary` | `pkgdesc` | preferred when both `summary` and `description` are set — PKGBUILD has one description field, `stone.yaml` has two |
| `description` | `pkgdesc` | fallback when `summary` is absent |
| `builddeps` | `makedepends` | |
| `checkdeps` | `makedepends` | **[inference]** YAP has no separate check-time dependency concept; would need to fold in, or be dropped with a warning like nfpm's `rpm.requires.post` |
| `rundeps` | `depends` | |
| `setup` phase | `prepare()` function body | after macro expansion |
| `build` phase | `build()` function body | after macro expansion |
| `install` phase | `package()` function body (single-package recipes) | after macro expansion; `%(installroot)` plays the role of `${pkgdir}` |
| `check` phase | `check()` function body | after macro expansion |
| `workload` phase | *(no equivalent)* | a post-install runtime smoke-test invocation for `moss`, not a build phase — nothing in PKGBUILD models this |
| `environment` phase | *(no single field)* | **[inference]** would need to prefix every synthesized phase body, the way `pkg/nfpm/script.go` synthesizes `package()` for nfpm `contents:` |
| `packages[].name` | `pkgname` array entry + a `package_<name>()` function | matches YAP's existing split-package mechanism (see `examples/split-package/PKGBUILD`) directly — this is the best-fitting row in the whole table |
| `packages[].summary` / `description` | `pkgdesc=` assignment inside that `package_<name>()` body | per-subpackage metadata in YAP is a shell variable set inside the function, not a top-level field |
| `packages[].rundeps` | `depends=` assignment inside that `package_<name>()` body | same mechanism |
| `packages[].paths` (globs) | install commands inside that `package_<name>() `body | analogous to how nfpm's `contents:` become synthesized install commands |
| `architectures` | `arch` array | **[inference]** needs its own name normalizer; `stone.yaml` architecture names are not confirmed to match `constants.NormalizeArchitecture`'s input set |

`rundeps-exclude`, `provides-exclude`, `conflicts` (subpackage-scoped),
`emul32`, `mold`, `toolchain`, `debug`/`strip`/`lastrip`, `networking`,
`cspgo`/`samplepgo`, `compressman`, `tuning`, and `profiles` have no PKGBUILD
analogue at all and would need the same "dropped with one logged message per
feature" treatment YAP already applies to nfpm's `ipk` section and signature
blocks.

## Part B — the binary `.stone` container

**[unverified beyond the research digest]** — there is no published `.stone`
format specification; `crates/stone` in `AerynOS/os-tools` is the only
normative source, and only format version 1 exists.

- **Header** — 32 bytes: 4-byte magic `\0mos`, 24 bytes of version-specific
  data, then a big-endian u32 version (currently `1`). The v1 data is
  `num_payloads` (u16 BE), a 21-byte integrity pattern, and `file_type` (u8:
  `Binary=1`, `Delta=2`, `Repository=3`, `BuildManifest=4`).
- **Payloads** — N of them follow the header, each with its own 39-byte
  header: `stored_size` (u64 BE), `plain_size` (u64 BE), an 8-byte checksum
  (XXH64 of the stored bytes), `num_records` (u32 BE), `version` (u16 BE),
  `kind` (u8: `Meta=1`, `Content=2`, `Layout=3`, `Index=4`, `Attributes=5`),
  and `compression` (u8: `None=1`, `Zstd=2`). The payload body follows,
  compressed independently of every other payload.
- **Meta payload** — records of `{length u32, tag u16, primitive_kind u8,
  pad u8, value}`. Known tags: `Name=1`, `Architecture=2`, `Version=3`,
  `Summary=4`, `Description=5`, `Homepage=6`, `SourceID=7`, `Depends=8`,
  `Provides=9`, `Conflicts=10`, `Release=11`, `License=12`,
  `BuildRelease=13`, `PackageURI=14`, `PackageHash=15`, `PackageSize=16`,
  `BuildDepends=17`, `SourceURI=18`, `SourcePath=19`, `SourceRef=20`.
  Dependency kinds: `PackageName=0`, `SharedLibrary=1`, `PkgConfig=2`,
  `Interpreter=3`, `CMake=4`, `Python=5`, `Binary=6`, `SystemBinary=7`,
  `PkgConfig32=8`.
- **Content payload** — a single zstd stream holding every *unique* file
  body concatenated together (`num_records = 0`); deduplication happens at
  this layer.
- **Index payload** — one `{start u64 BE, end u64 BE, digest u128 BE}`
  record per unique file. `start`/`end` are byte offsets into the
  **uncompressed** content stream; `digest` is the XXH128 of the file's
  plain (uncompressed) bytes.
- **Layout payload** — one `{uid u32, gid u32, mode u32, tag u32,
  source_len u16, target_len u16, file_type u8, 11 bytes padding, source,
  target}` record per filesystem entry. `file_type` is `Regular=1` (`source`
  is the 16-byte XXH128 digest that keys back into the Index payload),
  `Symlink=2` (`source` is the link-target string), `Directory=3`,
  `CharacterDevice=4`, `BlockDevice=5`, `Fifo=6`, `Socket=7`.
- **Attributes payload** — reserved, minimal observed use.
- There is no mandatory package-level signature; the only built-in integrity
  check is the per-payload XXH64 checksum. `moss install ./foo.stone` is
  reported to work directly against locally-built files.

## Part C — verdict

**Recipe-level (`stone.yaml`) support is a natural fit** for YAP's spec
front-end architecture — the same seam nfpm now uses via
`pkg/parser.DetectSpec`. `stone.yaml` is, like nfpm.yaml, a declarative YAML
document that reduces to the same shape YAP already parses PKGBUILDs into: a
handful of identity/dependency fields plus a small number of named shell
script phases. The concrete, scoped work such support would need is:

1. A `stone_recipe` YAML reader (`gopkg.in/yaml.v3`, in-tree, matching the
   nfpm/deb822/apkindex precedent — no new dependency).
2. A macro expander for `%(defs)` and `%actions`, since phase bodies are not
   directly runnable shell without it.
3. Phase mapping: `setup` -> `prepare()`, `build` -> `build()`,
   `install` -> `package()`, `check` -> `check()`.
4. Subpackage `packages[].paths` globs -> YAP's existing split-package
   mechanism (`pkgname` array + `package_<name>()` functions), which is
   already a better structural match for `stone.yaml` subpackages than it
   was for nfpm (nfpm has no subpackage concept at all).

**A Go `.stone` *writer* is not recommended right now.** Three concrete
blockers, not just general caution:

- **No published specification.** The Rust crate is the only normative
  source for the binary layout; every detail above was read out of that
  crate's source, not a spec document, and only format version 1 has ever
  existed to compare against.
- **The XXH128 variant is unpinned.** The Index payload's per-file digest is
  described only as "XXH128". `github.com/zeebo/xxh3` provides Go's XXH3-128,
  which is not necessarily bit-identical to whatever the Rust `xxhash-rust`
  crate produces under its `xxh128`/`xxh3` feature selection. This must be
  pinned and cross-checked against real `boulder`-built `.stone` files
  before any Go writer could be trusted — a mismatched variant silently
  produces packages that hash-check internally but that `moss` cannot
  extract, since `moss` recomputes and compares the same digest on install.
- **Content-stream offsets require a buffered/two-pass writer.** The Index
  payload's `start`/`end` offsets point into the *uncompressed* Content
  stream, which itself must be fully assembled (with cross-file
  deduplication already applied) before those offsets are known — a
  streaming, single-pass writer is not possible with this layout.

Reading (`.stone` extraction) is lower risk than writing, since a reader can
tolerate being wrong about a hash variant it never needs to reproduce — but
this evaluation does not extend to design a reader either. `zstd` support is
already present in YAP as a direct dependency
(`github.com/klauspost/compress/zstd`), which removes one otherwise-open
question for either direction.

**Nothing here is a promise of future work.** This document exists so that,
if `stone` support is prioritized later, the scoping questions above are
already answered instead of being rediscovered from scratch — not to
schedule that work.
