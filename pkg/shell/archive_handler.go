// Package shell provides process execution and shell operations.
package shell

// archiveExecHandler intercepts archive-extraction commands inside build
// scripts and replaces them with in-process implementations from pkg/archive
// and pkg/builders/common (stdlib archive/tar + archive/zip + compress/bzip2 +
// klauspost/compress + ulikunitz/xz + bodgit/sevenzip + nwaples/rardecode).
// This eliminates the need for the corresponding system binaries in build
// containers.
//
// All handled formats are dispatched in-process (no binary needed):
//   - unzip       → stdlib archive/zip
//   - jar         → stdlib archive/zip (JARs are zips)
//   - gunzip/gzip → stdlib compress/gzip (both directions)
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
//	gzip [-c] [-k] [-1..-9] [file]
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
	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
)

const (
	cmdJar    = "jar"
	cmdGunzip = "gunzip"
)

// archiveExecHandler returns an interp.ExecHandlerFunc that intercepts
// archive-extraction commands and handles them in-process.
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
			return handleGzip(ctx, args)
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

	logger.Info(i18n.T("logger.shell.info.archive_handler_unzip"),
		"archive", archivePath, "dest", destDir, "filters", filters)

	return archive.ExtractFiltered(ctx, archivePath, destDir, filters)
}

// gzipArgs holds the parsed form of a gzip/gunzip invocation.
type gzipArgs struct {
	inputPath  string
	toStdout   bool
	keepOrig   bool
	decompress bool
	level      int
}

// parseGzipArgs parses gzip/gunzip arguments. Decompression is the default for
// `gunzip`; `gzip` compresses unless -d/--decompress/--uncompress is given.
func parseGzipArgs(args []string) gzipArgs {
	opts := gzipArgs{
		level:      gzip.DefaultCompression,
		decompress: filepath.Base(args[0]) == cmdGunzip,
	}

	for _, arg := range args[1:] {
		switch {
		case arg == "-c", arg == "--stdout", arg == "--to-stdout":
			opts.toStdout = true
		case arg == "-k", arg == "--keep":
			opts.keepOrig = true
		case arg == "-d", arg == "--decompress", arg == "--uncompress":
			opts.decompress = true
		case arg == "--fast":
			opts.level = gzip.BestSpeed
		case arg == "--best":
			opts.level = gzip.BestCompression
		case arg == "-f", arg == "--force", arg == "-q", arg == "--quiet",
			arg == "-n", arg == "--no-name", arg == "-N", arg == "--name":
			// accepted for compatibility; no effect on the in-process implementation
		case strings.HasPrefix(arg, "-") && len(arg) > 1:
			opts.parseShortFlags(arg)
		default:
			if opts.inputPath == "" {
				opts.inputPath = arg
			}
		}
	}

	return opts
}

// parseShortFlags handles clustered short flags such as "-dc" or "-9".
func (o *gzipArgs) parseShortFlags(arg string) {
	for _, flag := range strings.TrimLeft(arg, "-") {
		switch flag {
		case 'c':
			o.toStdout = true
		case 'k':
			o.keepOrig = true
		case 'd':
			o.decompress = true
		case '1', '2', '3', '4', '5', '6', '7', '8', '9':
			o.level = int(flag - '0')
		}
	}
}

// handleGzip handles both compression and decompression:
//
//	gzip [-c] [-k] [-1..-9] [file]     → compress (stdin filter when no file)
//	gunzip [-c] [-d] [-k] [file]       → decompress
//
// With -c the result goes to hc.Stdout so shell redirections like
// `gzip -c page.8 > page.8.gz` work correctly.
func handleGzip(ctx context.Context, args []string) error {
	opts := parseGzipArgs(args)
	if opts.decompress {
		return gunzipPath(ctx, opts)
	}

	return gzipPath(ctx, opts)
}

// resolvePath makes a relative operand absolute against the script's cwd.
func resolvePath(dir, path string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}

	return filepath.Join(dir, path)
}

// compressStream gzips r into w at the given level.
func compressStream(dst io.Writer, src io.Reader, level int) error {
	if src == nil {
		return errors.New(errors.ErrTypeBuild, "gzip: no input file specified").
			WithOperation("handleGzip")
	}

	gw, err := gzip.NewWriterLevel(dst, level)
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeBuild, "gzip: create gzip writer").
			WithOperation("handleGzip")
	}

	if _, err = io.Copy(gw, src); err != nil { //nolint:gosec
		_ = gw.Close()

		return errors.Wrap(err, errors.ErrTypeBuild, "gzip: compress").
			WithOperation("handleGzip")
	}

	if err = gw.Close(); err != nil {
		return errors.Wrap(err, errors.ErrTypeBuild, "gzip: compress").
			WithOperation("handleGzip")
	}

	return nil
}

// gzipPath compresses a file (or stdin) according to opts.
func gzipPath(ctx context.Context, opts gzipArgs) error {
	hc := interp.HandlerCtx(ctx)

	// No operand: behave as a stdin → stdout filter, like `cat x | gzip > x.gz`.
	if opts.inputPath == "" {
		return compressStream(hc.Stdout, hc.Stdin, opts.level)
	}

	inputPath := resolvePath(hc.Dir, opts.inputPath)

	input, err := os.Open(filepath.Clean(inputPath))
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeBuild, "gzip: open input").
			WithOperation("handleGzip")
	}

	defer func() { _ = input.Close() }()

	if opts.toStdout {
		logger.Info(i18n.T("logger.shell.info.archive_handler_gzip_stdout"), "input", inputPath)

		return compressStream(hc.Stdout, input, opts.level)
	}

	outPath := inputPath + ".gz"

	logger.Info(i18n.T("logger.shell.info.archive_handler_gzip_place"),
		"input", inputPath, "output", outPath)

	outFile, err := os.Create(filepath.Clean(outPath))
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeBuild, "gzip: create output").
			WithOperation("handleGzip")
	}

	defer func() { _ = outFile.Close() }()

	if cErr := compressStream(outFile, input, opts.level); cErr != nil {
		return cErr
	}

	if err = outFile.Close(); err != nil {
		return errors.Wrap(err, errors.ErrTypeBuild, "gzip: create output").
			WithOperation("handleGzip")
	}

	if !opts.keepOrig {
		_ = os.Remove(inputPath)
	}

	return nil
}

// gunzipPath decompresses a file according to opts. With -c it writes to
// hc.Stdout; otherwise it decompresses in place (strips the .gz suffix).
func gunzipPath(ctx context.Context, opts gzipArgs) error {
	hc := interp.HandlerCtx(ctx)

	if opts.inputPath == "" {
		return errors.New(errors.ErrTypeBuild, "gunzip: no input file specified").
			WithOperation("handleGunzip")
	}

	inputPath := resolvePath(hc.Dir, opts.inputPath)

	input, err := os.Open(filepath.Clean(inputPath))
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeBuild, "gunzip: open input").
			WithOperation("handleGunzip")
	}

	defer func() { _ = input.Close() }()

	gzipReader, err := gzip.NewReader(input)
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeBuild, "gunzip: create gzip reader").
			WithOperation("handleGunzip")
	}

	defer func() { _ = gzipReader.Close() }()

	if opts.toStdout {
		logger.Info(i18n.T("logger.shell.info.archive_handler_gunzip_stdout"), "input", inputPath)

		_, err = io.Copy(hc.Stdout, gzipReader) //nolint:gosec

		return err
	}

	// In-place: strip .gz suffix and write decompressed file.
	outPath := strings.TrimSuffix(inputPath, ".gz")
	if outPath == inputPath {
		outPath = inputPath + ".out"
	}

	logger.Info(i18n.T("logger.shell.info.archive_handler_gunzip_place"), "input", inputPath, "output", outPath)

	outFile, err := os.Create(filepath.Clean(outPath))
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeBuild, "gunzip: create output").
			WithOperation("handleGunzip")
	}

	defer func() { _ = outFile.Close() }()

	if _, err = io.Copy(outFile, gzipReader); err != nil { //nolint:gosec
		return errors.Wrap(err, errors.ErrTypeBuild, "gunzip: decompress").
			WithOperation("handleGunzip")
	}

	if !opts.keepOrig {
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

	logger.Info(i18n.T("logger.shell.info.archive_handler_unrar"), "archive", archivePath, "dest", destDir)

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

	logger.Info(i18n.T("logger.shell.info.archive_handler_7z"), "archive", archivePath, "dest", destDir)

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

	logger.Info(i18n.T("logger.shell.info.archive_handler_jar"), "archive", archivePath, "dest", destDir)

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

	logger.Info(i18n.T("logger.shell.info.archive_handler_dpkg_deb"), "archive", archivePath, "dest", destDir)

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

	logger.Info(i18n.T("logger.shell.info.archive_handler_rpm2cpio"), "archive", archivePath)

	f, err := os.Open(filepath.Clean(archivePath))
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
