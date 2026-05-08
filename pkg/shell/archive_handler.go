// Package shell provides process execution and shell operations.
package shell

// archiveExecHandler intercepts archive-extraction commands inside build scripts
// (unzip, unrar, 7z, 7za, jar) and replaces them with pure-Go implementations
// backed by mholt/archives.  This eliminates the need for those binaries inside
// build containers and makes extraction deterministic and cross-platform.
//
// Supported command forms:
//
//	unzip [-o] [-q] [-d <destdir>] <archive> [files...]
//	unrar x [-o+] <archive> [destdir]
//	7z x [-o<destdir>] <archive>
//	7za x [-o<destdir>] <archive>
//	jar xf <archive>
//
// For any command not in the intercept list the handler falls through to the
// next handler (which will invoke the OS binary as usual).

import (
	"context"
	"path/filepath"
	"strings"

	"mvdan.cc/sh/v3/interp"

	"github.com/M0Rf30/yap/v2/pkg/archive"
	"github.com/M0Rf30/yap/v2/pkg/errors"
	"github.com/M0Rf30/yap/v2/pkg/logger"
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
		case "jar":
			return handleJar(ctx, args)
		default:
			return next(ctx, args)
		}
	}
}

// handleUnzip handles: unzip [-o] [-q] [-d <destdir>] <archive> [files...]
// We ignore file-list filtering (extract everything) and honour -d.
func handleUnzip(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)
	destDir := hc.Dir // default: script's working directory
	archivePath := ""

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
			}
			// remaining positional args are file filters — ignored (extract all)
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

	logger.Info("archive handler: unzip", "archive", archivePath, "dest", destDir)

	return archive.Extract(ctx, archivePath, destDir)
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
// JAR files are ZIP archives — mholt/archives handles them transparently.
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
