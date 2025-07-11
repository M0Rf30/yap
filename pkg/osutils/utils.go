package osutils

import (
	"context"
	"io"
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
	"github.com/mholt/archives"
	"github.com/pkg/errors"
	"github.com/pterm/pterm"
	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"
)

const (
	goArchivePath = "/tmp/go.tar.gz"
	goExecutable  = "/usr/bin/go"
)

var (
	// Logger is the default logger with information level logging.
	// It writes to the MultiPrinter's writer.
	Logger = pterm.DefaultLogger.WithLevel(pterm.LogLevelInfo).WithWriter(MultiPrinter.Writer)
	// MultiPrinter is the default multi printer.
	MultiPrinter = pterm.DefaultMultiPrinter
)

// CheckGO checks if the GO executable is already installed.
//
// It does not take any parameters.
// It returns a boolean value indicating whether the GO executable is already installed.
func CheckGO() bool {
	_, err := os.Stat(goExecutable)
	if err == nil {
		Logger.Info("go is already installed")

		return true
	}

	return false
}

// CreateTarZst creates a compressed tar.zst archive from the specified source
// directory. It takes the source directory and the output file path as
// arguments and returns an error if any occurs.
func CreateTarZst(sourceDir, outputFile string, formatGNU bool) error {
	ctx := context.TODO()
	options := &archives.FromDiskOptions{
		FollowSymlinks: false,
	}

	// Retrieve the list of files from the source directory on disk.
	// The map specifies that the files should be read from the sourceDir
	// and the output path in the archive should be empty.
	files, err := archives.FilesFromDisk(ctx, options, map[string]string{
		sourceDir + string(os.PathSeparator): "",
	})
	if err != nil {
		return err
	}

	cleanFilePath := filepath.Clean(outputFile)

	out, err := os.Create(cleanFilePath)
	if err != nil {
		return err
	}
	defer out.Close()

	format := archives.CompressedArchive{
		Compression: archives.Zstd{},
		Archival: archives.Tar{
			FormatGNU: formatGNU,
			Uid:       0,
			Gid:       0,
			Uname:     "root",
			Gname:     "root",
		},
	}

	return format.Archive(ctx, out, files)
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
		return errors.Errorf("download failed %s", err)
	}

	resp := client.Do(req)
	if resp.HTTPResponse == nil {
		Logger.Fatal("download failed: no response", Logger.Args("error", resp.Err()))
	}

	// start download
	Logger.Info("downloading", Logger.Args("url", req.URL()))
	Logger.Info("response", Logger.Args("status", resp.HTTPResponse.Status))
	Logger.Info("download in progress")

	// start UI loop
	ticker := time.NewTicker(500 * time.Millisecond)
	progressBar, _ := pterm.DefaultProgressbar.Start()

Loop:
	for {
		select {
		case <-resp.Done:
			progressBar.Current = 100

			_, err = progressBar.Stop()
			if err != nil {
				Logger.Fatal("failed to stop progress bar", Logger.Args("error", err))
			}

			ticker.Stop()
			Logger.Info("download completed", Logger.Args("path", destination))

			break Loop

		case <-ticker.C:
			progressBar.Current = int(100 * resp.Progress())
		}
	}

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
		Progress:      MultiPrinter.Writer,
		ReferenceName: referenceName,
		URL:           sourceItemURI,
	}

	plainOpenOptions := &ggit.PlainOpenOptions{
		DetectDotGit:          true,
		EnableDotGitCommonDir: true,
	}

	Logger.Info("cloning",
		Logger.Args("repo", sourceItemURI))

	if Exists(dloadFilePath) {
		_, err := ggit.PlainOpenWithOptions(dloadFilePath, plainOpenOptions)
		if err != nil {
			return err
		}
	}

	_, err := ggit.PlainClone(dloadFilePath, false, cloneOptions)
	if err != nil && strings.Contains(err.Error(), "authentication required") {
		sourceURL, _ := url.Parse(sourceItemURI)
		sshKeyPath := os.Getenv("HOME") + "/.ssh/id_rsa"

		publicKey, err := ssh.NewPublicKeysFromFile("git", sshKeyPath, sshPassword)
		if err != nil {
			Logger.Error("failed to load ssh key")
			Logger.Warn("try to use an ssh-password with the -p")

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
		Logger.Fatal("download failed",
			Logger.Args("error", err))
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

	err = os.RemoveAll(goArchivePath)
	if err != nil {
		return err
	}

	Logger.Info("go successfully installed")

	return err
}

// PullContainers pulls the specified container image.
//
// distro: the name of the container image to pull.
// error: returns an error if the container image cannot be pulled.
func PullContainers(distro string) error {
	var containerApp string

	if Exists("/usr/bin/podman") {
		containerApp = "/usr/bin/podman"
	} else if Exists("/usr/bin/docker") {
		containerApp = "/usr/bin/docker"
	} else {
		return errors.Errorf("no container application found")
	}

	args := []string{
		"pull",
		constants.DockerOrg + distro,
	}

	_, err := os.Stat(containerApp)
	if err == nil {
		return Exec(false, "", containerApp, args...)
	}

	return nil
}

// RunScript runs a shell script.
//
// It takes a string parameter `cmds` which represents the shell script to be executed.
// The function returns an error if there was an issue running the script.
func RunScript(cmds string) error {
	script, _ := syntax.NewParser().Parse(strings.NewReader(cmds), "")

	_, err := MultiPrinter.Start()
	if err != nil {
		return err
	}

	runner, _ := interp.New(
		interp.Env(expand.ListEnviron(os.Environ()...)),
		interp.StdIO(nil, MultiPrinter.Writer, MultiPrinter.Writer),
	)

	err = runner.Run(context.TODO(), script)
	if err != nil {
		return err
	}

	return err
}

// Unarchive is a function that takes a source file and a destination. It opens
// the source archive file, identifies its format, and extracts it to the
// destination.
//
// Returns an error if there was a problem extracting the files.
func Unarchive(source, destination string) error {
	ctx := context.TODO()

	// Open the source archive file
	archive, err := Open(source)
	if err != nil {
		return err
	}

	// Identify the archive file's format
	format, archiveReader, _ := archives.Identify(ctx, "", archive)

	dirMap := make(map[string]bool)

	// Check if the format is an extractor. If not, skip the archive file.
	extractor, ok := format.(archives.Extractor)

	if !ok {
		return nil
	}

	return extractor.Extract(ctx, archiveReader, func(_ context.Context, archiveFile archives.FileInfo) error {
		fileName := archiveFile.NameInArchive
		newPath := filepath.Join(destination, fileName)

		if archiveFile.IsDir() {
			dirMap[newPath] = true

			return os.MkdirAll(newPath, 0o755) // #nosec
		}

		fileDir := filepath.Dir(newPath)
		_, seenDir := dirMap[fileDir]

		if !seenDir {
			dirMap[fileDir] = true

			_ = os.MkdirAll(fileDir, 0o755) // #nosec
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
	})
}
