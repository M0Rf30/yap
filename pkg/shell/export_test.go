package shell

// ParseGzipArgsForTesting exposes parseGzipArgs for unit tests.
func ParseGzipArgsForTesting(args []string) (inputPath string, toStdout, keepOrig, decompress bool, level int) {
	opts := parseGzipArgs(args)

	return opts.inputPath, opts.toStdout, opts.keepOrig, opts.decompress, opts.level
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
