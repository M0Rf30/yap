// Package nfpm implements an in-tree, dependency-free reader/writer for the
// goreleaser/nfpm ("nfpm.yaml") package specification format, enabling YAP to
// build deb/rpm/apk/pacman artifacts directly from an nfpm spec through the
// existing pkg/builders pipeline — without importing
// github.com/goreleaser/nfpm.
//
// The package is organised the same way as the other in-tree format
// reimplementations (pkg/deb822, pkg/sbom, pkg/apkindex):
//
//   - nfpm.go      — the nfpm.yaml schema (Config/Info/Overridables and every
//     per-packager sub-block: Scripts/RPM/Deb/APK/ArchLinux/IPK).
//   - contents.go  — the `contents:` list: type constants, glob/tree
//     expansion, and FileInfo default resolution (PrepareForPackager).
//   - load.go      — strict YAML decoding (Parse/ParseWithEnvMapping/Load),
//     ${VAR} expansion, signature-passphrase resolution, defaulting
//     (WithDefaults), validation (Validate), and per-packager override
//     resolution (ForPackager).
//   - version.go   — semver splitting (SplitVersion).
//   - detect.go    — nfpm spec file discovery (FindSpec/IsSpecFile).
//   - changelog.go — the goreleaser/chglog changelog dialect nfpm's
//     `changelog:` key points at, rendered into YAP's RPM/Debian changelog
//     text (RenderRPM/RenderDebian).
//
// Conversion to and from YAP's extended PKGBUILD dialect lives in
// convert_to.go, convert_from.go, render_yaml.go, render_pkgbuild.go and
// script.go, owned by separate slices of the same feature.
package nfpm
