package source

import (
	"crypto/sha256"
	"crypto/sha512"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/M0Rf30/yap/constants"
	"github.com/M0Rf30/yap/utils"
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

func (s *Source) getType() string {
	if strings.HasPrefix(s.Source, "http://") {
		return "http"
	}

	if strings.HasPrefix(s.Source, "https://") {
		return "https"
	}

	if strings.HasPrefix(s.Source, "ftp://") {
		return "ftp"
	}

	if strings.HasPrefix(s.Source, git+"+https://") {
		return git
	}

	return "file"
}

func (s *Source) parsePath() {
	s.Path = filepath.Join(s.Output, utils.Filename(s.Source))
}

func (s *Source) getURL(protocol string) error {
	exists, err := utils.Exists(s.Path)
	if err != nil {
		return err
	}

	if !exists {
		if protocol != git {
			err = utils.HTTPGet(s.Source, s.Path, protocol)
		}

		if protocol == git {
			err = utils.Exec("", git, "clone", strings.Trim(s.Source, "git+"), s.Path)
		}
	}

	return err
}

func (s *Source) getPath() error {
	err := utils.Copy(s.Root, s.Source, s.Path, true)
	if err != nil {
		return err
	}

	return err
}

func (s *Source) extract() error {
	var err error

	switch {
	case strings.HasSuffix(s.Path, ".tar"):
		err = utils.Exec("tar", "--no-same-owner", "-xf", s.Path)
	case strings.HasSuffix(s.Path, ".zip"):
		err = utils.Exec("", "unzip", s.Path)
	default:
		split := strings.Split(s.Path, ".")
		if len(split) > 2 && split[len(split)-2] == "tar" {
			err = utils.Exec("tar", "--no-same-owner", "-xf", s.Path)
		} else {
			return err
		}
	}

	if err != nil {
		fmt.Printf("%s❌ :: %sfailed to extract source %s\n",
			string(constants.ColorBlue),
			string(constants.ColorYellow), s.Source)

		return err
	}

	return err
}

func (s *Source) validate() error {
	var err error

	file, err := os.Open(s.Path)
	if err != nil {
		fmt.Printf("source: Failed to open file for hash")

		return err
	}

	defer file.Close()

	var hash hash.Hash

	if s.Hash == "SKIP" {
		fmt.Printf("\t\t%s:: %sSkip integrity check for %s%s\n",
			string(constants.ColorBlue),
			string(constants.ColorYellow),
			string(constants.ColorWhite),
			s.Source)
	}

	switch len(s.Hash) {
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

	if hexSum != s.Source {
		fmt.Printf("source: Hash verification failed for '%s'\n", s.Source)
	}

	return err
}

func (s *Source) Get() error {
	s.parsePath()

	var err error

	switch s.getType() {
	case "http":
		err = s.getURL("http")
	case "https":
		err = s.getURL("https")
	case "ftp":
		err = s.getURL("ftp")
	case "git":
		err = s.getURL("git")
	case "file":
		err = s.getPath()
	default:
		panic("utils: Unknown type")
	}

	if err != nil {
		return err
	}

	err = s.validate()
	if err != nil {
		return err
	}

	err = s.extract()
	if err != nil {
		return err
	}

	return err
}
