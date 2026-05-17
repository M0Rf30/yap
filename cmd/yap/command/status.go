package command

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"github.com/M0Rf30/yap/v2/pkg/buildinfo"
	"github.com/M0Rf30/yap/v2/pkg/color"
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
	fmt.Print(color.Section("System"))

	sysRows := [][]string{
		{"Distro", osRelease.ID},
		{"Arch", fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH)},
		{"Go", goVer},
	}

	fmt.Print(color.Table(sysRows, false))

	fmt.Println()

	// YAP section
	fmt.Print(color.Section("YAP"))

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

	fmt.Print(color.Table(yapRows, false))

	fmt.Println()

	// Distributions section
	fmt.Print(color.Section("Distributions"))

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

	fmt.Print(color.Table(distroRows, false))

	fmt.Println()

	// Environment section
	fmt.Print(color.Section("Environment"))

	envRows := [][]string{
		{"Container runtime", checkContainerRuntime()},
	}

	fmt.Print(color.Table(envRows, false))
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
