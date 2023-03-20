package utils

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/M0Rf30/yap/constants"
	"github.com/cavaliergopher/grab/v3"
	"github.com/mholt/archiver/v4"
)

const goArchivePath = "/tmp/go.tar.gz"
const goExecutable = "/usr/bin/go"

func CheckGO() bool {
	if _, err := os.Stat(goExecutable); err == nil {
		fmt.Printf("%s🪛 :: %sGO is already installed%s\n",
			string(constants.ColorBlue),
			string(constants.ColorYellow),
			string(constants.ColorWhite))

		return true
	}

	return false
}

func GOSetup() {
	if CheckGO() {
		return
	}

	Download(goArchivePath, constants.GoArchiveURL)

	dlFile, err := os.Open(goArchivePath)
	if err != nil {
		fmt.Printf("%s❌ :: %sfailed to open %s\n",
			string(constants.ColorBlue),
			string(constants.ColorYellow), goArchivePath)

		os.Exit(1)
	}

	err = Unarchive(dlFile, "/usr/lib")
	if err != nil {
		log.Panic(err)
	}

	err = os.Symlink("/usr/lib/go/bin/go", goExecutable)
	if err != nil {
		log.Panic(err)
	}

	err = os.Symlink("/usr/lib/go/bin/gofmt", "/usr/bin/gofmt")
	if err != nil {
		log.Panic(err)
	}

	err = os.Symlink("/usr/share/doc/go", "/usr/lib/go/doc")
	if err != nil {
		log.Panic(err)
	}

	fmt.Printf("%s🪛 :: %sGO successfully installed%s\n",
		string(constants.ColorBlue),
		string(constants.ColorYellow),
		string(constants.ColorWhite))
}

func Download(destination string, URL string) {
	// create client
	client := grab.NewClient()
	req, _ := grab.NewRequest(destination, URL)

	// start download
	fmt.Printf("%s📥 :: %sDownloading %s\t%v\n",
		string(constants.ColorBlue),
		string(constants.ColorYellow),
		string(constants.ColorWhite),
		req.URL(),
	)

	resp := client.Do(req)
	fmt.Printf("%s📥 :: %sResponse: %s\t%v\n",
		string(constants.ColorBlue),
		string(constants.ColorYellow),
		string(constants.ColorWhite),
		resp.HTTPResponse.Status,
	)

	fmt.Printf("%s📥 :: %sDownload in progress: %s\n",
		string(constants.ColorBlue),
		string(constants.ColorYellow),
		string(constants.ColorWhite),
	)

	// start UI loop
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

Loop:
	for {
		select {
		case <-ticker.C:
			fmt.Printf("\033[2K\r%d.1 of %d.1 MB | %.2f of 100",
				resp.BytesComplete()/1024/1024,
				resp.Size()/1024/1024,
				100*resp.Progress(),
			)

		case <-resp.Done:
			// download is complete
			break Loop
		}
	}

	// check for errors
	if err := resp.Err(); err != nil {
		fmt.Printf("%s❌ :: %sDownload failed: %s\n%v\n",
			string(constants.ColorBlue),
			string(constants.ColorYellow),
			string(constants.ColorWhite),
			err,
		)

		os.Exit(1)
	}

	fmt.Printf("%s📥 :: %sDownload saved: %s\t%v\n",
		string(constants.ColorBlue),
		string(constants.ColorYellow),
		string(constants.ColorWhite),
		resp.Filename,
	)
}

func PullContainers(target string) error {
	containerApp := "/usr/bin/docker"
	args := []string{
		"pull",
		constants.DockerOrg + target,
	}

	var err error

	if _, err = os.Stat(containerApp); err == nil {
		err = Exec("", containerApp, args...)
	} else {
		err = Exec("", "podman", args...)
	}

	if err != nil {
		return err
	}

	return err
}

func Unarchive(input io.Reader, dir string) error {
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
		}

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
