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

type Source struct {
	Hash           string
	RefKey         string
	RefValue       string
	SourceItemPath string
	SourceItemURI  string
	SrcDir         string
	StartDir       string
}

func (src *Source) Get() error {
	var err error

	src.parseURI()

	sourceType := src.getType()

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

func (src *Source) getType() string {
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

func (src *Source) symlinkSources() error {
	var err error

	symlinkSource := filepath.Join(src.StartDir, src.SourceItemPath)

	symLinkTarget := filepath.Join(src.SrcDir, src.SourceItemPath)

	if !utils.Exists(symLinkTarget) {
		err = os.Symlink(symlinkSource, symLinkTarget)
	}

	return err
}

// validate checks that items declared in the source array have a valid hashsum.
// It returns any error encountered.
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
