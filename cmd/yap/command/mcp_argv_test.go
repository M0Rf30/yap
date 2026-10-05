package command_test

import (
	"strings"
	"testing"

	"github.com/spf13/pflag"

	"github.com/M0Rf30/yap/v2/cmd/yap/command"
	"github.com/M0Rf30/yap/v2/pkg/mcp"
)

func checkArgvFlags(t *testing.T, argv []string, sets ...*pflag.FlagSet) {
	t.Helper()

	for _, a := range argv {
		name, ok := strings.CutPrefix(a, "--")
		if !ok {
			continue
		}

		found := false

		for _, fs := range sets {
			if fs.Lookup(name) != nil {
				found = true
			}
		}

		if !found {
			t.Errorf("argv carries --%s, which the real CLI does not define", name)
		}
	}
}

// TestMCPBuildArgvUsesRealCLIFlags guards against the MCP dispatcher emitting
// flags the yap CLI does not define (cobra would reject them in the container).
func TestMCPBuildArgvUsesRealCLIFlags(t *testing.T) {
	b, r := command.BuildCommand(), command.RootCommand()
	if b.Flags().Lookup("skip-sync") == nil {
		t.Fatal("real build flagset looks empty")
	}

	checkArgvFlags(t, mcp.ContainerBuildArgvSample(),
		b.Flags(), b.PersistentFlags(), r.PersistentFlags())
}

func TestMCPPrepareArgvUsesRealCLIFlags(t *testing.T) {
	p, r := command.PrepareCommand(), command.RootCommand()
	argv := mcp.ContainerPrepareArgvSample()

	if len(argv) < 2 || argv[0] != "prepare" || argv[1] != "ubuntu-noble" {
		t.Fatalf("prepare argv head = %v", argv)
	}

	checkArgvFlags(t, argv, p.Flags(), p.PersistentFlags(), r.PersistentFlags())
}
