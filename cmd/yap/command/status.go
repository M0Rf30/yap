package command

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"

	"github.com/M0Rf30/yap/v2/pkg/buildinfo"
	"github.com/M0Rf30/yap/v2/pkg/constants"
	"github.com/M0Rf30/yap/v2/pkg/platform"
)

// statusCmd provides system and environment information.
var statusCmd = &cobra.Command{
	Use:     commandStatus,
	GroupID: commandUtility,
	Aliases: []string{"info", "env"},
	Short:   "Display system status and environment information",
	Run: func(_ *cobra.Command, _ []string) {
		showSystemStatus()
	},
}

func showSystemStatus() {
	osRelease, _ := platform.ParseOSRelease()

	goVer := runtime.Version()
	if idx := strings.IndexByte(goVer, '-'); idx != -1 {
		goVer = goVer[:idx]
	}

	// System section
	pterm.DefaultSection.Println("System")

	sysRows := [][]string{
		{"Distro", osRelease.ID},
		{"Arch", fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH)},
		{"Go", goVer},
	}

	_ = pterm.DefaultTable.WithData(sysRows).Render()

	pterm.Println()

	// YAP section
	pterm.DefaultSection.Println("YAP")

	ver := strings.TrimPrefix(buildinfo.Version, "v")

	currentDir, _ := os.Getwd()
	projectStatus := "no yap.json in current directory"

	if _, err := os.Stat(filepath.Join(currentDir, "yap.json")); err == nil {
		projectStatus = currentDir
	}

	yapRows := [][]string{
		{"Version", ver},
		{"Project", projectStatus},
	}

	_ = pterm.DefaultTable.WithData(yapRows).Render()

	pterm.Println()

	// Distributions section
	pterm.DefaultSection.Println("Distributions")

	distroRows := [][]string{
		{"Supported", fmt.Sprintf("%d", len(constants.Releases))},
	}

	if verbose {
		families := make(map[string][]string)

		for _, release := range &constants.Releases {
			family := getDistroFamily(release)
			families[family] = append(families[family], release)
		}

		for family, distros := range families {
			distroRows = append(distroRows, []string{family, strings.Join(distros, ", ")})
		}
	}

	_ = pterm.DefaultTable.WithData(distroRows).Render()

	pterm.Println()

	// Environment section
	pterm.DefaultSection.Println("Environment")

	envRows := [][]string{
		{"Container runtime", checkContainerRuntime()},
	}

	_ = pterm.DefaultTable.WithData(envRows).Render()
}

func getDistroFamily(release string) string {
	switch {
	case strings.Contains(release, "ubuntu") || strings.Contains(release, distroFamilyDebian):
		return distroFamilyDebian
	case strings.Contains(release, "fedora") || strings.Contains(release, "rhel") ||
		strings.Contains(release, "centos") || strings.Contains(release, "rocky"):
		return distroFamilyRedhat
	case strings.Contains(release, "opensuse") || strings.Contains(release, "suse"):
		return "suse"
	case strings.Contains(release, archDistro):
		return archDistro
	case strings.Contains(release, alpineDistro):
		return alpineDistro
	default:
		return distroFamilyUnknown
	}
}

func checkContainerRuntime() string {
	for _, rt := range []string{"podman", "docker"} {
		if _, err := exec.LookPath(rt); err == nil {
			return rt
		}
	}

	return "not detected"
}

//nolint:gochecknoinits // Required for cobra command registration
func init() {
	rootCmd.AddCommand(statusCmd)
}
