package aptinstall // nolint:revive // package comment is in aptinstall.go

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/M0Rf30/yap/v2/pkg/errors"
	"github.com/M0Rf30/yap/v2/pkg/logger"
)

// scriptletEnvAllowList enumerates the environment variables we forward to
// maintainer scripts. Everything else (CI secrets, signing passphrases,
// cloud credentials) is stripped so a malicious or buggy postinst cannot
// exfiltrate them.
var scriptletEnvAllowList = map[string]bool{
	"PATH":        true,
	"HOME":        true,
	"LANG":        true,
	"LC_ALL":      true,
	"TERM":        true,
	"TZ":          true,
	"USER":        true,
	"LOGNAME":     true,
	"SHELL":       true,
	"TMPDIR":      true,
	"PWD":         true,
	"HOSTNAME":    true,
	"COLUMNS":     true,
	"LINES":       true,
	"NO_COLOR":    true,
	"FORCE_COLOR": true,
}

// filterScriptletEnv returns the subset of os.Environ() that's safe to
// forward to a maintainer script.
func filterScriptletEnv() []string {
	parent := os.Environ()
	filtered := make([]string, 0, len(parent))

	for _, kv := range parent {
		k, _, ok := strings.Cut(kv, "=")
		if !ok {
			continue
		}

		if scriptletEnvAllowList[k] {
			filtered = append(filtered, kv)
		}
	}

	// Always provide a sane PATH so /usr/bin lookups (debconf, dpkg-trigger,
	// update-alternatives, ldconfig) work even if the parent env stripped it.
	if !hasEnvKey(filtered, "PATH") {
		filtered = append(filtered, "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin")
	}

	return filtered
}

func hasEnvKey(env []string, key string) bool {
	prefix := key + "="
	for _, kv := range env {
		if strings.HasPrefix(kv, prefix) {
			return true
		}
	}

	return false
}

// runScriptlet executes a maintainer scriptlet via /bin/sh as a child
// process.
//
// Why /bin/sh and not mvdan.cc/sh: real Debian maintainer scripts rely
// on dpkg's exec semantics. Notably:
//
//   - debconf (/usr/share/debconf/confmodule, sourced by many postinsts)
//     re-execs the script as a child via Perl IPC::Open2 to wire it up
//     to the debconf protocol. That open2 spawn needs $0 to be a real
//     on-disk executable path; an in-process shell interpreter can't
//     satisfy it.
//   - update-alternatives, dpkg-trigger, ldconfig, addgroup, useradd,
//     systemctl, etc. all expect to be looked up in PATH and exec'd.
//   - mvdan.cc/sh refuses several builtins (umask, fg, bg) that real
//     scripts use without thought.
//
// The script must already exist at scriptPath on disk (written by
// writeDpkgInfoFiles before this is called). action and args become
// positional parameters $1, $2, ...; $0 is automatically the script
// path, matching dpkg's own behaviour.
func runScriptlet(
	ctx context.Context,
	scriptPath, scriptName, pkgName, action string,
	args ...string,
) error {
	// Sanity: the file must exist or /bin/sh will fail with a confusing
	// error message.
	if _, err := os.Stat(scriptPath); err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem, "scriptlet not on disk").
			WithOperation("runScriptlet").WithContext("scriptlet", scriptName).WithContext("path", scriptPath)
	}

	logger.Debug("running maintainer scriptlet",
		"package", pkgName,
		"scriptlet", scriptName,
		"action", action)

	cmdArgs := append([]string{scriptPath, action}, args...)
	cmd := exec.CommandContext(ctx, "/bin/sh", cmdArgs...)

	cmd.Env = append(filterScriptletEnv(),
		"DEBIAN_FRONTEND=noninteractive",
		"DPKG_MAINTSCRIPT_PACKAGE="+pkgName,
		"DPKG_MAINTSCRIPT_NAME="+scriptName,
	)

	// Capture output so we can attach it to errors and avoid noisy stderr
	// flooding the build log on success.
	var stdout, stderr bytes.Buffer

	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	if stdout.Len() > 0 {
		logger.Debug("scriptlet stdout",
			"scriptlet", scriptName, "output", strings.TrimRight(stdout.String(), "\n"))
	}

	if stderr.Len() > 0 {
		level := logger.Debug
		if err != nil {
			level = logger.Warn
		}

		level("scriptlet stderr",
			"scriptlet", scriptName, "output", strings.TrimRight(stderr.String(), "\n"))
	}

	if err != nil {
		return errors.Wrap(err, errors.ErrTypeBuild, "scriptlet execution failed").
			WithOperation("runScriptlet").WithContext("scriptlet", scriptName).WithContext("package", pkgName)
	}

	return nil
}

// dpkgInfoDir holds the per-package metadata + maintainer scripts.
// It is the directory writeDpkgInfoFiles populates.
const dpkgInfoDir = "/var/lib/dpkg/info"

// scriptletPathForPackage returns the on-disk path that
// writeDpkgInfoFiles uses for the given (pkgName, arch, scriptName)
// tuple. Used by installPackage to hand runScriptlet the right path.
func scriptletPathForPackage(pkgName, arch, control, scriptName string) string {
	baseName := pkgName
	// Multi-Arch: same uses pkgname:arch — see writeDpkgInfoFiles.
	if arch != "" && strings.Contains(control, "Multi-Arch: same") {
		baseName = pkgName + ":" + arch
	}

	return filepath.Join(dpkgInfoDir, baseName+"."+scriptName)
}
