package source

import (
	"context"
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
	"github.com/cavaliergopher/grab/v3"
	ggit "github.com/go-git/go-git/v5"
	"github.com/mholt/archiver/v4"
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

func (source *Source) getType() string {
	if strings.HasPrefix(source.Source, "http://") {
		return "http"
	}

	if strings.HasPrefix(source.Source, "https://") {
		return "https"
	}

	if strings.HasPrefix(source.Source, "ftp://") {
		return "ftp"
	}

	if strings.HasPrefix(source.Source, git+"+https://") {
		return git
	}

	return "file"
}

func (source *Source) parsePath() {
	source.Path = filepath.Join(source.Output, utils.Filename(source.Source))
}

func (s *Source) getURL(protocol string) error {
	exists, err := utils.Exists(s.Path)
	if err != nil {
		return err
	}

	if !exists {
		if protocol != git {
			// create client
			client := grab.NewClient()
			req, _ := grab.NewRequest(s.Path, s.Source)

			// start download
			fmt.Printf("Downloading %v...\n", req.URL())
			resp := client.Do(req)
			fmt.Printf("  %v\n", resp.HTTPResponse.Status)

			// check for errors
			if err := resp.Err(); err != nil {
				fmt.Fprintf(os.Stderr, "Download failed: %v\n", err)
				os.Exit(1)
			}

			fmt.Printf("Download saved to ./%v \n", resp.Filename)
		}

		if protocol == git {
			_, err = ggit.PlainClone(s.Path, false, &ggit.CloneOptions{
				URL:      strings.Trim(s.Source, "git+"),
				Progress: os.Stdout,
			})
		}
	}

	return err
}

func (source *Source) getPath() error {
	err := utils.CopyFile(filepath.Join(source.Root, source.Source), source.Path)
	if err != nil {
		return err
	}

	return err
}

func (source *Source) Unarchive(input io.Reader, dir string) error {
	format, input, _ := archiver.Identify("", input)
	// the list of files we want out of the archive; any
	// directories will include all their contents unless
	// we return fs.SkipDir from our handler
	// (leave this nil to walk ALL files from the archive)
	dirMap := map[string]bool{}

	// not sure if this should be a syncmap, or if a map is ok?
	// not sure if the handler itself is invoked serially or if it
	// is concurrent?
	handler := func(ctx context.Context, archiveFile archiver.File) error {
		fileName := archiveFile.NameInArchive
		newPath := filepath.Join(dir, fileName)

		var err error

		if archiveFile.IsDir() {
			dirMap[newPath] = true

			return os.MkdirAll(newPath, archiveFile.Mode())
		} else {
			// check if we've seen the dir before, if not, we'll attempt to create
			// it in case its not there. This needs to be done as archive formats
			// do not necessarily always have the directory in order/present
			// eg zip dirs for quarto definitely are missing seemingly random dirs
			// when talking with charles about it, we were both unsure what might
			// be the reason, and assume its probably the powershell compress-archive
			// encantation, so rather than trying to go down that rabbit hole too far,
			// some additional checking here
			fileDir := filepath.Dir(newPath)
			_, seenDir := dirMap[fileDir]

			if !seenDir {
				dirMap[fileDir] = true
				// linux default for new directories is 777 and let the umask handle
				// if should have other controls
				err = os.MkdirAll(fileDir, 0777)
			}
		}

		if err != nil {
			return err
		}

		newFile, err := os.OpenFile(newPath, os.O_CREATE|os.O_WRONLY, archiveFile.Mode())
		if err != nil {
			return err
		}

		defer newFile.Close()

		// copy file data into tar writer
		archiveFileTemp, err := archiveFile.Open()
		if err != nil {
			return err
		}

		defer archiveFileTemp.Close()

		if _, err := io.Copy(newFile, archiveFileTemp); err != nil {
			return err
		}

		return err
	}

	// make sure the format is capable of extracting
	ex, ok := format.(archiver.Extractor)
	if !ok {
		return nil
	}

	return ex.Extract(context.Background(), input, nil, handler)
}

func (source *Source) extract() error {
	dlFile, err := os.Open(source.Path)
	if err != nil {
		fmt.Printf("%s❌ :: %sfailed to open source %s\n",
			string(constants.ColorBlue),
			string(constants.ColorYellow), source.Source)

		return err
	}

	err = source.Unarchive(dlFile, source.Output)
	if err != nil {
		fmt.Printf("%s❌ :: %sfailed to extract source %s\n",
			string(constants.ColorBlue),
			string(constants.ColorYellow), source.Path)

		log.Panic(err)
	}

	return err
}

func (source *Source) validate() error {
	var err error

	file, err := os.Open(source.Path)
	if err != nil {
		fmt.Printf("source: Failed to open file for hash\n")

		return err
	}

	defer file.Close()

	var hash hash.Hash

	if source.Hash == "SKIP" {
		fmt.Printf("%s:: %sSkip integrity check for %s%s\n",
			string(constants.ColorBlue),
			string(constants.ColorYellow),
			string(constants.ColorWhite),
			source.Source)
	}

	switch len(source.Hash) {
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

	if hexSum != source.Source {
		fmt.Printf("source: Hash verification failed for '%s'\n", source.Source)
	}

	return err
}

func (source *Source) Get() error {
	source.parsePath()

	var err error

	switch source.getType() {
	case "http":
		err = source.getURL("http")
	case "https":
		err = source.getURL("https")
	case "ftp":
		err = source.getURL("ftp")
	case "git":
		err = source.getURL("git")
	case "file":
		err = source.getPath()
	default:
		panic("utils: Unknown type")
	}

	if err != nil {
		return err
	}

	err = source.validate()
	if err != nil {
		return err
	}

	err = source.extract()

	if err != nil {
		return err
	}

	return err
}
