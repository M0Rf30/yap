// Package rpm provides RPM package building functionality and constants.
package rpm

import "github.com/M0Rf30/yap/v2/pkg/constants"

const (
	// communications represents the applications/communications RPM group.
	communications = "Applications/Communications"
	// engineering represents the applications/engineering RPM group.
	engineering = "Applications/Engineering"
	// internet represents the applications/internet RPM group.
	internet = "Applications/Internet"
	// multimedia represents the applications/multimedia RPM group.
	multimedia = "Applications/Multimedia"
	// tools represents the development/tools RPM group.
	tools = "Development/Tools"
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
		"comm":         communications,
		"database":     "Applications/Databases",
		"debug":        "Development/Debuggers",
		"devel":        tools,
		"doc":          "Documentation",
		"editors":      "Applications/Editors",
		"electronics":  engineering,
		"embedded":     engineering,
		"fonts":        "Interface/Desktops",
		"games":        "Amusements/Games",
		groupGraphics:  multimedia,
		"httpd":        internet,
		"interpreters": tools,
		"kernel":       "System Environment/Kernel",
		"libdevel":     "Development/Libraries",
		"libs":         "System Environment/Libraries",
		"localization": "Development/Languages",
		"mail":         communications,
		"math":         "Applications/Productivity",
		groupMisc:      applicationsSystem,
		groupNet:       internet,
		"news":         "Applications/Publishing",
		"science":      engineering,
		"shells":       "System Environment/Shells",
		"sound":        multimedia,
		"text":         "Applications/Text",
		"vcs":          tools,
		"video":        multimedia,
		groupWeb:       internet,
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
