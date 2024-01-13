package source

import (
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/M0Rf30/yap/pkg/constants"
	"github.com/M0Rf30/yap/pkg/utils"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/pkg/errors"
)

var (
	SSHPassword string
)

// Source defines all the fields accepted by a source item.
type Source struct {
	// Hash is the integrity hashsum for a source item
	Hash string
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
		if !utils.Exists(sourceFilePath) {
			return src.getURL(sourceType, sourceFilePath, SSHPassword)
		}
	case "file":
	default:
		return errors.New("unsupported source type")
	}

	if err := src.validateSource(sourceFilePath); err != nil {
		return err
	}

	if err := src.symlinkSources(sourceFilePath); err != nil {
		return err
	}

	if err := src.extract(sourceFilePath); err != nil {
		return err
	}

	return nil
}

// extract extracts the contents of a source file to the specified source
// directory.
//
// sourceFilePath: The path to the source file to be extracted. Returns: An
// error if there was a problem opening the source file or extracting its
// contents.
func (src *Source) extract(sourceFilePath string) error {
	dlFile, err := os.Open(filepath.Clean(sourceFilePath))
	if err != nil {
		return fmt.Errorf("failed to open source %s: %w", src.SourceItemURI, err)
	}

	err = utils.Unarchive(dlFile, src.SrcDir)
	if err != nil {
		return fmt.Errorf("failed to extract source %s: %w", src.SourceItemPath, err)
	}

	return nil
}

// getReferenceType returns the reference type for the given source.
//
// It takes no parameters.
// It returns a plumbing.ReferenceName.
func (src *Source) getReferenceType() plumbing.ReferenceName {
	switch {
	case src.RefKey == "branch":
		return plumbing.NewBranchReferenceName(src.RefValue)
	case src.RefKey == "tag":
		return plumbing.NewTagReferenceName(src.RefValue)
	}

	return ""
}

// getProtocol returns the protocol of the source item URI.
func (src *Source) getProtocol() string {
	if !strings.Contains(src.SourceItemURI, "://") {
		return "file"
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
	var err error

	switch protocol {
	case constants.Git:
		referenceName := src.getReferenceType()
		err = utils.GitClone(src.SourceItemURI, dloadFilePath, sshPassword, referenceName)
	default:
		err = utils.Download(dloadFilePath, src.SourceItemURI)
	}

	return err
}

// parseURI parses the URI of the Source and updates the SourceItemPath,
// SourceItemURI, RefKey, and RefValue fields accordingly.
//
// No parameters.
// No return types.
func (src *Source) parseURI() {
	src.SourceItemPath = utils.Filename(src.SourceItemURI)

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
	}
}

// symlinkSources creates a symbolic link from symlinkSource to symLinkTarget.
//
// It returns an error if the symlink creation fails.
func (src *Source) symlinkSources(symlinkSource string) error {
	symLinkTarget := filepath.Join(src.SrcDir, src.SourceItemPath)

	if !utils.Exists(symLinkTarget) {
		return os.Symlink(symlinkSource, symLinkTarget)
	}

	return nil
}

// validateSource checks the integrity of the source files.
//
// It takes the source file path as a parameter and returns an error if any.
func (src *Source) validateSource(sourceFilePath string) error {
	info, err := os.Stat(sourceFilePath)
	if err != nil {
		return fmt.Errorf("failed to open file for hash: %w", err)
	}

	if src.Hash == "SKIP" || info.IsDir() {
		fmt.Printf("%s:: %sSkip integrity check for %s\t%s\n",
			string(constants.ColorBlue),
			string(constants.ColorYellow),
			string(constants.ColorWhite),
			src.SourceItemURI)

		return nil
	}

	var hashSum hash.Hash

	switch len(src.Hash) {
	case 64:
		hashSum = sha256.New()
	case 128:
		hashSum = sha512.New()
	default:
		return errors.Wrapf(errors.New("invalid hash length: %s"), strconv.Itoa(len(src.Hash)))
	}

	file, err := os.Open(filepath.Clean(sourceFilePath))
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	if _, err := io.Copy(hashSum, file); err != nil {
		return fmt.Errorf("failed to copy file: %w", err)
	}

	sum := hashSum.Sum(nil)
	hexSum := hex.EncodeToString(sum)

	if hexSum != src.Hash {
		return errors.Wrapf(errors.New("hash verification failed"), src.SourceItemPath)
	}

	fmt.Printf("%s:: %sIntegrity check for %s\t%s\n",
		string(constants.ColorBlue),
		string(constants.ColorYellow),
		string(constants.ColorWhite),
		src.SourceItemURI)

	return nil
}
