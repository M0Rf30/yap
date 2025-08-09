// Package rpm provides RPM package building functionality and constants.
package rpm

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
)

var buildEnvironmentDeps = []string{
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
}

var installArgs = []string{
	"-y",
	"install",
}

var (
	// RPMArchs maps standard architecture names to RPM package architecture names.
	RPMArchs = map[string]string{
		"x86_64":  "x86_64",
		"i686":    "i686",
		"aarch64": "aarch64",
		"armv7h":  "armv7h",
		"armv6h":  "armv6h",
		"arm":     "arm",
		"any":     "noarch",
	}

	// RPMGroups maps common group names to RPM group categories.
	RPMGroups = map[string]string{
		"admin":        "Applications/System",
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
		"graphics":     Multimedia,
		"httpd":        Internet,
		"interpreters": Tools,
		"kernel":       "System Environment/Kernel",
		"libdevel":     "Development/Libraries",
		"libs":         "System Environment/Libraries",
		"localization": "Development/Languages",
		"mail":         Communications,
		"math":         "Applications/Productivity",
		"misc":         "Applications/System",
		"net":          Internet,
		"news":         "Applications/Publishing",
		"science":      Engineering,
		"shells":       "System Environment/Shells",
		"sound":        Multimedia,
		"text":         "Applications/Text",
		"vcs":          Tools,
		"video":        Multimedia,
		"web":          Internet,
		"x11":          "User Interface/X",
	}

	// RPMDistros maps distribution names to their RPM suffix.
	RPMDistros = map[string]string{
		"almalinux": ".el",
		"amzn":      ".amzn",
		"fedora":    ".fc",
		"ol":        ".ol",
		"rhel":      ".el",
		"rocky":     ".el",
	}
)
