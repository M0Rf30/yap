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
	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"
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
func GitClone(dloadFilePath, sourceItemURI, sshPassword string,
	referenceName plumbing.ReferenceName) error {
	cloneOptions := &ggit.CloneOptions{
		Progress:      os.Stdout,
		ReferenceName: referenceName,
		URL:           sourceItemURI,
	}

	// start download
	fmt.Printf("%s📥 :: %sCloning %s\t%v\n",
		string(constants.ColorBlue),
		string(constants.ColorYellow),
		string(constants.ColorWhite),
		sourceItemURI,
	)

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
		sourceURL, _ := url.Parse(sourceItemURI)
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

		sshURL := constants.Git + "@" + sourceURL.Hostname() +
			strings.Replace(sourceURL.EscapedPath(), "/", ":", 1)
		cloneOptions.Auth = publicKey
		cloneOptions.URL = sshURL
		_, err = ggit.PlainClone(dloadFilePath, false, cloneOptions)

		if err != nil {
			return err
		}
	}

	return nil
}

// GOSetup sets up the Go environment.
//
// It checks if Go is installed and if not, it downloads and installs it.
// The function takes no parameters and does not return anything.
func GOSetup() error {
	if CheckGO() {
		return nil
	}

	err := Download(goArchivePath, constants.GoArchiveURL)
	if err != nil {
		log.Panic(err)
	}

	err = Unarchive(goArchivePath, "/usr/lib")
	if err != nil {
		return err
	}

	err = os.Symlink("/usr/lib/go/bin/go", goExecutable)
	if err != nil {
		return err
	}

	err = os.Symlink("/usr/lib/go/bin/gofmt", "/usr/bin/gofmt")
	if err != nil {
		return err
	}

	err = RemoveAll(goArchivePath)
	if err != nil {
		return err
	}

	fmt.Printf("%s🪛 :: %sGO successfully installed%s\n",
		string(constants.ColorBlue),
		string(constants.ColorYellow),
		string(constants.ColorWhite))

	return err
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

	if _, err := os.Stat(containerApp); err == nil {
		return Exec("", containerApp, args...)
	}

	return nil
}

// RunScript runs a shell script.
//
// It takes a string parameter `cmds` which represents the shell script to be executed.
// The function returns an error if there was an issue running the script.
func RunScript(cmds string) error {
	script, _ := syntax.NewParser().Parse(strings.NewReader(cmds), "")

	runner, _ := interp.New(
		interp.Env(expand.ListEnviron(os.Environ()...)),
		interp.StdIO(nil, os.Stdout, os.Stdout),
	)

	return runner.Run(context.TODO(), script)
}

// Unarchive is a function that takes a source file and a destination. It opens
// the source archive file, identifies its format, and extracts it to the
// destination.
//
// Returns an error if there was a problem extracting the files.
func Unarchive(source, destination string) error {
	// Open the source archive file
	archiveFile, err := Open(source)
	if err != nil {
		return err
	}

	// Identify the archive file's format
	format, archiveReader, _ := archiver.Identify("", archiveFile)

	// Check if the format is an extractor. If not, skip the archive file.
	extractor, ok := format.(archiver.Extractor)

	if !ok {
		return nil
	}

	handler := func(_ context.Context, archiveFile archiver.File) error {
		fileName := archiveFile.NameInArchive
		newPath := filepath.Join(destination, fileName)

		if archiveFile.IsDir() {
			return os.MkdirAll(newPath, archiveFile.Mode())
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

	return extractor.Extract(context.Background(),
		archiveReader,
		nil,
		handler)
}
