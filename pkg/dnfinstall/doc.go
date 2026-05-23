// Package dnfinstall replaces "dnf install" and "rpm --install" subprocesses.
//
// It resolves transitive package dependencies via dnfcache, downloads RPM
// files, and extracts them to a target filesystem root. This mirrors the
// functionality of pkg/aptinstall (for Debian) and pkg/apkindex (for Alpine).
//
// It supports GPG signature verification, scriptlet execution (%pretrans, %pre,
// %post, %posttrans), and state tracking via yapdb. System rpmdb write is optional.
package dnfinstall
