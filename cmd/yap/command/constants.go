package command

// Command group IDs and titles
const (
	buildGroup           = "build"
	utilityCommandsGroup = "Utility Commands"
)

// Other command names
const (
	commandYap         = "yap"
	commandEnvironment = "environment"
	commandUtility     = "utility"
	commandInstall     = "install"
	commandListDistro  = "list-distros"
	commandPull        = "pull"
	commandStatus      = "status"
	commandVersion     = "version"
	commandZap         = "zap"
	commandHelp        = "help"
)

// Distro family names
const (
	distroFamilyDebian  = "debian"
	distroFamilyRedhat  = "redhat"
	distroFamilyUnknown = "unknown"
)

// Aliases and other command-related strings
const (
	aliasPrep = "prep"
	aliasHelp = "help"
)

// Package manager commands
const (
	pmApk = "apk"
)

// Flag names used across multiple commands
const (
	flagSkipSync = "skip-sync"
	flagFrom     = "from"
	flagSkip     = "skip"
)
