package dnfinstall

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	rpmutils "github.com/sassoftware/go-rpmutils"

	"github.com/M0Rf30/yap/v2/pkg/errors"
	"github.com/M0Rf30/yap/v2/pkg/logger"
)

// scriptletKind identifies which scriptlet to run.
type scriptletKind int

const (
	scriptletPreTrans scriptletKind = iota
	scriptletPreIn
	scriptletPostIn
	scriptletPostTrans
)

// Scriptlet kind names (as used in RPM tags and log output).
const (
	kindPreTrans  = "pretrans"
	kindPreIn     = "prein"
	kindPostIn    = "postin"
	kindPostTrans = "posttrans"
)

// Common interpreter paths used in RPM scriptlets.
const (
	interpSh   = "/bin/sh"
	interpBash = "/bin/bash"
)

// scriptletTagPair maps a scriptlet kind to its body and interpreter tags.
type scriptletTagPair struct {
	bodyTag  int
	progTag  int
	kindName string
	argValue string // "1" for fresh install (v1 only)
}

// scriptletTags maps each scriptlet kind to its RPM header tags.
var scriptletTags = map[scriptletKind]scriptletTagPair{
	scriptletPreTrans: {
		bodyTag:  1151, // RPMTAG_PRETRANS
		progTag:  1152, // RPMTAG_PRETRANSPROG
		kindName: kindPreTrans,
		argValue: "1",
	},
	scriptletPreIn: {
		bodyTag:  rpmutils.PREIN,
		progTag:  rpmutils.PREINPROG,
		kindName: kindPreIn,
		argValue: "1",
	},
	scriptletPostIn: {
		bodyTag:  rpmutils.POSTIN,
		progTag:  rpmutils.POSTINPROG,
		kindName: kindPostIn,
		argValue: "1",
	},
	scriptletPostTrans: {
		bodyTag:  1153, // RPMTAG_POSTTRANS
		progTag:  1154, // RPMTAG_POSTTRANSPROG
		kindName: kindPostTrans,
		argValue: "1",
	},
}

// scriptletEnvAllowList defines which environment variables are safe to
// forward to RPM scriptlets. Mirrors rpm's own filtering.
var scriptletEnvAllowList = map[string]bool{
	"PATH":        true,
	"HOME":        true,
	"LANG":        true,
	"LC_ALL":      true,
	"LOGNAME":     true,
	"SHELL":       true,
	"TERM":        true,
	"USER":        true,
	"TMPDIR":      true,
	"TZ":          true,
	"COLORTERM":   true,
	"FORCE_COLOR": true,
}

// filterScriptletEnv returns a filtered environment suitable for scriptlet
// execution. Mirrors the deb/apt approach but for RPM.
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

	// Always provide a sane PATH so lookups (ldconfig, systemctl, etc.) work.
	if !hasEnvKey(filtered, "PATH") {
		filtered = append(filtered, "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin")
	}

	// Always provide HOME.
	if !hasEnvKey(filtered, "HOME") {
		filtered = append(filtered, "HOME=/")
	}

	return filtered
}

// looksLikeLua returns true when the body appears to be an RPM Lua scriptlet
// rather than POSIX shell. Some RPMs (notably json-c-devel on EL8) omit the
// PROG tag for Lua bodies, so we have to detect the language from content.
//
// Heuristics: presence of common RPM-Lua API tokens that would never appear
// as shell builtins/commands.
func looksLikeLua(body string) bool {
	// Only match tokens that are impossible in a POSIX shell
	// scriptlet but are idiomatic in rpm-Lua. The RPM-Lua API exposes
	// `path`, `posix`, `rpm`, `hashlib`, `macros`, `fd`, etc. as global
	// tables, so a leading-token call like "path.something(" or
	// "rpm.execute(" is a strong signal.
	luaMarkers := []string{
		"path.",
		"posix.",
		"rpm.b64",
		"rpm.define",
		"rpm.execute",
		"rpm.expand",
		"rpm.spawn",
		"hashlib.",
		"macros.",
	}
	for _, m := range luaMarkers {
		if strings.Contains(body, m) {
			return true
		}
	}

	return false
}

// hasEnvKey checks if an environment variable key exists in the list.
func hasEnvKey(env []string, key string) bool {
	prefix := key + "="
	for _, kv := range env {
		if strings.HasPrefix(kv, prefix) {
			return true
		}
	}

	return false
}

// runScriptlet executes a single scriptlet from the RPM header.
// Returns nil if no scriptlet body for the given kind, or if SkipScriptlets is set.
// arg = "1" for fresh install (always 1 for v1 — no upgrades).
//
//nolint:gocyclo,cyclop // scriptlet orchestration (env, interpreter, chroot, output) is inherently branchy
func runScriptlet(
	ctx context.Context,
	kind scriptletKind,
	rpm *rpmutils.Rpm,
	rootDir string,
	opts Options,
) error {
	// If scriptlets are disabled, skip.
	if opts.SkipScriptlets {
		return nil
	}

	tags := scriptletTags[kind]

	// Read scriptlet body from header.
	body, err := rpm.Header.GetString(tags.bodyTag)
	if err != nil || body == "" {
		// No scriptlet for this kind — not an error.
		return nil
	}

	// Read interpreter from header. PROG tags are commonly stored as
	// STRING_ARRAY (e.g. ["<lua>"], ["/sbin/ldconfig"], ["/bin/sh", "-e"]),
	// so use GetStrings and take the first non-empty entry.
	interpreter := interpSh

	var interpreterArgs []string

	if progs, err := rpm.Header.GetStrings(tags.progTag); err == nil && len(progs) > 0 {
		if progs[0] != "" {
			interpreter = progs[0]
			if len(progs) > 1 {
				interpreterArgs = progs[1:]
			}
		}
	}

	// Get package name for logging.
	pkgName, _ := rpm.Header.GetString(rpmutils.NAME)

	// Detect Lua scriptlets and skip with warning. Some RPMs (notably
	// json-c-devel on EL8) ship Lua bodies without setting the PROG tag,
	// so also heuristically detect Lua syntax in the body when the
	// declared interpreter is the default /bin/sh fallback.
	if interpreter == "<lua>" || strings.HasPrefix(interpreter, "<lua>") || looksLikeLua(body) {
		logger.Warn("skipping Lua scriptlet",
			"kind", tags.kindName,
			"package", pkgName,
			"interpreter", interpreter)

		return nil
	}

	logger.Debug("running RPM scriptlet",
		"package", pkgName,
		"kind", tags.kindName,
		"interpreter", interpreter)

	// Create a context with timeout.
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	// Prepare command. Use explicit interpreter args from the header if
	// present (e.g. ["-p", "<lua>"]); otherwise fall back to "-e" for
	// shell interpreters so non-zero exits propagate as in rpm.
	args := interpreterArgs
	if len(args) == 0 && (interpreter == interpSh ||
		interpreter == "/usr/bin/sh" ||
		interpreter == interpBash ||
		interpreter == "/usr/bin/bash") {
		args = []string{"-e"}
	}

	cmd := exec.CommandContext(ctx, interpreter, args...)
	cmd.Stdin = strings.NewReader(body)

	// Set up environment.
	cmd.Env = append(filterScriptletEnv(),
		"RPM_PACKAGE_NAME="+pkgName,
	)

	// Add version and release if available.
	if version, err := rpm.Header.GetString(rpmutils.VERSION); err == nil && version != "" {
		cmd.Env = append(cmd.Env, "RPM_PACKAGE_VERSION="+version)
	}

	if release, err := rpm.Header.GetString(rpmutils.RELEASE); err == nil && release != "" {
		cmd.Env = append(cmd.Env, "RPM_PACKAGE_RELEASE="+release)
	}

	// Handle chroot if rootDir is set and not "/".
	if rootDir != "" && rootDir != "/" {
		if os.Getuid() == 0 {
			// Running as root: use chroot.
			cmd.SysProcAttr = &syscall.SysProcAttr{
				Chroot: rootDir,
			}
			cmd.Dir = "/"
		} else {
			// Not root: log debug and run anyway (container build scenario).
			logger.Debug("skipping chroot, not running as root",
				"kind", tags.kindName,
				"package", pkgName,
				"rootDir", rootDir)
			cmd.Dir = rootDir
		}
	}

	// Capture output.
	var stdout, stderr bytes.Buffer

	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Run the scriptlet.
	err = cmd.Run()

	// Log output.
	if stdout.Len() > 0 {
		logger.Debug("scriptlet stdout",
			"kind", tags.kindName,
			"package", pkgName,
			"output", strings.TrimRight(stdout.String(), "\n"))
	}

	if stderr.Len() > 0 {
		level := logger.Debug
		if err != nil {
			level = logger.Warn
		}

		level("scriptlet stderr",
			"kind", tags.kindName,
			"package", pkgName,
			"output", strings.TrimRight(stderr.String(), "\n"))
	}

	// Handle execution errors.
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeBuild, "scriptlet execution failed").
			WithOperation("runScriptlet").
			WithContext("kind", tags.kindName).
			WithContext("package", pkgName).
			WithContext("interpreter", interpreter)
	}

	return nil
}
