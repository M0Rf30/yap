package source

import (
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/M0Rf30/yap/pkg/constants"
	"github.com/M0Rf30/yap/pkg/utils"
	ggit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
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
	var err error

	src.parseURI()

	sourceType := src.getProtocol()

	switch sourceType {
	case "http":
		src.getURL("http")
	case "https":
		src.getURL("https")
	case "ftp":
		src.getURL("ftp")
	case "git":
		src.getURL("git")
	case "file":
	default:
		fmt.Printf("%s❌ :: %sunknown or unsupported source type: %s\n",
			string(constants.ColorBlue),
			string(constants.ColorYellow), sourceType)

		os.Exit(1)
	}

	if err != nil {
		return err
	}

	err = src.validate()
	if err != nil {
		return err
	}

	err = src.symlinkSources()
	if err != nil {
		return err
	}

	err = src.extract()
	if err != nil {
		return err
	}

	return err
}

// extract extracts the source file to the destination directory.
//
// It opens the source file specified by the SourceItemPath field of the Source struct.
// If the file cannot be opened, it prints an error message and returns the error.
// Otherwise, it unarchives the file to the destination directory specified by the SrcDir field of the Source struct.
// If the unarchiving fails, it prints an error message and panics.
// Finally, it returns any error that occurred during the extraction process.
func (src *Source) extract() error {
	dlFile, err := os.Open(filepath.Join(src.StartDir, src.SourceItemPath))
	if err != nil {
		fmt.Printf("%s❌ :: %sfailed to open source %s\n",
			string(constants.ColorBlue),
			string(constants.ColorYellow), src.SourceItemURI)

		return err
	}

	err = utils.Unarchive(dlFile, src.SrcDir)
	if err != nil {
		fmt.Printf("%s❌ :: %sfailed to extract source %s\n",
			string(constants.ColorBlue),
			string(constants.ColorYellow), src.SourceItemPath)

		log.Panic(err)
	}

	return err
}

// getReferenceType returns the reference type for the given source.
//
// It takes no parameters.
// It returns a plumbing.ReferenceName.
func (src *Source) getReferenceType() plumbing.ReferenceName {
	var referenceName plumbing.ReferenceName

	switch src.RefKey {
	case "branch":
		referenceName = plumbing.NewBranchReferenceName(src.RefValue)
	case "tag":
		referenceName = plumbing.NewTagReferenceName(src.RefValue)
	default:
	}

	return referenceName
}

// getProtocol returns the protocol of the source item URI.
//
// It checks if the source item URI starts with "http://", "https://", or "ftp://".
// If it does, it returns the corresponding protocol.
// If the source item URI starts with "git+https://", it returns "git".
// Otherwise, it returns "file".
//
// Returns:
//
//	string: The protocol of the source item URI.
func (src *Source) getProtocol() string {
	if strings.HasPrefix(src.SourceItemURI, "http://") {
		return "http"
	}

	if strings.HasPrefix(src.SourceItemURI, "https://") {
		return "https"
	}

	if strings.HasPrefix(src.SourceItemURI, "ftp://") {
		return "ftp"
	}

	if strings.HasPrefix(src.SourceItemURI, git+"+https://") {
		return git
	}

	return "file"
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
		split := strings.Split(src.SourceItemURI, "::")
		src.SourceItemPath = split[0]
		src.SourceItemURI = split[1]
	}

	if strings.Contains(src.SourceItemURI, "#") {
		split := strings.Split(src.SourceItemURI, "#")
		src.SourceItemURI = split[0]
		fragment := split[1]
		splitFragment := strings.Split(fragment, "=")
		src.RefKey = splitFragment[0]
		src.RefValue = splitFragment[1]
	}
}

// symlinkSources creates a symbolic link from symlinkSource to symLinkTarget.
//
// It returns an error if the symlink creation fails.
func (src *Source) symlinkSources() error {
	var err error

	symlinkSource := filepath.Join(src.StartDir, src.SourceItemPath)

	symLinkTarget := filepath.Join(src.SrcDir, src.SourceItemPath)

	if !utils.Exists(symLinkTarget) {
		err = os.Symlink(symlinkSource, symLinkTarget)
	}

	return err
}

// validate validates the source by checking its integrity.
//
// It checks the hash of the source file against the expected hash value.
// If the hashes don't match, it returns an error.
// The function takes no parameters and returns an error.
func (src *Source) validate() error {
	info, err := os.Stat(filepath.Join(src.StartDir, src.SourceItemPath))
	if err != nil {
		fmt.Printf("%s❌ :: %sfailed to open file for hash\n",
			string(constants.ColorBlue),
			string(constants.ColorYellow))

		return err
	}

	var hashSum hash.Hash

	if src.Hash == "SKIP" || info.IsDir() {
		fmt.Printf("%s:: %sSkip integrity check for %s%s\n",
			string(constants.ColorBlue),
			string(constants.ColorYellow),
			string(constants.ColorWhite),
			src.SourceItemURI)
	}

	switch len(src.Hash) {
	case 64:
		hashSum = sha256.New()
	case 128:
		hashSum = sha512.New()
	default:
		return err
	}

	file, _ := os.Open(filepath.Join(src.StartDir, src.SourceItemPath))

	_, err = io.Copy(hashSum, file)
	if err != nil {
		return err
	}

	sum := hashSum.Sum([]byte{})
	hexSum := hex.EncodeToString(sum)

	if hexSum != src.Hash {
		fmt.Printf("%s❌ :: %sHash verification failed for %s\n",
			string(constants.ColorBlue),
			string(constants.ColorYellow), src.SourceItemPath)
	}

	return err
}
