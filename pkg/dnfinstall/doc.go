// Package dnfinstall provides a pure-Go replacement for "dnf install" and
// "rpm --install" subprocesses.
//
// It resolves transitive package dependencies via dnfcache, downloads RPM
// files, and extracts them to a target filesystem root. This mirrors the
// functionality of pkg/aptinstall (for Debian) and pkg/apkindex (for Alpine).
//
// Phase 1 provides the public API and dependency resolution skeleton.
// Phase 2 implements RPM extraction from CPIO payloads with safe-path validation.
// Phases 3-6 (future) will add scriptlet execution, GPG verification, and rpmdb write.
package dnfinstall
