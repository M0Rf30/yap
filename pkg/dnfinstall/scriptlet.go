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

// scriptletTagPair maps a scriptlet kind to its body and interpreter tags.
type scriptletTagPair struct {
	bodyTag   int
	progTag   int
	kindName  string
	argValue  string // "1" for fresh install (v1 only)
}

// scriptletTags maps each scriptlet kind to its RPM header tags.
var scriptletTags = map[scriptletKind]scriptletTagPair{
	scriptletPreTrans: {
		bodyTag:  1151, // RPMTAG_PRETRANS
		progTag:  1152, // RPMTAG_PRETRANSPROG
		kindName: "pretrans",
		argValue: "1",
	},
	scriptletPreIn: {
		bodyTag:  rpmutils.PREIN,
		progTag:  rpmutils.PREINPROG,
		kindName: "prein",
		argValue: "1",
	},
	scriptletPostIn: {
		bodyTag:  rpmutils.POSTIN,
		progTag:  rpmutils.POSTINPROG,
		kindName: "postin",
		argValue: "1",
	},
	scriptletPostTrans: {
		bodyTag:  1153, // RPMTAG_POSTTRANS
		progTag:  1154, // RPMTAG_POSTTRANSPROG
		kindName: "posttrans",
		argValue: "1",
	},
}

// scriptletEnvAllowList defines which environment variables are safe to
// forward to RPM scriptlets. Mirrors rpm's own filtering.
var scriptletEnvAllowList = map[string]bool{
	"PATH":     true,
	"HOME":     true,
	"LANG":     true,
	"LC_ALL":   true,
	"LOGNAME":  true,
	"SHELL":    true,
	"TERM":     true,
	"USER":     true,
	"TMPDIR":   true,
	"TZ":       true,
	"COLORTERM": true,
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

	// Read interpreter from header. Default to /bin/sh.
	interpreter, _ := rpm.Header.GetString(tags.progTag)
	if interpreter == "" {
		interpreter = "/bin/sh"
	}

	// Get package name for logging.
	pkgName, _ := rpm.Header.GetString(rpmutils.NAME)

	// Detect Lua scriptlets and skip with warning.
	if interpreter == "<lua>" || strings.HasPrefix(interpreter, "<lua>") {
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

	// Prepare command.
	// RPM passes the script body as a file argument, but we'll use stdin
	// for simplicity (equivalent behavior).
	cmd := exec.CommandContext(ctx, interpreter, "-e") // #nosec G204 — interpreter is from RPM header
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
