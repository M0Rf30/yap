# nfpm specfile example

This example is a self-contained package described entirely in `nfpm.yaml`
(the [nfpm](https://nfpm.goreleaser.com/) format) instead of a `PKGBUILD`. It
demonstrates the same `contents:` primitives YAP's nfpm front-end supports:
a plain file, a `config|noreplace` file, an empty directory, and a symlink,
plus a `postinstall` script and a per-packager `overrides.rpm.depends`.

See [`docs/nfpm-support.md`](../../docs/nfpm-support.md) for the full
nfpm -> PKGBUILD field mapping and the list of nfpm features YAP deliberately
does not support.

## Layout

```text
examples/nfpm/
├── nfpm.yaml                       # the spec
├── files/
│   ├── usr/bin/hello-nfpm          # -> /usr/bin/hello-nfpm
│   └── etc/hello-nfpm/hello-nfpm.conf  # -> /etc/hello-nfpm/hello-nfpm.conf
└── scripts/
    └── postinstall.sh              # scripts.postinstall
```

`nfpm.yaml` resolves every relative `contents[].src` and `scripts.*` path
against this directory (the directory containing the spec file) — not
against the caller's current working directory, which is how upstream nfpm
resolves them. See "Path semantics" in `docs/nfpm-support.md`.

## Build it

`yap` auto-detects `nfpm.yaml` the same way it detects `PKGBUILD` — just
point `yap build` at this directory:

```bash
# Auto-detect host distro
yap build examples/nfpm

# Or target a specific distro/format
yap build ubuntu-jammy examples/nfpm   # .deb, via overrides.deb (none set, base applies)
yap build fedora-38 examples/nfpm      # .rpm, via overrides.rpm.depends above
yap build alpine examples/nfpm         # .apk
yap build arch examples/nfpm           # .pkg.tar.zst
```

Internally this parses `nfpm.yaml` into a synthesized `PKGBUILD` (via
`pkg/nfpm.Config.ToPKGBUILD`) whose `package()` function installs the
`contents:` entries above, then runs the same builder, signing (`--sign`),
and SBOM (`--sbom`) pipeline as a hand-written PKGBUILD.

## Convert it

Turn this spec into an editable PKGBUILD (e.g. to add a real `build()` step,
or to hand it to someone who prefers the Arch dialect):

```bash
yap convert examples/nfpm/nfpm.yaml --to pkgbuild -o examples/nfpm/PKGBUILD.generated
```

Convert only one packager's view (drops the other `__apt`/`__yum`/`__apk`/
`__pacman` suffixed directives, keeping unsuffixed base fields plus that
packager's overrides merged in):

```bash
yap convert examples/nfpm/nfpm.yaml --to pkgbuild --packager rpm -o -
```

Round-trip a PKGBUILD back into nfpm.yaml (useful once you've hand-edited the
generated PKGBUILD and want an nfpm consumer to see it):

```bash
yap convert examples/nfpm/PKGBUILD.generated --to nfpm -o -
```
