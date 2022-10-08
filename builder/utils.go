package builder

import (
	"context"
	"os"
	"strings"

	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"
)

func runScript(cmds string) error {
	buildScript, _ := syntax.NewParser().Parse(strings.NewReader(cmds), "")

	runner, _ := interp.New(
		interp.Env(expand.ListEnviron(os.Environ()...,
		)),
		interp.StdIO(nil, os.Stdout, os.Stdout),
	)

	err := runner.Run(context.TODO(), buildScript)
	if err != nil {
		return err
	}

	return err
}
