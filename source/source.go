package source

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"fmt"
	"hash"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/packagefoundation/yap/constants"
	"github.com/packagefoundation/yap/utils"
)

const (
	file  = "file"
	http  = "http"
	https = "https"
	ftp   = "ftp"
	git   = "git"
)

type Source struct {
	Root   string
	Hash   string
	Source string
	Output string
	Path   string
}

func (s *Source) getType() string {
	if strings.HasPrefix(s.Source, "http") {
		return http
	}
	if strings.HasPrefix(s.Source, "ftp") {
		return ftp
	}
	if strings.HasPrefix(s.Source, "git") {
		return git
	}
	return file
}

func (s *Source) parsePath() {
	s.Path = filepath.Join(s.Output, utils.Filename(s.Source))
}

func (s *Source) getUrl(protocol string) (err error) {
	exists, err := utils.Exists(s.Path)
	if err != nil {
		return
	}

	if !exists {
		err = utils.HttpGet(s.Source, s.Path, protocol)
		if err != nil {
			return
		}
	}

	return
}

func (s *Source) getPath() (err error) {
	err = utils.Copy(s.Root, s.Source, s.Path, true)
	if err != nil {
		return
	}

	return
}

func (s *Source) extract() (err error) {
	var cmd *exec.Cmd
	if strings.HasSuffix(s.Path, ".tar") {
		cmd = exec.Command("tar", "--no-same-owner", "-xf", s.Path)
	} else if strings.HasSuffix(s.Path, ".tgz") {
		cmd = exec.Command("tar", "--no-same-owner", "-xf", s.Path)
	} else if strings.HasSuffix(s.Path, ".zip") {
		cmd = exec.Command("unzip", s.Path)
	} else {
		split := strings.Split(s.Path, ".")
		if len(split) > 2 && split[len(split)-2] == "tar" {
			cmd = exec.Command("tar", "--no-same-owner", "-xf", s.Path)
		} else {
			return
		}
	}

	cmd.Dir = s.Output
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = cmd.Run()
	if err != nil {
		fmt.Printf("%s❌ :: %sfailed to extract source %s\n", string(constants.ColorBlue), string(constants.ColorYellow), s.Source)
		log.Fatal(err)
		return
	}

	return
}

func (s *Source) validate() (err error) {
	if strings.ToLower(s.Hash) == "skip" {
		return
	}

	file, err := os.Open(s.Path)
	if err != nil {
		fmt.Printf("source: Failed to open file for hash")
		log.Fatal(err)
		return
	}
	defer file.Close()

	var hash hash.Hash
	switch len(s.Hash) {
	case 32:
		hash = md5.New()
	case 40:
		hash = sha1.New()
	case 64:
		hash = sha256.New()
	case 128:
		hash = sha512.New()
	default:
		fmt.Printf("source: Unknown hash type for hash '%s'\n", s.Hash)
		return
	}

	_, err = io.Copy(hash, file)
	if err != nil {
		return
	}

	sum := hash.Sum([]byte{})

	hexSum := fmt.Sprintf("%x", sum)

	if hexSum != s.Hash {
		fmt.Printf("source: Hash verification failed for '%s'\n", s.Source)
		return
	}

	return
}

func (s *Source) Get() (err error) {
	s.parsePath()

	switch s.getType() {
	case http:
		err = s.getUrl(http)
	case https:
		err = s.getUrl(https)
	case ftp:
		err = s.getUrl(ftp)
	case git:
		err = s.getUrl(git)
	case file:
		err = s.getPath()
	default:
		panic("utils: Unknown type")
	}
	if err != nil {
		return
	}

	err = s.validate()
	if err != nil {
		return
	}

	err = s.extract()
	if err != nil {
		return
	}

	return
}
