// Package git provides Git repository operations.
package git

import (
	"fmt"
	"net/url"
	"os"
	"strings"

	ggit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport/ssh"

	"github.com/M0Rf30/yap/v2/pkg/constants"
	"github.com/M0Rf30/yap/v2/pkg/files"
	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
	"github.com/M0Rf30/yap/v2/pkg/shell"
)

// Clone clones a Git repository from the given sourceItemURI to the specified dloadFilePath.
//
// Parameters:
// - sourceItemURI: the URI of the Git repository to clone.
// - dloadFilePath: the file path to clone the repository into.
// - sshPassword: the password for SSH authentication (optional).
// - referenceName: the reference name for the clone operation.
func Clone(dloadFilePath, sourceItemURI, sshPassword string,
	referenceName plumbing.ReferenceName,
) error {
	if dloadFilePath == "" {
		return fmt.Errorf("%s", i18n.T("errors.git.empty_download_path"))
	}
	// Start multiprinter for consistent output handling
	_, err := shell.MultiPrinter.Start()
	if err != nil {
		return err
	}

	// Create git progress writer for properly formatted git clone output
	gitProgressWriter := shell.NewGitProgressWriter(shell.MultiPrinter.Writer, "yap")

	defer func() {
		_ = gitProgressWriter.Close()
	}()

	cloneOptions := &ggit.CloneOptions{
		Progress: gitProgressWriter,
		URL:      sourceItemURI,
	}

	// If a specific branch or tag is requested, set it as the reference to clone
	if referenceName != "" {
		cloneOptions.ReferenceName = referenceName
		cloneOptions.SingleBranch = true
	}

	plainOpenOptions := &ggit.PlainOpenOptions{
		DetectDotGit:          true,
		EnableDotGitCommonDir: true,
	}

	logger.Info("cloning", "repo", sourceItemURI)

	if files.Exists(dloadFilePath) {
		return handleExistingRepo(dloadFilePath, referenceName, plainOpenOptions)
	}

	repo, err := ggit.PlainClone(dloadFilePath, false, cloneOptions)
	if err != nil && strings.Contains(err.Error(), "authentication required") {
		sourceURL, _ := url.Parse(sourceItemURI)
		sshKeyPath := os.Getenv("HOME") + "/.ssh/id_rsa"

		publicKey, err := ssh.NewPublicKeysFromFile("git", sshKeyPath, sshPassword)
		if err != nil {
			logger.Error(i18n.T("logger.clone.error.failed_to_load_ssh_1"))
			logger.Warn(i18n.T("logger.clone.warn.try_to_use_an_1"))

			return err
		}

		sshURL := fmt.Sprintf("%s@%s%s", constants.Git, sourceURL.Hostname(),
			strings.Replace(sourceURL.EscapedPath(), "/", ":", 1))
		cloneOptions.Auth = publicKey
		cloneOptions.URL = sshURL

		repo, err = ggit.PlainClone(dloadFilePath, false, cloneOptions)
		if err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	// After successful clone, ensure we're on the correct branch if specified
	if referenceName != "" && repo != nil {
		return checkoutReference(repo, referenceName)
	}

	return nil
}

// handleExistingRepo handles the case where a git repository already exists
// and potentially needs to checkout a specific branch or tag.
func handleExistingRepo(dloadFilePath string, referenceName plumbing.ReferenceName,
	plainOpenOptions *ggit.PlainOpenOptions,
) error {
	repo, err := ggit.PlainOpenWithOptions(dloadFilePath, plainOpenOptions)
	if err != nil {
		return err
	}

	if referenceName == "" {
		return nil
	}

	return checkoutReference(repo, referenceName)
}

// checkoutReference attempts to checkout the specified reference,
// fetching it first if necessary.
func checkoutReference(repo *ggit.Repository, referenceName plumbing.ReferenceName) error {
	workTree, err := repo.Worktree()
	if err != nil {
		return err
	}

	branchName := referenceName.Short()

	// First, try to fetch the latest changes from remote
	fetchOptions := &ggit.FetchOptions{}
	_ = repo.Fetch(fetchOptions) // Ignore fetch errors

	// Try to checkout the specified reference directly first
	checkoutOptions := &ggit.CheckoutOptions{
		Branch: referenceName,
	}

	err = workTree.Checkout(checkoutOptions)
	if err == nil {
		return nil // Success
	}

	// If direct checkout fails, the local branch might not exist
	// Try to create a local branch that tracks the remote branch
	remoteBranchRef := plumbing.NewRemoteReferenceName("origin", branchName)

	// Check if the remote branch exists
	remoteRef, err := repo.Reference(remoteBranchRef, true)
	if err != nil {
		return fmt.Errorf(i18n.T("errors.git.remote_branch_not_found"), branchName, err)
	}

	// Create a new local branch that tracks the remote branch
	localBranchRef := plumbing.NewBranchReferenceName(branchName)
	localRef := plumbing.NewHashReference(localBranchRef, remoteRef.Hash())

	err = repo.Storer.SetReference(localRef)
	if err != nil {
		return err
	}

	// Now checkout the newly created local branch
	checkoutOptions = &ggit.CheckoutOptions{
		Branch: localBranchRef,
	}

	err = workTree.Checkout(checkoutOptions)
	if err != nil {
		return err
	}

	return nil
}

// GetCommitHash returns the current git commit hash for the given directory.
// Returns empty string if not a git repository or on error.
func GetCommitHash(repoPath string) string {
	plainOpenOptions := &ggit.PlainOpenOptions{
		DetectDotGit:          true,
		EnableDotGitCommonDir: true,
	}

	repo, err := ggit.PlainOpenWithOptions(repoPath, plainOpenOptions)
	if err != nil {
		return ""
	}

	head, err := repo.Head()
	if err != nil {
		return ""
	}

	return head.Hash().String()
}
