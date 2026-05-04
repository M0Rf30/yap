// Package rpm provides RPM package building functionality and constants.
package rpm

import "github.com/M0Rf30/yap/v2/pkg/constants"

const (
	// Communications represents the applications/communications RPM group.
	Communications = "Applications/Communications"
	// Engineering represents the applications/engineering RPM group.
	Engineering = "Applications/Engineering"
	// Internet represents the applications/internet RPM group.
	Internet = "Applications/Internet"
	// Multimedia represents the applications/multimedia RPM group.
	Multimedia = "Applications/Multimedia"
	// Tools represents the development/tools RPM group.
	Tools = "Development/Tools"
	// applicationsSystem is the RPM group for system applications.
	applicationsSystem = "Applications/System"
	// rpmSuffixEL is the RPM dist tag suffix for Enterprise Linux distros.
	rpmSuffixEL = ".el"
	// groupGraphics is the common group name for graphics-related packages.
	groupGraphics = "graphics"
	// groupMisc is the common group name for miscellaneous packages.
	groupMisc = "misc"
	// groupNet is the common group name for networking-related packages.
	groupNet = "net"
	// groupWeb is the common group name for web-related packages.
	groupWeb = "web"
)

var (
	// RPMGroups maps common group names to RPM group categories.
	// This is RPM-specific logic that should remain here.
	RPMGroups = map[string]string{
		"admin":        applicationsSystem,
		"any":          "noarch",
		"comm":         Communications,
		"database":     "Applications/Databases",
		"debug":        "Development/Debuggers",
		"devel":        Tools,
		"doc":          "Documentation",
		"editors":      "Applications/Editors",
		"electronics":  Engineering,
		"embedded":     Engineering,
		"fonts":        "Interface/Desktops",
		"games":        "Amusements/Games",
		groupGraphics:  Multimedia,
		"httpd":        Internet,
		"interpreters": Tools,
		"kernel":       "System Environment/Kernel",
		"libdevel":     "Development/Libraries",
		"libs":         "System Environment/Libraries",
		"localization": "Development/Languages",
		"mail":         Communications,
		"math":         "Applications/Productivity",
		groupMisc:      applicationsSystem,
		groupNet:       Internet,
		"news":         "Applications/Publishing",
		"science":      Engineering,
		"shells":       "System Environment/Shells",
		"sound":        Multimedia,
		"text":         "Applications/Text",
		"vcs":          Tools,
		"video":        Multimedia,
		groupWeb:       Internet,
		"x11":          "User Interface/X",
	}

	// RPMDistros maps distribution names to their RPM suffix.
	// This is RPM-specific logic that should remain here.
	RPMDistros = map[string]string{
		constants.DistroAlmalinux: rpmSuffixEL,
		constants.DistroAmzn:      ".amzn",
		constants.DistroFedora:    ".fc",
		constants.DistroOl:        ".ol",
		constants.DistroRhel:      rpmSuffixEL,
		constants.DistroRocky:     rpmSuffixEL,
	}
)
