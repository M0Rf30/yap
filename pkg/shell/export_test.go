package shell

// ParseGunzipArgsForTesting exposes parseGunzipArgs for unit tests.
func ParseGunzipArgsForTesting(args []string) (inputPath string, toStdout, keepOrig bool) {
	return parseGunzipArgs(args)
}

// ParseJarArgsForTesting exposes parseJarArgs for unit tests.
func ParseJarArgsForTesting(args []string, defaultDir string) (archivePath, destDir string) {
	return parseJarArgs(args, defaultDir)
}
