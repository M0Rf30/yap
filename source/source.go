package source

import (
	"crypto/sha256"
	"crypto/sha512"
	"fmt"
	"hash"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/M0Rf30/yap/constants"
	"github.com/M0Rf30/yap/utils"
	ggit "github.com/go-git/go-git/v5"
)

const (
	git = "git"
)

type Source struct {
	StartDir       string
	Hash           string
	SourceItemURI  string
	SrcDir         string
	SourceItemPath string
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

func (src *Source) parsePath() {
	src.SourceItemPath = utils.Filename(src.SourceItemURI)
}

func (src *Source) getURL(protocol string) {
	dloadFilePath := filepath.Join(src.StartDir, src.SourceItemPath)

	if protocol != git {
		utils.Download(dloadFilePath, src.SourceItemURI)
	}

	if protocol == git {
		_, err := ggit.PlainClone(dloadFilePath,
			false, &ggit.CloneOptions{
				URL:      strings.Trim(src.SourceItemURI, "git+"),
				Progress: os.Stdout,
			})

		if err != nil && err.Error() == "repository already exists" {
			_, _ = ggit.PlainOpenWithOptions(dloadFilePath, &ggit.PlainOpenOptions{
				DetectDotGit:          true,
				EnableDotGitCommonDir: true,
			})
		}
	}
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

func (src *Source) validate() error {
	info, err := os.Stat(filepath.Join(src.StartDir, src.SourceItemPath))
	if err != nil {
		fmt.Printf("%s❌ :: %sfailed to open file for hash\n",
			string(constants.ColorBlue),
			string(constants.ColorYellow))

		return err
	}

	var hash hash.Hash

	if src.Hash == "SKIP" || info.IsDir() {
		fmt.Printf("%s:: %sSkip integrity check for %s%s\n",
			string(constants.ColorBlue),
			string(constants.ColorYellow),
			string(constants.ColorWhite),
			src.SourceItemURI)
	}

	switch len(src.Hash) {
	case 64:
		hash = sha256.New()
	case 128:
		hash = sha512.New()
	default:
		return err
	}

	file, _ := os.Open(filepath.Join(src.StartDir, src.SourceItemPath))

	_, err = io.Copy(hash, file)
	if err != nil {
		return err
	}

	sum := hash.Sum([]byte{})

	hexSum := fmt.Sprintf("%x", sum)

	if hexSum != src.Hash {
		fmt.Printf("%s❌ :: %sHash verification failed for %s\n",
			string(constants.ColorBlue),
			string(constants.ColorYellow), src.SourceItemURI)
	}

	return err
}

func (src *Source) Get() error {
	var err error

	src.parsePath()

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

func (src *Source) symlinkSources() error {
	var err error

	symlinkSource := filepath.Join(src.StartDir, src.SourceItemPath)

	symLinkTarget := filepath.Join(src.SrcDir, src.SourceItemPath)

	if !utils.Exists(symLinkTarget) {
		err = os.Symlink(symlinkSource, symLinkTarget)
	}

	return err
}
