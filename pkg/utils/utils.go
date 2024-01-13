package utils

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/M0Rf30/yap/pkg/constants"
	"github.com/cavaliergopher/grab/v3"
	ggit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport/ssh"
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
func Download(destination, uri string) error {
	// create client
	client := grab.NewClient()
	req, err := grab.NewRequest(destination, uri)

	if err != nil {
		return err
	}

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
	if err = resp.Err(); err != nil {
		fmt.Printf("%s❌ :: %sdownload failed: %s\n",
			string(constants.ColorBlue),
			string(constants.ColorYellow),
			string(constants.ColorWhite))

		return err
	}

	defer ticker.Stop()

	fmt.Printf("\n%s📥 :: %sDownload saved: %s\t%v\n",
		string(constants.ColorBlue),
		string(constants.ColorYellow),
		string(constants.ColorWhite),
		destination,
	)

	return err
}

// GitClone clones a Git repository from the given sourceItemURI to the specified dloadFilePath.
//
// Parameters:
// - sourceItemURI: the URI of the Git repository to clone.
// - dloadFilePath: the file path to clone the repository into.
// - sshPassword: the password for SSH authentication (optional).
// - referenceName: the reference name for the clone operation.
func GitClone(sourceItemURI, dloadFilePath, sshPassword string,
	referenceName plumbing.ReferenceName) error {
	normalizedURI := strings.TrimPrefix(sourceItemURI, constants.Git+"+")
	cloneOptions := &ggit.CloneOptions{
		Progress:      os.Stdout,
		ReferenceName: referenceName,
		URL:           normalizedURI,
	}

	if Exists(dloadFilePath) {
		_, err := ggit.PlainOpenWithOptions(dloadFilePath, &ggit.PlainOpenOptions{
			DetectDotGit:          true,
			EnableDotGitCommonDir: true,
		})
		if err != nil {
			return err
		}
	}

	_, err := ggit.PlainClone(dloadFilePath, false, cloneOptions)
	if err != nil && err.Error() == "authentication required" {
		sourceURL, _ := url.Parse(normalizedURI)
		sshKeyPath := os.Getenv("HOME") + "/.ssh/id_rsa"
		publicKey, err := ssh.NewPublicKeysFromFile("git", sshKeyPath, sshPassword)

		if err != nil {
			fmt.Printf("%s❌ :: %sfailed to load ssh key%s\n",
				string(constants.ColorBlue),
				string(constants.ColorYellow),
				string(constants.ColorWhite))
			fmt.Printf("%s:: %sTry to use an ssh-password with the -p flag%s\n",
				string(constants.ColorBlue),
				string(constants.ColorYellow),
				string(constants.ColorWhite))

			return err
		}

		sshURL := constants.Git + "@" + sourceURL.Hostname() + strings.Replace(sourceURL.EscapedPath(), "/", ":", 1)
		cloneOptions.Auth = publicKey
		cloneOptions.URL = sshURL
		_, err = ggit.PlainClone(dloadFilePath, false, cloneOptions)

		if err != nil {
			return err
		}
	}

	return err
}

// GOSetup sets up the Go environment.
//
// It checks if Go is installed and if not, it downloads and installs it.
// The function takes no parameters and does not return anything.
func GOSetup() {
	if CheckGO() {
		return
	}

	err := Download(goArchivePath, constants.GoArchiveURL)
	if err != nil {
		log.Panic(err)
	}

	dlFile, err := os.Open(goArchivePath)
	if err != nil {
		fmt.Printf("%s❌ :: %sfailed to open %s\n",
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
