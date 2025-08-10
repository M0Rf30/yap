// Package constants provides centralized constants and mappings for all package formats.
package constants

// RPMGroups maps common group names to RPM group categories.
// This consolidates the group mapping logic from the RPM package.
var RPMGroups = map[string]string{
	"admin":        "Applications/System",
	"any":          "noarch",
	"comm":         "Applications/Communications",
	"database":     "Applications/Databases",
	"debug":        "Development/Debuggers",
	"devel":        "Development/Tools",
	"doc":          "Documentation",
	"editors":      "Applications/Editors",
	"electronics":  "Applications/Engineering",
	"embedded":     "Applications/Engineering",
	"fonts":        "Interface/Desktops",
	"games":        "Amusements/Games",
	"graphics":     "Applications/Multimedia",
	"httpd":        "Applications/Internet",
	"interpreters": "Development/Tools",
	"kernel":       "System Environment/Kernel",
	"libdevel":     "Development/Libraries",
	"libs":         "System Environment/Libraries",
	"localization": "Development/Languages",
	"mail":         "Applications/Communications",
	"math":         "Applications/Productivity",
	"misc":         "Applications/System",
	"net":          "Applications/Internet",
	"news":         "Applications/Publishing",
	"science":      "Applications/Engineering",
	"shells":       "System Environment/Shells",
	"sound":        "Applications/Multimedia",
	"text":         "Applications/Text",
	"vcs":          "Development/Tools",
	"video":        "Applications/Multimedia",
	"web":          "Applications/Internet",
	"x11":          "User Interface/X",
}

// RPMDistros maps distribution names to their RPM suffix.
var RPMDistros = map[string]string{
	"almalinux": ".el",
	"amzn":      ".amzn",
	"fedora":    ".fc",
	"ol":        ".ol",
	"rhel":      ".el",
	"rocky":     ".el",
}

// BuildEnvironmentDeps provides build environment dependencies for each package manager.
type BuildEnvironmentDeps struct {
	APK    []string
	DEB    []string
	RPM    []string
	Pacman []string
}

// GetBuildDeps returns the build environment dependencies for all package formats.
func GetBuildDeps() *BuildEnvironmentDeps {
	return &BuildEnvironmentDeps{
		APK: []string{
			"alpine-sdk",
		},
		DEB: []string{
			"build-essential",
			"fakeroot",
		},
		RPM: []string{
			"autoconf",
			"automake",
			"diffutils",
			"expect",
			"gcc",
			"gcc-c++",
			"libtool-ltdl",
			"libtool-ltdl-devel",
			"make",
			"openssl",
			"patch",
			"pkgconf",
			"which",
		},
		Pacman: []string{
			"base-devel",
		},
	}
}

// GetInstallArgs returns the package manager install arguments.
func GetInstallArgs(format string) []string {
	switch format {
	case "apk":
		return []string{"add", "--allow-untrusted"}
	case "deb":
		return []string{"--allow-downgrades", "--assume-yes", "install"}
	case "rpm":
		return []string{"-y", "install"}
	case "pacman":
		return []string{"-U", "--noconfirm"}
	default:
		return []string{}
	}
}
