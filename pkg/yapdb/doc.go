// Package yapdb provides a pure-Go state tracker for packages installed by YAP.
//
// It maintains a SQLite database of installed packages, their files, and
// capabilities (provides, requires, conflicts, obsoletes). This allows YAP
// to track installations across builds without modifying the system rpmdb,
// apk database, or dpkg status file.
//
// The database is stored at <rootDir>/var/lib/yap/installed.db by default.
// It is used by pkg/dnfinstall, pkg/aptinstall, and pkg/apkindex to record
// what was installed and enable conflict detection and uninstall tracking.
//
// Schema version 1 is the current stable version.
package yapdb
