package shell

// ParseGunzipArgsForTesting exposes parseGunzipArgs for unit tests.
func ParseGunzipArgsForTesting(args []string) (inputPath string, toStdout, keepOrig bool) {
	return parseGunzipArgs(args)
}

// ParseJarArgsForTesting exposes parseJarArgs for unit tests.
func ParseJarArgsForTesting(args []string, defaultDir string) (archivePath, destDir string) {
	return parseJarArgs(args, defaultDir)
}

// ExtractErrorLinesForTesting exposes extractErrorLines for unit tests.
func ExtractErrorLinesForTesting(raw, fallback string) string {
	return extractErrorLines(raw, fallback)
}

// NormalizeScriptContentForTesting exposes normalizeScriptContent for unit tests.
func NormalizeScriptContentForTesting(script string) string {
	return normalizeScriptContent(script)
}

// LogScriptResultForTesting exposes logScriptResult for unit tests.
// Re-exported as a package-level function so tests in the same package can call it.
var LogScriptResultForTesting = logScriptResult
