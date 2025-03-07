package rpm

const (
	Communications = "Applications/Communications"
	Engineering    = "Applications/Engineering"
	Internet       = "Applications/Internet"
	Multimedia     = "Applications/Multimedia"
	Tools          = "Development/Tools"
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

var (
	RPMArchs = map[string]string{
		"x86_64":  "x86_64",
		"i686":    "i686",
		"aarch64": "aarch64",
		"armv7h":  "armv7h",
		"armv6h":  "armv6h",
		"arm":     "arm",
		"any":     "noarch",
	}

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

	RPMDistros = map[string]string{
		"almalinux": ".el",
		"amzn":      ".amzn",
		"fedora":    ".fc",
		"ol":        ".ol",
		"rhel":      ".el",
		"rocky":     ".el",
	}
)
