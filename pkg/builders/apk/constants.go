package apk

var (
	// APKArchs maps Go architecture names to APK architecture names.
	APKArchs = map[string]string{
		"x86_64":  "x86_64",
		"i686":    "x86",
		"aarch64": "aarch64",
		"armv7h":  "armv7h",
		"armv6h":  "armv6h",
		"any":     "all",
	}
)
