// Package shell provides process execution and shell operations.
package shell

// archiveExecHandler intercepts archive-extraction commands inside build
// scripts and replaces them with pure-Go implementations from pkg/archive
// and pkg/builders/common (stdlib archive/tar + archive/zip + compress/bzip2 +
// klauspost/compress + ulikunitz/xz + bodgit/sevenzip + nwaples/rardecode).
// This eliminates the need for the corresponding system binaries in build
// containers.
//
// All handled formats are pure-Go (no binary needed):
//   - unzip       → stdlib archive/zip
//   - jar         → stdlib archive/zip (JARs are zips)
//   - gunzip/gzip → stdlib compress/gzip
//   - unrar       → nwaples/rardecode/v2
//   - 7z, 7za     → bodgit/sevenzip
//   - dpkg-deb    → pkg/builders/common.ExtractDEB (ar + tar)
//   - rpm2cpio    → github.com/sassoftware/go-rpmutils (raw cpio stream)
//
// For any command not in the intercept list the handler falls through to the
// next handler (which will invoke the OS binary as usual).
//
// Supported command forms:
//
//	unzip [-o] [-q] [-d <destdir>] <archive> [files/globs...]
//	unrar x [-o+] <archive> [destdir]
//	7z x [-o<destdir>] <archive>
//	7za x [-o<destdir>] <archive>
//	jar xf <archive>
//	gunzip [-c] [-d] [-k] [file]
//	dpkg-deb -x <deb> <dir>
//	dpkg-deb --extract <deb> <dir>
//	dpkg-deb -X <deb> <dir>
//	rpm2cpio <rpm>
//
// For any command not in the intercept list the handler falls through to the
// next handler (which will invoke the OS binary as usual).
//
// Note: alien (format conversion) is not supported and will fall through to
// the next handler (which will fail if alien is not installed).

import (
	"compress/gzip"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"

	rpmutils "github.com/sassoftware/go-rpmutils"
	"mvdan.cc/sh/v3/interp"

	"github.com/M0Rf30/yap/v2/pkg/archive"
	"github.com/M0Rf30/yap/v2/pkg/errors"
	"github.com/M0Rf30/yap/v2/pkg/logger"
)

const (
	cmdJar    = "jar"
	cmdGunzip = "gunzip"
)

// archiveExecHandler returns an interp.ExecHandlerFunc that intercepts
// archive-extraction commands and handles them with pure-Go code.
func archiveExecHandler(next interp.ExecHandlerFunc) interp.ExecHandlerFunc {
	return func(ctx context.Context, args []string) error {
		if len(args) == 0 {
			return next(ctx, args)
		}

		cmd := filepath.Base(args[0])

		switch cmd {
		case "unzip":
			return handleUnzip(ctx, args)
		case "unrar":
			return handleUnrar(ctx, args)
		case "7z", "7za":
			return handle7z(ctx, args)
		case cmdJar:
			return handleJar(ctx, args)
		case cmdGunzip, "gzip":
			return handleGunzip(ctx, args)
		case "dpkg-deb":
			return handleDpkgDeb(ctx, args, next)
		case "rpm2cpio":
			return handleRpm2Cpio(ctx, args)
		default:
			return next(ctx, args)
		}
	}
}

// handleUnzip handles: unzip [-o] [-q] [-d <destdir>] <archive> [files/globs...]
// File/glob filters after the archive path are honoured via ExtractFiltered.
func handleUnzip(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)
	destDir := hc.Dir // default: script's working directory
	archivePath := ""

	var filters []string

	for i := 1; i < len(args); i++ {
		arg := args[i]

		switch {
		case arg == "-d" && i+1 < len(args):
			i++
			destDir = args[i]
		case strings.HasPrefix(arg, "-d"):
			destDir = arg[2:]
		case arg == "-o", arg == "-q", arg == "-n", arg == "-j", arg == "-a",
			arg == "-aa", arg == "-p", arg == "-v", arg == "-l", arg == "-t":
			// known flags — skip
		case strings.HasPrefix(arg, "-"):
			// unknown flag — skip
		default:
			if archivePath == "" {
				archivePath = arg
			} else {
				filters = append(filters, arg)
			}
		}
	}

	if archivePath == "" {
		return errors.New(errors.ErrTypeBuild, "unzip: no archive specified").
			WithOperation("handleUnzip")
	}

	// Resolve relative paths against the script's working directory.
	if !filepath.IsAbs(archivePath) {
		archivePath = filepath.Join(hc.Dir, archivePath)
	}

	if !filepath.IsAbs(destDir) {
		destDir = filepath.Join(hc.Dir, destDir)
	}

	logger.Info("archive handler: unzip", "archive", archivePath, "dest", destDir, "filters", filters)

	return archive.ExtractFiltered(ctx, archivePath, destDir, filters)
}

// parseGunzipArgs parses gunzip/gzip arguments and returns the input path,
// whether to write to stdout, and whether to keep the original file.
func parseGunzipArgs(args []string) (inputPath string, toStdout, keepOrig bool) {
	for i := 1; i < len(args); i++ {
		arg := args[i]

		switch {
		case arg == "-c", arg == "--stdout", arg == "--to-stdout":
			toStdout = true
		case arg == "-k", arg == "--keep":
			keepOrig = true
		case arg == "-d", arg == "--decompress":
			// decompress mode (default for gunzip) — no-op
		case strings.HasPrefix(arg, "-"):
			// combined short flags like "-dc"
			stripped := strings.TrimLeft(arg, "-")
			if strings.ContainsRune(stripped, 'c') {
				toStdout = true
			}

			if strings.ContainsRune(stripped, 'k') {
				keepOrig = true
			}
		default:
			if inputPath == "" {
				inputPath = arg
			}
		}
	}

	return inputPath, toStdout, keepOrig
}

// handleGunzip handles: gunzip [-c] [-d] [-k] [file]
// With -c (stdout mode) it decompresses to hc.Stdout so shell redirections
// like `gunzip -dc file.gz > out.txt` work correctly.
// Without -c it decompresses in-place (removes .gz, writes plain file).
func handleGunzip(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)

	inputPath, toStdout, keepOrig := parseGunzipArgs(args)

	if inputPath == "" {
		return errors.New(errors.ErrTypeBuild, "gunzip: no input file specified").
			WithOperation("handleGunzip")
	}

	if !filepath.IsAbs(inputPath) {
		inputPath = filepath.Join(hc.Dir, inputPath)
	}

	f, err := os.Open(filepath.Clean(inputPath)) // #nosec G304 -- user-supplied build script path
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeBuild, "gunzip: open input").
			WithOperation("handleGunzip")
	}

	defer func() { _ = f.Close() }()

	gr, err := gzip.NewReader(f)
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeBuild, "gunzip: create gzip reader").
			WithOperation("handleGunzip")
	}

	defer func() { _ = gr.Close() }()

	if toStdout {
		logger.Info("archive handler: gunzip → stdout", "input", inputPath)
		// #nosec G110 -- decompression of user-supplied build sources
		_, err = io.Copy(hc.Stdout, gr)

		return err
	}

	// In-place: strip .gz suffix and write decompressed file.
	outPath := strings.TrimSuffix(inputPath, ".gz")
	if outPath == inputPath {
		outPath = inputPath + ".out"
	}

	logger.Info("archive handler: gunzip in-place", "input", inputPath, "output", outPath)

	outFile, err := os.Create(filepath.Clean(outPath)) // #nosec G304
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeBuild, "gunzip: create output").
			WithOperation("handleGunzip")
	}

	defer func() { _ = outFile.Close() }()

	if _, err = io.Copy(outFile, gr); err != nil { // #nosec G110 -- decompression of build sources
		return errors.Wrap(err, errors.ErrTypeBuild, "gunzip: decompress").
			WithOperation("handleGunzip")
	}

	if !keepOrig {
		_ = os.Remove(inputPath)
	}

	return nil
}

// handleUnrar handles: unrar x [-o+] <archive> [destdir]
func handleUnrar(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)

	// args[1] should be the sub-command; we only handle "x" (extract with full paths)
	if len(args) < 3 || (args[1] != "x" && args[1] != "e") {
		return errors.New(errors.ErrTypeBuild, "unrar: unsupported sub-command or missing archive").
			WithOperation("handleUnrar")
	}

	archivePath := ""
	destDir := hc.Dir

	for i := 2; i < len(args); i++ {
		arg := args[i]

		switch {
		case strings.HasPrefix(arg, "-"):
			// flags like -o+ — skip
		default:
			if archivePath == "" {
				archivePath = arg
			} else {
				destDir = arg
			}
		}
	}

	if archivePath == "" {
		return errors.New(errors.ErrTypeBuild, "unrar: no archive specified").
			WithOperation("handleUnrar")
	}

	// Resolve relative paths against the script's working directory.
	if !filepath.IsAbs(archivePath) {
		archivePath = filepath.Join(hc.Dir, archivePath)
	}

	if !filepath.IsAbs(destDir) {
		destDir = filepath.Join(hc.Dir, destDir)
	}

	logger.Info("archive handler: unrar", "archive", archivePath, "dest", destDir)

	return archive.Extract(ctx, archivePath, destDir)
}

// handle7z handles: 7z x [-o<destdir>] <archive>
// (also 7za — same syntax)
func handle7z(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)

	// args[1] should be the sub-command; we only handle "x" or "e"
	if len(args) < 3 || (args[1] != "x" && args[1] != "e") {
		return errors.New(errors.ErrTypeBuild, "7z: unsupported sub-command or missing archive").
			WithOperation("handle7z")
	}

	destDir := hc.Dir
	archivePath := ""

	for i := 2; i < len(args); i++ {
		arg := args[i]

		switch {
		case strings.HasPrefix(arg, "-o"):
			destDir = arg[2:]
		case strings.HasPrefix(arg, "-"):
			// other flags — skip
		default:
			if archivePath == "" {
				archivePath = arg
			}
		}
	}

	if archivePath == "" {
		return errors.New(errors.ErrTypeBuild, "7z: no archive specified").
			WithOperation("handle7z")
	}

	// Resolve relative paths against the script's working directory.
	if !filepath.IsAbs(archivePath) {
		archivePath = filepath.Join(hc.Dir, archivePath)
	}

	if !filepath.IsAbs(destDir) {
		destDir = filepath.Join(hc.Dir, destDir)
	}

	logger.Info("archive handler: 7z", "archive", archivePath, "dest", destDir)

	return archive.Extract(ctx, archivePath, destDir)
}

// parseJarArgs parses jar command arguments and returns the archive path,
// destination directory, and whether a file argument is expected next.
// It handles combined flag strings like "xf" or "-xf" and long options.
func parseJarArgs(args []string, defaultDir string) (archivePath, destDir string) {
	destDir = defaultDir
	wantFile := false

	for i := 1; i < len(args); i++ {
		arg := args[i]

		switch {
		case arg == "-C" && i+1 < len(args):
			i++
			destDir = args[i]
		case strings.HasPrefix(arg, "--file="):
			archivePath = strings.TrimPrefix(arg, "--file=")
		case strings.HasPrefix(arg, "--"):
			// long flags like --extract — skip
		case strings.HasPrefix(arg, "-") || (i == 1 && !strings.HasPrefix(arg, "/")):
			// Combined flag string like "xf" or "-xf": if 'f' is present the
			// next positional argument is the archive filename.
			stripped := strings.TrimLeft(arg, "-")
			if strings.ContainsRune(stripped, 'f') {
				wantFile = true
			}
		default:
			if wantFile && archivePath == "" {
				archivePath = arg
				wantFile = false
			}
			// remaining positional args are file filters — ignored
		}
	}

	return archivePath, destDir
}

// handleJar handles: jar xf <archive> [files...]
// JAR files are ZIP archives, handled via stdlib archive/zip.
//
// jar uses a non-standard flag syntax where the first argument is a string of
// mode/option characters (e.g. "xf", "-xf", "--extract").  We only support
// extract mode ("x" present in flags) and resolve the archive path from the
// remaining positional arguments or the "--file=" long option.
func handleJar(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)

	archivePath, destDir := parseJarArgs(args, hc.Dir)

	if archivePath == "" {
		return errors.New(errors.ErrTypeBuild, "jar: no archive specified").
			WithOperation("handleJar")
	}

	// Resolve relative paths against the script's working directory.
	if !filepath.IsAbs(archivePath) {
		archivePath = filepath.Join(hc.Dir, archivePath)
	}

	if !filepath.IsAbs(destDir) {
		destDir = filepath.Join(hc.Dir, destDir)
	}

	logger.Info("archive handler: jar", "archive", archivePath, "dest", destDir)

	return archive.Extract(ctx, archivePath, destDir)
}

// handleDpkgDeb handles: dpkg-deb -x <deb> <dir>
// Falls through to next handler for unsupported sub-commands (e.g. -c, --info).
func handleDpkgDeb(ctx context.Context, args []string, next interp.ExecHandlerFunc) error {
	hc := interp.HandlerCtx(ctx)

	// Find the mode flag (-x, --extract, -X)
	extract := false

	var positional []string

	for i := 1; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "-x", "--extract", "-X":
			extract = true
		case "--vextract":
			extract = true
		default:
			if strings.HasPrefix(arg, "-") {
				// unsupported flag — fall through
				return next(ctx, args)
			}

			positional = append(positional, arg)
		}
	}

	if !extract || len(positional) < 2 {
		// Not an extract command, or missing args — let the binary handle it
		return next(ctx, args)
	}

	archivePath := positional[0]
	destDir := positional[1]

	if !filepath.IsAbs(archivePath) {
		archivePath = filepath.Join(hc.Dir, archivePath)
	}

	if !filepath.IsAbs(destDir) {
		destDir = filepath.Join(hc.Dir, destDir)
	}

	logger.Info("archive handler: dpkg-deb -x", "archive", archivePath, "dest", destDir)

	return archive.ExtractDEB(archivePath, destDir)
}

// handleRpm2Cpio handles: rpm2cpio <rpm>
// Writes the decompressed cpio payload to stdout (matching rpm2cpio's behaviour).
func handleRpm2Cpio(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)

	if len(args) < 2 {
		return errors.New(errors.ErrTypeBuild, "rpm2cpio: no rpm file specified").
			WithOperation("handleRpm2Cpio")
	}

	archivePath := args[1]
	if !filepath.IsAbs(archivePath) {
		archivePath = filepath.Join(hc.Dir, archivePath)
	}

	logger.Info("archive handler: rpm2cpio", "archive", archivePath)

	f, err := os.Open(filepath.Clean(archivePath)) // #nosec G304 -- user-supplied build script path
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeBuild, "rpm2cpio: open").
			WithOperation("handleRpm2Cpio")
	}
	defer func() { _ = f.Close() }()

	rpm, err := rpmutils.ReadRpm(f)
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeBuild, "rpm2cpio: read rpm header").
			WithOperation("handleRpm2Cpio")
	}

	// Get the raw decompressed cpio payload stream.
	payload, err := rpm.PayloadReader()
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeBuild, "rpm2cpio: open payload").
			WithOperation("handleRpm2Cpio")
	}

	_, err = io.Copy(hc.Stdout, payload)

	return err
}
