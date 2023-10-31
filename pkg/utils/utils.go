package utils

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/M0Rf30/yap/pkg/constants"
	"github.com/cavaliergopher/grab/v3"
	"github.com/mholt/archiver/v4"
)

const goArchivePath = "/tmp/go.tar.gz"
const goExecutable = "/usr/bin/go"

// CheckGO checks if the GO executable is already installed.
//
// It does not take any parameters.
// It returns a boolean value indicating whether the GO executable is already installed.
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

// Download downloads a file from the given URL and saves it to the specified destination.
//
// Parameters:
// - destination: the path where the downloaded file will be saved.
// - url: the URL of the file to download.
func Download(destination, url string) {
	// create client
	client := grab.NewClient()
	req, _ := grab.NewRequest(destination, url)

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

Loop:
	for {
		select {
		case <-ticker.C:
			fmt.Printf("\033[2K\r%d.1 of %d.1 MB | %.2f %%",
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
		log.Fatalf("%s❌ :: %sdownload failed: %s\n%v\n",
			string(constants.ColorBlue),
			string(constants.ColorYellow),
			string(constants.ColorWhite),
			err,
		)
	}

	defer ticker.Stop()

	fmt.Printf("\n%s📥 :: %sDownload saved: %s\t%v\n",
		string(constants.ColorBlue),
		string(constants.ColorYellow),
		string(constants.ColorWhite),
		destination,
	)
}

// GOSetup sets up the Go environment.
//
// It checks if Go is installed and if not, it downloads and installs it.
// The function takes no parameters and does not return anything.
func GOSetup() {
	if CheckGO() {
		return
	}

	Download(goArchivePath, constants.GoArchiveURL)

	dlFile, err := os.Open(goArchivePath)
	if err != nil {
		log.Fatalf("%s❌ :: %sfailed to open %s\n",
			string(constants.ColorBlue),
			string(constants.ColorYellow), goArchivePath)
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

	fmt.Printf("%s🪛 :: %sGO successfully installed%s\n",
		string(constants.ColorBlue),
		string(constants.ColorYellow),
		string(constants.ColorWhite))
}

// PullContainers pulls the specified container image.
//
// target: the name of the container image to pull.
// error: returns an error if the container image cannot be pulled.
func PullContainers(target string) error {
	containerApp := "/usr/bin/podman"
	args := []string{
		"pull",
		constants.DockerOrg + target,
	}

	var err error

	if _, err = os.Stat(containerApp); err == nil {
		err = Exec("", containerApp, args...)
	}

	if err != nil {
		return err
	}

	return nil
}

// Unarchive extracts files from an archive and saves them to a destination directory.
//
// archiveReader is an io.Reader that represents the archive file.
// destination is the path to the directory where the files will be saved.
// Returns an error if there was a problem extracting the files.
func Unarchive(archiveReader io.Reader, destination string) error {
	format, archiveReader, _ := archiver.Identify("", archiveReader)

	dirMap := make(map[string]bool)

	handler := func(ctx context.Context, archiveFile archiver.File) error {
		fileName := archiveFile.NameInArchive
		newPath := filepath.Join(destination, fileName)

		if archiveFile.IsDir() {
			dirMap[newPath] = true

			return os.MkdirAll(newPath, archiveFile.Mode())
		}

		fileDir := filepath.Dir(newPath)
		_, seenDir := dirMap[fileDir]

		if !seenDir {
			dirMap[fileDir] = true
			// #nosec
			return os.MkdirAll(fileDir, 0777)
		}

		cleanNewPath := filepath.Clean(newPath)

		newFile, err := os.OpenFile(cleanNewPath,
			os.O_CREATE|os.O_WRONLY,
			archiveFile.Mode())
		if err != nil {
			return err
		}
		defer newFile.Close()

		archiveFileTemp, err := archiveFile.Open()
		if err != nil {
			return err
		}
		defer archiveFileTemp.Close()

		_, err = io.Copy(newFile, archiveFileTemp)

		return err
	}

	ex, ok := format.(archiver.Extractor)
	if !ok {
		return nil
	}

	return ex.Extract(context.Background(),
		archiveReader,
		nil,
		handler)
}
