package builder

import (
	"context"
	"os"
	"strings"

	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"
)

// RunScript runs a shell script.
//
// It takes a string parameter `cmds` which represents the shell script to be executed.
// The function returns an error if there was an issue running the script.
func RunScript(cmds string) error {
	script, _ := syntax.NewParser().Parse(strings.NewReader(cmds), "")

	runner, _ := interp.New(
		interp.Env(expand.ListEnviron(os.Environ()...)),
		interp.StdIO(nil, os.Stdout, os.Stdout),
	)

	return runner.Run(context.TODO(), script)
}
