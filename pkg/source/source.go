// Package source provides source file download and management functionality.
package source

import (
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"hash"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/pkg/errors"

	"github.com/M0Rf30/yap/v2/pkg/archive"
	"github.com/M0Rf30/yap/v2/pkg/constants"
	"github.com/M0Rf30/yap/v2/pkg/download"
	"github.com/M0Rf30/yap/v2/pkg/files"
	"github.com/M0Rf30/yap/v2/pkg/git"
	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
	"github.com/M0Rf30/yap/v2/pkg/shell"
)

const (
	fileProtocol = "file"
)

// Global variables for source handling
var (
	// SSHPassword contains the SSH password for authentication.
	SSHPassword string
	// downloadMutexes tracks ongoing downloads to prevent duplicate downloads.
	downloadMutexes  = make(map[string]*sync.Mutex)
	downloadMapMutex = sync.Mutex{}
)

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
	// SSHPassword is used to store the password for SSH authentication.
	// SSHPassword contains the SSH password for authentication.
	SSHPassword string
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
}

// Get retrieves the source file from the specified URI.
//
// It parses the URI and determines the source file path and type.
// If the source file does not exist, it retrieves it from the specified URI.
// It validates the source file and symlinks any additional source files.
// Finally, it extracts the source file if necessary.
//
// Returns an error if any step fails.
func (src *Source) Get() error {
	src.parseURI()
	sourceFilePath := filepath.Join(src.StartDir, src.SourceItemPath)
	sourceType := src.getProtocol()

	switch sourceType {
	case "http", "https", "ftp", constants.Git:
		var err error

		// Use mutex to prevent duplicate downloads of the same file
		downloadMapMutex.Lock()

		mutex, exists := downloadMutexes[sourceFilePath]
		if !exists {
			mutex = &sync.Mutex{}
			downloadMutexes[sourceFilePath] = mutex
		}

		downloadMapMutex.Unlock()

		// Lock this specific file's download
		mutex.Lock()
		defer mutex.Unlock()

		// Check again after acquiring the lock (double-checked locking pattern)
		if !files.Exists(sourceFilePath) {
			err = src.getURL(sourceType, sourceFilePath, SSHPassword)
		}

		if err != nil {
			return err
		}
	case fileProtocol:
	default:
		return errors.Errorf("%s", i18n.T("errors.source.unsupported_source_type"))
	}

	err := src.validateSource(sourceFilePath)
	if err != nil {
		return err
	}

	err = src.symlinkSources(sourceFilePath)
	if err != nil {
		return err
	}

	err = archive.Extract(sourceFilePath, src.SrcDir)
	if err != nil {
		return err
	}

	return nil
}

// getReferenceType returns the reference type for the given source.
//
// It takes no parameters.
// It returns a plumbing.ReferenceName.
func (src *Source) getReferenceType() plumbing.ReferenceName {
	switch src.RefKey {
	case "branch":
		return plumbing.NewBranchReferenceName(src.RefValue)
	case "tag":
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

// getURL is a function that retrieves a URL based on the provided protocol and
// download file path.
//
// Parameters:
// - protocol: a string representing the protocol for the URL.
// - dloadFilePath: a string representing the file path for the downloaded file.
func (src *Source) getURL(protocol, dloadFilePath, sshPassword string) error {
	normalizedURI := strings.TrimPrefix(src.SourceItemURI, constants.Git+"+")

	switch protocol {
	case constants.Git:
		referenceName := src.getReferenceType()

		return git.Clone(dloadFilePath, normalizedURI, sshPassword, referenceName)
	default:
		// Use enhanced download with resume capability and 3 retries, with context information
		_, err := shell.MultiPrinter.Start()
		if err != nil {
			return err
		}

		return download.WithResumeContext(
			dloadFilePath,
			normalizedURI,
			3,
			src.PkgName,
			src.SourceItemPath,
			shell.MultiPrinter.Writer)
	}
}

// parseURI parses the URI of the Source and updates the SourceItemPath,
// SourceItemURI, RefKey, and RefValue fields accordingly.
//
// No parameters.
// No return types.
func (src *Source) parseURI() {
	src.SourceItemPath = filepath.Base(src.SourceItemURI)

	if strings.Contains(src.SourceItemURI, "::") {
		split := strings.SplitN(src.SourceItemURI, "::", 2)
		src.SourceItemPath = split[0]
		src.SourceItemURI = split[1]
	}

	if strings.Contains(src.SourceItemURI, "#") {
		split := strings.SplitN(src.SourceItemURI, "#", 2)
		src.SourceItemURI = split[0]
		fragment := split[1]
		splitFragment := strings.SplitN(fragment, "=", 2)
		src.RefKey = splitFragment[0]
		src.RefValue = splitFragment[1]

		// Update SourceItemPath to remove the fragment only if no custom name was used
		if src.SourceItemPath == filepath.Base(split[0]+"#"+fragment) {
			src.SourceItemPath = filepath.Base(src.SourceItemURI)
		}
	}
}

// symlinkSources creates a symbolic link from symlinkSource to symLinkTarget.
//
// It returns an error if the symlink creation fails.
func (src *Source) symlinkSources(symlinkSource string) error {
	symlinkTarget := filepath.Join(src.SrcDir, src.SourceItemPath)

	// Check if target already exists and points to the correct source
	if linkTarget, err := os.Readlink(symlinkTarget); err == nil {
		if linkTarget == symlinkSource {
			return nil // Already correctly linked
		}
		// Remove existing incorrect symlink
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
		return errors.Errorf(i18n.T("errors.source.failed_to_open_file_for_hash"), sourceFilePath)
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

		return errors.Errorf(i18n.T("errors.source.directory_not_supported"), sourceFilePath)
	}

	if src.Hash == "SKIP" {
		logger.Info(i18n.T("logger.skip_integrity_check_for"),
			"source", src.SourceItemURI)

		return nil
	}

	var hashSum hash.Hash

	switch len(src.Hash) {
	case 64:
		hashSum = sha256.New()
	case 128:
		hashSum = sha512.New()
	default:
		return errors.Errorf(i18n.T("errors.source.unsupported_hash_length"), len(src.Hash))
	}

	file, err := files.Open(filepath.Clean(sourceFilePath))
	if err != nil {
		return err
	}

	defer func() {
		err := file.Close()
		if err != nil {
			logger.Warn(
				"failed to close source file",
				"path", sourceFilePath,
				"error", err)
		}
	}()

	_, err = io.Copy(hashSum, file)
	if err != nil {
		return errors.Errorf(i18n.T("errors.source.failed_to_copy_file"), sourceFilePath)
	}

	sum := hashSum.Sum(nil)
	hexSum := hex.EncodeToString(sum)

	if hexSum != src.Hash {
		return errors.Errorf(i18n.T("errors.source.hash_verification_failed"), src.SourceItemPath)
	}

	logger.Info(i18n.T("logger.integrity_check_for"),
		"source", src.SourceItemURI)

	return nil
}
