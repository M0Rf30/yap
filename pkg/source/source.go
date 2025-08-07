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

	"github.com/M0Rf30/yap/pkg/constants"
	"github.com/M0Rf30/yap/pkg/osutils"
)

const (
	fileProtocol = "file"
)

var (
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
		if !osutils.Exists(sourceFilePath) {
			err = src.getURL(sourceType, sourceFilePath, SSHPassword)
		}

		if err != nil {
			return err
		}
	case fileProtocol:
	default:
		return errors.Errorf("unsupported source type")
	}

	err := src.validateSource(sourceFilePath)
	if err != nil {
		return err
	}

	err = src.symlinkSources(sourceFilePath)
	if err != nil {
		return err
	}

	err = osutils.Unarchive(sourceFilePath, src.SrcDir)
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

	// Create component logger for this package
	var logger *osutils.ComponentLogger
	if src.PkgName != "" {
		logger = osutils.WithComponent(src.PkgName)
	}

	switch protocol {
	case constants.Git:
		referenceName := src.getReferenceType()

		return osutils.GitClone(dloadFilePath, normalizedURI, sshPassword, referenceName, logger)
	default:
		// Use enhanced download with resume capability and 3 retries, with context information
		return osutils.DownloadWithResumeContext(dloadFilePath, normalizedURI, logger, 3, src.PkgName, src.SourceItemPath)
	}
}

// parseURI parses the URI of the Source and updates the SourceItemPath,
// SourceItemURI, RefKey, and RefValue fields accordingly.
//
// No parameters.
// No return types.
func (src *Source) parseURI() {
	src.SourceItemPath = osutils.Filename(src.SourceItemURI)

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
		if src.SourceItemPath == osutils.Filename(split[0]+"#"+fragment) {
			src.SourceItemPath = osutils.Filename(src.SourceItemURI)
		}
	}
}

// symlinkSources creates a symbolic link from symlinkSource to symLinkTarget.
//
// It returns an error if the symlink creation fails.
func (src *Source) symlinkSources(symlinkSource string) error {
	symlinkTarget := filepath.Join(src.SrcDir, src.SourceItemPath)
	if !osutils.Exists(symlinkTarget) {
		return os.Symlink(symlinkSource, symlinkTarget)
	}

	return nil
}

// validateSource checks the integrity of the source files.
//
// It takes the source file path as a parameter and returns an error if any.
func (src *Source) validateSource(sourceFilePath string) error {
	// Create component logger for this package
	var logger *osutils.ComponentLogger
	if src.PkgName != "" {
		logger = osutils.WithComponent(src.PkgName)
	} else {
		logger = osutils.ServiceLogger()
	}

	info, err := os.Stat(sourceFilePath)
	if err != nil {
		return errors.Errorf("failed to open file for hash %s", sourceFilePath)
	}

	if src.Hash == "SKIP" || info.IsDir() {
		logger.Info("skip integrity check for", osutils.Logger.Args("source", src.SourceItemURI))

		return nil
	}

	var hashSum hash.Hash

	switch len(src.Hash) {
	case 64:
		hashSum = sha256.New()
	case 128:
		hashSum = sha512.New()
	default:
		return errors.Errorf("unsupported hash length %d", len(src.Hash))
	}

	file, err := osutils.Open(filepath.Clean(sourceFilePath))
	if err != nil {
		return err
	}

	defer func() {
		err := file.Close()
		if err != nil {
			osutils.Logger.Warn("failed to close source file", osutils.Logger.Args("path", sourceFilePath, "error", err))
		}
	}()

	_, err = io.Copy(hashSum, file)
	if err != nil {
		return errors.Errorf("failed to copy file %s", sourceFilePath)
	}

	sum := hashSum.Sum(nil)
	hexSum := hex.EncodeToString(sum)

	if hexSum != src.Hash {
		return errors.Errorf("hash verification failed %s", src.SourceItemPath)
	}

	logger.Info("integrity check for", osutils.Logger.Args("source", src.SourceItemURI))

	return nil
}

// GetConcurrently retrieves multiple source files concurrently with enhanced progress tracking.
// This function downloads all remote sources (http/https/ftp) in parallel while maintaining
// proper progress reporting and error handling.
//
// Parameters:
// - sources: slice of Source structs to download
// - maxConcurrent: maximum number of concurrent downloads (0 = default)
//
// Returns an error if any critical download fails.
func GetConcurrently(sources []*Source, maxConcurrent int) error {
	if len(sources) == 0 {
		return nil
	}

	// Separate sources by type for optimal processing
	var (
		downloadSources []*Source
		localSources    []*Source
	)

	for _, src := range sources {
		src.parseURI()
		sourceType := src.getProtocol()

		switch sourceType {
		case "http", "https", "ftp":
			downloadSources = append(downloadSources, src)
		case constants.Git, fileProtocol:
			localSources = append(localSources, src)
		}
	}

	// Process local sources (git/file) sequentially first
	// These typically need to be processed in order and don't benefit from concurrency
	for _, src := range localSources {
		err := src.Get()
		if err != nil {
			return err
		}
	}

	// Process download sources concurrently if there are multiple
	if len(downloadSources) > 1 && maxConcurrent > 0 {
		return processConcurrentDownloads(downloadSources, maxConcurrent)
	}

	// Process download sources sequentially if only one or concurrency disabled
	for _, src := range downloadSources {
		err := src.Get()
		if err != nil {
			return err
		}
	}

	return nil
}

// processConcurrentDownloads handles multiple downloads concurrently.
func processConcurrentDownloads(sources []*Source, maxConcurrent int) error {
	downloads, sourceMap := prepareDownloadMap(sources)
	if len(downloads) == 0 {
		return nil // All files already exist
	}

	logger := createSourceLogger(sources)
	results := osutils.DownloadConcurrently(downloads, logger, maxConcurrent, 3)

	return processDownloadResults(results, sourceMap)
}

// prepareDownloadMap builds the download map and source mapping.
func prepareDownloadMap(sources []*Source) (map[string]string, map[string]*Source) {
	downloads := make(map[string]string)
	sourceMap := make(map[string]*Source)

	for _, src := range sources {
		sourceFilePath := filepath.Join(src.StartDir, src.SourceItemPath)

		// Skip if file already exists (respecting existing duplicate download prevention)
		if osutils.Exists(sourceFilePath) {
			continue
		}

		normalizedURI := strings.TrimPrefix(src.SourceItemURI, constants.Git+"+")
		downloads[sourceFilePath] = normalizedURI
		sourceMap[sourceFilePath] = src
	}

	return downloads, sourceMap
}

// createSourceLogger creates a component logger from the first source if available.
func createSourceLogger(sources []*Source) *osutils.ComponentLogger {
	if len(sources) > 0 && sources[0].PkgName != "" {
		return osutils.WithComponent(sources[0].PkgName)
	}

	return nil
}

// processDownloadResults processes download results and performs post-download operations.
func processDownloadResults(results map[string]error, sourceMap map[string]*Source) error {
	var firstError error

	for destination, err := range results {
		if err != nil {
			if firstError == nil {
				firstError = errors.Errorf("download failed for %s: %v", destination, err)
			}

			continue
		}

		// Perform validation, symlinking, and extraction for successful downloads
		processErr := processSuccessfulDownload(destination, sourceMap[destination])
		if processErr != nil && firstError == nil {
			firstError = processErr
		}
	}

	return firstError
}

// processSuccessfulDownload handles post-download processing for a successful download.
func processSuccessfulDownload(destination string, src *Source) error {
	if src == nil {
		return nil
	}

	err := src.validateSource(destination)
	if err != nil {
		return err
	}

	err = src.symlinkSources(destination)
	if err != nil {
		return err
	}

	return osutils.Unarchive(destination, src.SrcDir)
}
