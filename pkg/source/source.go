// Package source provides source file download and management functionality.
package source

import (
	"context"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	stderrors "errors"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/go-git/go-git/v5/plumbing"
	"golang.org/x/crypto/blake2b"
	"golang.org/x/sync/singleflight"

	"github.com/M0Rf30/yap/v2/pkg/archive"
	"github.com/M0Rf30/yap/v2/pkg/constants"
	"github.com/M0Rf30/yap/v2/pkg/download"
	"github.com/M0Rf30/yap/v2/pkg/errors"
	"github.com/M0Rf30/yap/v2/pkg/files"
	"github.com/M0Rf30/yap/v2/pkg/git"
	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
	"github.com/M0Rf30/yap/v2/pkg/shell"
)

const (
	fileProtocol = "file"
	branchKey    = "branch"
	commitKey    = "commit"
	tagKey       = "tag"
	skipValue    = "SKIP"
)

// partSuffix marks in-progress downloads; the final path only appears once a
// download has fully completed.
const partSuffix = ".part"

// Global variables for source handling
var (
	// sshPassword contains the SSH password for authentication; guarded by
	// sshPasswordMu.
	sshPassword   string
	sshPasswordMu sync.RWMutex
	// downloadGroup deduplicates concurrent downloads of the same file.
	downloadGroup singleflight.Group
)

// SetSSHPassword sets the SSH password used for authenticated git clone operations.
func SetSSHPassword(password string) {
	sshPasswordMu.Lock()
	defer sshPasswordMu.Unlock()

	sshPassword = password
}

// GetSSHPassword returns the SSH password used for authenticated git clone operations.
func GetSSHPassword() string {
	sshPasswordMu.RLock()
	defer sshPasswordMu.RUnlock()

	return sshPassword
}

// Source defines all the fields accepted by a source item.
type Source struct {
	// Hash is the integrity hashsum for a source item
	Hash string
	// PkgName is the package name for component logging
	PkgName string
	// RefKey is the reference name for a VCS fragment (branch, tag) declared in the
	// URI. i.e: "myfile::git+https://example.com/example.git#branch=example"
	RefKey string
	// RefValue is the reference value for a VCS fragment declared in the URI. i.e:
	// myfile::git+https://example.com/example.git#branch=refvalue
	RefValue string
	// SourceItemPath is the absolute path to a source item (folder or file)
	SourceItemPath string
	// SourceItemURI it the full source item URI. i.e:
	// "myfile::git+https://example.com/example.git#branch=example" i.e:
	// "https://example.com/example.tar.gz"
	SourceItemURI string
	// SrcDir is the directory where all the source items are symlinked, extracted
	// and processed by packaging functions.
	SrcDir string
	// StartDir is the root where a copied PKGBUILD lives and all the source items
	// are downloaded. It generally contains the src and pkg folders.
	StartDir string
	// NoExtract is the list of filenames that should NOT be extracted from
	// their archives, matching makepkg's noextract semantics: exact basename
	// match, file is still symlinked into $srcdir but archive.Extract is skipped.
	NoExtract []string
	// SkipHashCheck disables sha256/sha512 integrity verification for this
	// source item. Equivalent to setting the checksum to SKIP in the PKGBUILD.
	SkipHashCheck bool
}

// Get retrieves the source file from the specified URI.
//
// It is GetContext with context.Background().
func (src *Source) Get() error {
	return src.GetContext(context.Background())
}

// GetContext retrieves the source file from the specified URI, honouring
// cancellation of ctx while cloning or downloading.
//
// It parses the URI and determines the source file path and type.
// If the source file does not exist, it retrieves it from the specified URI.
// It validates the source file and symlinks any additional source files.
// Finally, it extracts the source file if necessary.
//
// Returns an error if any step fails, including ctx.Err() (wrapped) when the
// context is cancelled before or during the fetch.
func (src *Source) GetContext(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return cancelledError(err, src.SourceItemURI)
	}

	src.parseURI()
	sourceFilePath := filepath.Join(src.StartDir, src.SourceItemPath)
	sourceType := src.getProtocol()

	switch sourceType {
	case "http", "https", "ftp", constants.Git:
		// For git sources, if the path contains a bare/mirror repo (e.g. from
		// a CI stash or makepkg-style cache), create a working copy from it
		// matching makepkg's extract_git() behavior. If extraction fails
		// (e.g. stale cache missing the requested commit), remove the bare
		// repo and fall through to a fresh clone from the remote URL.
		if sourceType == constants.Git && files.Exists(sourceFilePath) && git.IsBareRepo(sourceFilePath) {
			if err := git.ExtractFromBare(sourceFilePath, src.SrcDir, src.RefKey, src.RefValue); err == nil {
				return nil
			}

			logger.Warn(i18n.T("logger.source.warn.bare_repo_extraction_failed"), "path", sourceFilePath)

			_ = os.RemoveAll(sourceFilePath)
			// Also remove the failed working copy attempt
			_ = os.RemoveAll(filepath.Join(src.SrcDir, filepath.Base(sourceFilePath)))
		}

		if !files.Exists(sourceFilePath) {
			if err := src.fetch(ctx, sourceType, sourceFilePath); err != nil {
				return err
			}
		}
	case fileProtocol:
	default:
		return errors.New(errors.ErrTypeValidation, i18n.T("errors.source.unsupported_source_type")).
			WithOperation("Get").
			WithContext("source_uri", src.SourceItemURI)
	}

	err := src.validateSource(sourceFilePath)
	if err != nil {
		return err
	}

	err = src.symlinkSources(sourceFilePath)
	if err != nil {
		return err
	}

	if !src.shouldSkipExtract() {
		if err := extractIfArchive(sourceFilePath, src.SrcDir); err != nil {
			return err
		}
	}

	return nil
}

// fetch downloads or clones the source at sourceFilePath. Concurrent fetches
// of the same path are deduplicated with singleflight (the work runs under
// the first caller's context); every caller stops waiting when its own ctx is
// cancelled.
func (src *Source) fetch(ctx context.Context, sourceType, sourceFilePath string) error {
	ch := downloadGroup.DoChan(sourceFilePath, func() (any, error) {
		// Double-check after acquiring the group slot
		if files.Exists(sourceFilePath) {
			//nolint:nilnil // Returning (nil, nil) is valid when file exists
			return nil, nil
		}

		return nil, src.getURL(ctx, sourceType, sourceFilePath)
	})

	select {
	case res := <-ch:
		return res.Err
	case <-ctx.Done():
		return cancelledError(ctx.Err(), src.SourceItemURI)
	}
}

// cancelledError wraps a context error raised while fetching a source.
func cancelledError(err error, uri string) error {
	return errors.Wrap(err, errors.ErrTypeNetwork, i18n.T("errors.download.download_failed")).
		WithOperation("GetContext").
		WithContext("source_uri", uri)
}

// getReferenceType returns the reference type for the given source.
//
// It takes no parameters.
// It returns a plumbing.ReferenceName.
func (src *Source) getReferenceType() plumbing.ReferenceName {
	switch src.RefKey {
	case branchKey:
		return plumbing.NewBranchReferenceName(src.RefValue)
	case tagKey:
		return plumbing.NewTagReferenceName(src.RefValue)
	}

	return ""
}

// getProtocol returns the protocol of the source item URI.
func (src *Source) getProtocol() string {
	if !strings.Contains(src.SourceItemURI, "://") {
		return fileProtocol
	}

	switch {
	case strings.HasPrefix(src.SourceItemURI, "http://"),
		strings.HasPrefix(src.SourceItemURI, "https://"),
		strings.HasPrefix(src.SourceItemURI, "ftp://"):
		return strings.Split(src.SourceItemURI, "://")[0]
	case strings.HasPrefix(src.SourceItemURI, constants.Git+"+https://"):
		return constants.Git
	default:
		return ""
	}
}

// getURL retrieves a URL based on the provided protocol and download file
// path. HTTP(S)/FTP downloads are written to "<dloadFilePath>.part" and only
// renamed into place once complete, so an interrupted or failed download is
// never mistaken for a finished source on the next run.
//
// Parameters:
// - ctx: cancels the in-flight clone or download.
// - protocol: a string representing the protocol for the URL.
// - dloadFilePath: a string representing the file path for the downloaded file.
func (src *Source) getURL(ctx context.Context, protocol, dloadFilePath string) error {
	normalizedURI := strings.TrimPrefix(src.SourceItemURI, constants.Git+"+")

	switch protocol {
	case constants.Git:
		referenceName := src.getReferenceType()

		commitHash := ""
		if src.RefKey == commitKey {
			commitHash = src.RefValue
		}

		return git.CloneContext(ctx, dloadFilePath, normalizedURI, GetSSHPassword(),
			referenceName, commitHash)
	default:
		// Use enhanced download with resume capability and the configured
		// retry budget, with context information
		_, err := shell.MultiPrinter.Start()
		if err != nil {
			return err
		}

		partPath := dloadFilePath + partSuffix

		err = download.WithContext(
			ctx,
			partPath,
			normalizedURI,
			download.MaxRetries(),
			src.PkgName,
			src.SourceItemPath,
			shell.MultiPrinter.Writer)
		if err != nil {
			return err
		}

		return os.Rename(partPath, dloadFilePath)
	}
}

// parseURI parses the URI of the Source and updates the SourceItemPath,
// SourceItemURI, RefKey, and RefValue fields accordingly.
//
// No parameters.
// No return types.
func (src *Source) parseURI() {
	src.SourceItemPath = filepath.Base(src.SourceItemURI)

	if before, after, found := strings.Cut(src.SourceItemURI, "::"); found {
		src.SourceItemPath = before
		src.SourceItemURI = after
	}

	if base, fragment, found := strings.Cut(src.SourceItemURI, "#"); found {
		originalURI := src.SourceItemURI
		src.SourceItemURI = base

		// A fragment that is not key=value (e.g. a plain "#anchor") carries no
		// VCS reference; it is stripped and ignored.
		if key, value, ok := strings.Cut(fragment, "="); ok {
			src.RefKey = key
			src.RefValue = value
		}

		// Update SourceItemPath to remove the fragment only if no custom name was used
		if src.SourceItemPath == filepath.Base(originalURI) {
			src.SourceItemPath = filepath.Base(src.SourceItemURI)
		}
	}
}

// symlinkSources creates a symbolic link from symlinkSource to symLinkTarget.
//
// It returns an error if the symlink creation fails.
func (src *Source) symlinkSources(symlinkSource string) error {
	symlinkTarget := filepath.Join(src.SrcDir, src.SourceItemPath)

	// Check if target already exists
	if linkTarget, err := os.Readlink(symlinkTarget); err == nil {
		// It's a symlink: check if it already points to the right place
		if linkTarget == symlinkSource {
			return nil // Already correctly linked
		}
		// Remove existing incorrect symlink
		if err := os.Remove(symlinkTarget); err != nil {
			return err
		}
	} else if _, err := os.Lstat(symlinkTarget); err == nil {
		// It's a regular file or directory — remove it so we can symlink
		if err := os.Remove(symlinkTarget); err != nil {
			return err
		}
	}

	return os.Symlink(symlinkSource, symlinkTarget)
}

// validateSource checks the integrity of the source files.
//
// It takes the source file path as a parameter and returns an error if any.
func (src *Source) validateSource(sourceFilePath string) error {
	info, err := os.Stat(sourceFilePath)
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem,
			i18n.T("errors.source.failed_to_open_file_for_hash")).
			WithOperation("validateSource").
			WithContext("path", sourceFilePath)
	}

	// If it's a directory, handle based on protocol (like makepkg)
	if info.IsDir() {
		protocol := src.getProtocol()
		// For VCS protocols (git), directories are expected (cloned repos)
		if protocol == constants.Git {
			logger.Info(i18n.T("logger.skip_integrity_check_for"),
				"source", src.SourceItemURI)

			return nil
		}
		// For non-VCS, non-file protocols, directories are not supported (like makepkg)
		// The file protocol (local files) also shouldn't accept directories in source arrays

		return errors.New(errors.ErrTypeValidation, i18n.T("errors.source.directory_not_supported")).
			WithOperation("validateSource").
			WithContext("path", sourceFilePath)
	}

	if src.Hash == skipValue || src.SkipHashCheck {
		logger.Info(i18n.T("logger.skip_integrity_check_for"),
			"source", src.SourceItemURI)

		return nil
	}

	candidates, err := hashCandidates(len(src.Hash))
	if err != nil {
		return err
	}

	file, err := files.Open(filepath.Clean(sourceFilePath))
	if err != nil {
		return err
	}

	defer func() {
		err := file.Close()
		if err != nil {
			logger.Warn(i18n.T("logger.source.warn.failed_close_source_file"), "path", sourceFilePath,
				"error", err)
		}
	}()

	writers := make([]io.Writer, len(candidates))
	for i, cand := range candidates {
		writers[i] = cand
	}

	_, err = io.Copy(io.MultiWriter(writers...), file)
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem, i18n.T("errors.source.failed_to_copy_file")).
			WithOperation("validateSource").
			WithContext("path", sourceFilePath)
	}

	// The digest algorithm is not carried with the checksum, so it is inferred
	// from the digest length. A 128-hex digest is ambiguous (sha512sums or
	// b2sums), so every candidate of that length is tried. Hex comparison is
	// case-insensitive.
	hexSum := hex.EncodeToString(candidates[0].Sum(nil))

	for _, cand := range candidates {
		if strings.EqualFold(hex.EncodeToString(cand.Sum(nil)), src.Hash) {
			logger.Info(i18n.T("logger.integrity_check_for"),
				"source", src.SourceItemURI)

			return nil
		}
	}

	return errors.New(errors.ErrTypeValidation, i18n.T("errors.source.hash_verification_failed")).
		WithOperation("validateSource").
		WithContext("source_path", src.SourceItemPath).
		WithContext("expected_hash", src.Hash).
		WithContext("actual_hash", hexSum)
}

// hashCandidates returns the digest algorithms whose hex output has the given
// length: sha224 (56), sha256 (64), sha384 (96), sha512 or blake2b-512 (128).
func hashCandidates(hexLen int) ([]hash.Hash, error) {
	switch hexLen {
	case 56:
		return []hash.Hash{sha256.New224()}, nil
	case 64:
		return []hash.Hash{sha256.New()}, nil
	case 96:
		return []hash.Hash{sha512.New384()}, nil
	case 128:
		b2, err := blake2b.New512(nil)
		if err != nil {
			return nil, err
		}

		return []hash.Hash{sha512.New(), b2}, nil
	default:
		return nil, errors.New(errors.ErrTypeValidation,
			fmt.Sprintf(i18n.T("errors.source.unsupported_hash_length"), hexLen)).
			WithOperation("validateSource").
			WithContext("hash_length", hexLen)
	}
}

// shouldSkipExtract reports whether this source file should be skipped during
// extraction, matching makepkg's noextract semantics: exact basename match only.
func (src *Source) shouldSkipExtract() bool {
	return slices.Contains(src.NoExtract, filepath.Base(src.SourceItemPath))
}

// extractIfArchive runs archive.Extract on sourceFilePath, tolerating the
// ErrUnrecognizedArchive sentinel. Non-archive sources (plain patches,
// scripts, .sig files, …) are legitimately not extractable and are left in
// place; symlinkSources has already made them available in destDir.
// Directories (e.g. git working copies) are never archives, so we skip them
// up front to avoid spurious "is a directory" read errors from archive.Extract.
func extractIfArchive(sourceFilePath, destDir string) error {
	if info, statErr := os.Stat(sourceFilePath); statErr == nil && info.IsDir() {
		return nil
	}

	err := archive.Extract(context.Background(), sourceFilePath, destDir)
	if err == nil || stderrors.Is(err, archive.ErrUnrecognizedArchive) {
		return nil
	}

	return err
}
