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
	Root   string
	Hash   string
	Source string
	Output string
	Path   string
}

func (src *Source) getType() string {
	if strings.HasPrefix(src.Source, "http://") {
		return "http"
	}

	if strings.HasPrefix(src.Source, "https://") {
		return "https"
	}

	if strings.HasPrefix(src.Source, "ftp://") {
		return "ftp"
	}

	if strings.HasPrefix(src.Source, git+"+https://") {
		return git
	}

	return "file"
}

func (src *Source) parsePath() {
	src.Path = filepath.Join(src.Output, utils.Filename(src.Source))
}

func (src *Source) getURL(protocol string) error {
	exists, err := utils.Exists(src.Path)
	if err != nil {
		return err
	}

	if !exists {
		if protocol != git {
			utils.Download(src.Path, src.Source)
		}

		if protocol == git {
			_, err = ggit.PlainClone(src.Path, false, &ggit.CloneOptions{
				URL:      strings.Trim(src.Source, "git+"),
				Progress: os.Stdout,
			})
		}
	}

	return err
}

func (src *Source) getPath() error {
	err := utils.CopyFile(filepath.Join(src.Root, src.Source), src.Path)
	if err != nil {
		return err
	}

	return err
}

func (src *Source) extract() error {
	dlFile, err := os.Open(src.Path)
	if err != nil {
		fmt.Printf("%s❌ :: %sfailed to open source %s\n",
			string(constants.ColorBlue),
			string(constants.ColorYellow), src.Source)

		return err
	}

	err = utils.Unarchive(dlFile, src.Output)
	if err != nil {
		fmt.Printf("%s❌ :: %sfailed to extract source %s\n",
			string(constants.ColorBlue),
			string(constants.ColorYellow), src.Path)

		log.Panic(err)
	}

	return err
}

func (src *Source) validate() error {
	var err error

	file, err := os.Open(src.Path)
	if err != nil {
		fmt.Printf("%s❌ :: %sfailed to open file for hash\n",
			string(constants.ColorBlue),
			string(constants.ColorYellow))

		return err
	}

	defer file.Close()

	var hash hash.Hash

	if src.Hash == "SKIP" {
		fmt.Printf("%s:: %sSkip integrity check for %s%s\n",
			string(constants.ColorBlue),
			string(constants.ColorYellow),
			string(constants.ColorWhite),
			src.Source)
	}

	switch len(src.Hash) {
	case 64:
		hash = sha256.New()
	case 128:
		hash = sha512.New()
	default:
		return err
	}

	_, err = io.Copy(hash, file)
	if err != nil {
		return err
	}

	sum := hash.Sum([]byte{})

	hexSum := fmt.Sprintf("%x", sum)

	if hexSum != src.Hash {
		fmt.Printf("%s❌ :: %sHash verification failed for %s\n",
			string(constants.ColorBlue),
			string(constants.ColorYellow), src.Source)
	}

	return err
}

func (src *Source) Get() error {
	src.parsePath()

	var err error

	switch src.getType() {
	case "http":
		err = src.getURL("http")
	case "https":
		err = src.getURL("https")
	case "ftp":
		err = src.getURL("ftp")
	case "git":
		err = src.getURL("git")
	case "file":
		err = src.getPath()
	default:
		panic("utils: Unknown type")
	}

	if err != nil {
		return err
	}

	err = src.validate()
	if err != nil {
		return err
	}

	err = src.extract()

	if err != nil {
		return err
	}

	return err
}
