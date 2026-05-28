package dnfinstall

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/M0Rf30/yap/v2/pkg/errors"
	"github.com/M0Rf30/yap/v2/pkg/logger"
)

// ldconfigPrimary is the canonical ldconfig path on RPM distros.
const ldconfigPrimary = "/sbin/ldconfig"

// ldconfigCandidates lists the ldconfig binary paths probed in order. RPM
// distros place it at /sbin/ldconfig; the /usr-merged path and bare name
// (resolved via PATH) are fallbacks.
var ldconfigCandidates = []string{
	ldconfigPrimary,
	"/usr/sbin/ldconfig",
	"ldconfig",
}

// runLDConfig refreshes the dynamic linker cache (ld.so.cache) once per
// install transaction. When rootDir is a sandbox (not "" or "/") and the
// process is privileged, ldconfig runs chrooted into that root so the cache
// is built against the installed libraries rather than the host's.
//
// A missing ldconfig binary is not fatal: minimal images and fakeroot
// scenarios legitimately lack it, so the failure is logged and swallowed.
func runLDConfig(ctx context.Context, rootDir string) error {
	bin := findLDConfig(rootDir)
	if bin == "" {
		logger.Debug("ldconfig not found; skipping ld.so.cache refresh", "rootDir", rootDir)

		return nil
	}

	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, bin)

	chrooted := rootDir != "" && rootDir != "/"
	if chrooted {
		if os.Getuid() != 0 {
			// Non-root inside a build container: a chroot is not possible and
			// running host ldconfig against a sandbox root is meaningless.
			logger.Debug("skipping ldconfig, not running as root", "rootDir", rootDir)

			return nil
		}

		cmd.SysProcAttr = &syscall.SysProcAttr{Chroot: rootDir}
		cmd.Dir = "/"
	}

	var stdout, stderr bytes.Buffer

	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if stderr.Len() > 0 {
			logger.Warn("ldconfig stderr",
				"output", strings.TrimRight(stderr.String(), "\n"))
		}

		return errors.Wrap(err, errors.ErrTypeBuild, "ldconfig failed").
			WithOperation("runLDConfig").
			WithContext("binary", bin).
			WithContext("rootDir", rootDir)
	}

	logger.Debug("refreshed ld.so.cache", "binary", bin, "rootDir", rootDir)

	return nil
}

// findLDConfig returns the first usable ldconfig path. When chrooting into a
// sandbox root the candidate must exist *inside* that root; otherwise the
// host filesystem is probed and bare "ldconfig" is resolved via PATH.
func findLDConfig(rootDir string) string {
	chrooted := rootDir != "" && rootDir != "/"

	for _, c := range ldconfigCandidates {
		if strings.Contains(c, "/") {
			probe := c
			if chrooted {
				probe = rootDir + c
			}

			if _, err := os.Stat(probe); err == nil {
				return c
			}

			continue
		}

		// Bare name: only meaningful when not chrooting (PATH lookup).
		if !chrooted {
			if p, err := exec.LookPath(c); err == nil {
				return p
			}
		}
	}

	return ""
}
