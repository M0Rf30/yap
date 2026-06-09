package dnfinstall

import (
	"github.com/M0Rf30/yap/v2/pkg/safepath"
)

// safeRPMPath joins rootDir with a sanitised CPIO entry name, rejecting
// traversal attempts and entries that resolve to the root itself.
// Containment logic lives in pkg/safepath.
func safeRPMPath(rootDir, name string) (string, error) {
	return safepath.JoinStrict(rootDir, name)
}

// safeRPMSymlinkTarget validates that a symlink's target stays under
// rootDir. Absolute targets are permitted (common in RPM packages; they
// only resolve at runtime); relative targets are resolved against the
// symlink's own location. Containment logic lives in pkg/safepath.
func safeRPMSymlinkTarget(rootDir, linkPath, target string) error {
	return safepath.SymlinkTarget(rootDir, linkPath, target)
}
