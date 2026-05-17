package command

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"

	"github.com/M0Rf30/yap/v2/pkg/buildinfo"
)

var versionCmd = &cobra.Command{
	Use:     commandVersion,
	GroupID: commandUtility,
	Short:   "Display version and build information",
	Run: func(_ *cobra.Command, _ []string) {
		printVersion()
	},
}

func printVersion() {
	ver := strings.TrimPrefix(buildinfo.Version, "v")

	commit := buildinfo.Commit
	if len(commit) > 7 {
		commit = commit[:7]
	}

	buildTime := buildinfo.BuildTime

	goVer := runtime.Version()
	// Strip internal build tags (e.g. "go1.26.3-X:nodwarf5" → "go1.26.3")
	if idx := strings.IndexByte(goVer, '-'); idx != -1 {
		goVer = goVer[:idx]
	}

	rows := [][]string{
		{"Version", ver},
		{"Commit", commit},
		{"Build time", buildTime},
		{"Go version", goVer},
		{"OS/Arch", fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH)},
	}

	data := make([][]string, 0, len(rows)+1)
	data = append(data, []string{"Field", "Value"})
	data = append(data, rows...)

	_ = pterm.DefaultTable.
		WithHasHeader().
		WithData(data).
		Render()
}

//nolint:gochecknoinits // Required for cobra command registration
func init() {
	rootCmd.AddCommand(versionCmd)
}
