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
	ggit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/pkg/errors"
)

const (
	git = "git"
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

// Get retrieves the source from the specified URI and performs necessary operations on it.
//
// It parses the URI, determines the source type, and calls the appropriate getURL function.
// If the source type is not recognized or supported, it prints an error message and exits.
// It then validates the source, creates symbolic links for sources, and extracts the source.
//
// Returns an error if any of the operations fail.
func (src *Source) Get() error {
	src.parseURI()

	sourceType := src.getProtocol()

	switch sourceType {
	case "http", "https", "ftp", "git":
		src.getURL(sourceType)
	case "file":
		return nil
	default:
		return errors.New("unsupported source type")
	}

	if err := src.validate(); err != nil {
		return err
	}

	if err := src.symlinkSources(); err != nil {
		return err
	}

	if err := src.extract(); err != nil {
		return err
	}

	return nil
}

// extract is a function that extracts a source file to a specified directory.
//
// It takes no parameters.
// It returns an error if there was a problem opening the source file or
// extracting it.
func (src *Source) extract() error {
	dlFile, err := os.Open(filepath.Join(src.StartDir, src.SourceItemPath))
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
	case strings.HasPrefix(src.SourceItemURI, git+"+https://"):
		return git
	default:
		return ""
	}
}

// getURL retrieves the URL for the Source object based on the specified protocol.
//
// protocol: The protocol to use for retrieving the URL.
// Returns: None.
func (src *Source) getURL(protocol string) {
	dloadFilePath := filepath.Join(src.StartDir, src.SourceItemPath)

	if protocol != git {
		utils.Download(dloadFilePath, src.SourceItemURI)
	}

	if protocol == git {
		_, err := ggit.PlainClone(dloadFilePath,
			false, &ggit.CloneOptions{
				Progress:      os.Stdout,
				ReferenceName: src.getReferenceType(),
				URL:           strings.Trim(src.SourceItemURI, "git+"),
			})

		if err != nil && err.Error() == "repository already exists" {
			_, _ = ggit.PlainOpenWithOptions(dloadFilePath, &ggit.PlainOpenOptions{
				DetectDotGit:          true,
				EnableDotGitCommonDir: true,
			})
		}
	}
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
func (src *Source) symlinkSources() error {
	symlinkSource := filepath.Join(src.StartDir, src.SourceItemPath)

	symLinkTarget := filepath.Join(src.SrcDir, src.SourceItemPath)

	if !utils.Exists(symLinkTarget) {
		return os.Symlink(symlinkSource, symLinkTarget)
	}

	return nil
}

// validate checks the source's integrity by comparing its hash with the expected hash value.
// It returns an error if the hashes don't match.
func (src *Source) validate() error {
	info, err := os.Stat(filepath.Join(src.StartDir, src.SourceItemPath))
	if err != nil {
		return fmt.Errorf("failed to open file for hash: %w", err)
	}

	if src.Hash == "SKIP" || info.IsDir() {
		fmt.Printf("%s:: %sSkip integrity check for %s%s\n",
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

	file, err := os.Open(filepath.Join(src.StartDir, src.SourceItemPath))
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
		return errors.Wrapf(errors.New("hash verification failed for %s"), src.SourceItemPath)
	}

	return nil
}
