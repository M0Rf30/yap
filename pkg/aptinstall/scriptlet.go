package aptinstall // nolint:revive // package comment is in aptinstall.go

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"

	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"

	"github.com/M0Rf30/yap/v2/pkg/logger"
)

// runScriptlet executes a maintainer scriptlet with the given arguments.
// scriptBody is the bash script text. action and args are passed as $1, $2, etc.
// Examples:
//
//	preinst install
//	preinst upgrade <old-version>
//	postinst configure <old-version>
//	prerm remove
//	postrm purge
func runScriptlet(ctx context.Context, scriptName, scriptBody, pkgName, action string, args ...string) error {
	if scriptBody == "" {
		return nil // No scriptlet to run.
	}

	logger.Info("Running maintainer scriptlet",
		"package", pkgName,
		"scriptlet", scriptName,
		"action", action)

	// Parse the script.
	file, err := syntax.NewParser().Parse(strings.NewReader(scriptBody), scriptName)
	if err != nil {
		return fmt.Errorf("parse %s: %w", scriptName, err)
	}

	// Build environment.
	env := append(os.Environ(),
		"DEBIAN_FRONTEND=noninteractive",
		"DPKG_MAINTSCRIPT_PACKAGE="+pkgName,
		"DPKG_MAINTSCRIPT_NAME="+scriptName,
	)

	// Build arguments: $0 is the script name, $1 is action, $2+ are additional args.
	scriptArgs := []string{scriptName, action}
	scriptArgs = append(scriptArgs, args...)

	// Create the interpreter.
	var stdout, stderr bytes.Buffer

	runner, err := interp.New(
		interp.Params(scriptArgs...),
		interp.Env(expand.ListEnviron(env...)),
		interp.StdIO(os.Stdin, &stdout, &stderr),
	)
	if err != nil {
		return fmt.Errorf("create interpreter: %w", err)
	}

	// Run the script.
	if err := runner.Run(ctx, file); err != nil {
		// Log the output for debugging.
		if stdout.Len() > 0 {
			logger.Debug("Scriptlet stdout", "output", stdout.String())
		}

		if stderr.Len() > 0 {
			logger.Warn("Scriptlet stderr", "output", stderr.String())
		}

		return fmt.Errorf("run %s: %w", scriptName, err)
	}

	// Log output if any.
	if stdout.Len() > 0 {
		logger.Debug("Scriptlet output", "scriptlet", scriptName, "output", stdout.String())
	}

	return nil
}
