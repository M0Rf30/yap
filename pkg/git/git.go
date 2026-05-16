// Package git provides Git repository operations.
package git

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	ggit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport/ssh"

	"github.com/M0Rf30/yap/v2/pkg/constants"
	"github.com/M0Rf30/yap/v2/pkg/errors"
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
// - commitHash: if non-empty, checkout this specific commit after cloning.
func Clone(dloadFilePath, sourceItemURI, sshPassword string,
	referenceName plumbing.ReferenceName, commitHash string,
) error {
	if dloadFilePath == "" {
		return errors.New(errors.ErrTypeValidation,
			i18n.T("errors.git.empty_download_path")).
			WithOperation("Clone")
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

	// After successful clone, checkout the requested reference or commit
	if commitHash != "" && repo != nil {
		return checkoutCommit(repo, commitHash)
	}

	if referenceName != "" && repo != nil {
		return checkoutReference(repo, referenceName)
	}

	return nil
}

// ExtractFromBare creates a working copy from a bare/mirror git repository,
// then checks out the requested reference. This mirrors makepkg's extract_git()
// behavior: open the bare cache, init a new repo in srcdir, fetch objects, and
// checkout the requested ref — all using pure Go (no git CLI required).
func ExtractFromBare(barePath, srcDir, refKey, refValue string) error {
	repoName := filepath.Base(barePath)
	workDir := filepath.Join(srcDir, repoName)

	logger.Info("creating working copy from bare repository",
		"bare", barePath, "workdir", workDir)

	// Remove any existing symlink or stale directory at the target path
	// (e.g. a symlink from a previous symlinkSources call pointing back
	// to the bare cache).
	if err := os.RemoveAll(workDir); err != nil {
		return err
	}

	// Open the bare repo to read its objects and references
	bareRepo, err := ggit.PlainOpen(barePath)
	if err != nil {
		return err
	}

	// Resolve the target hash from the requested ref
	targetHash, err := resolveRef(bareRepo, refKey, refValue)
	if err != nil {
		return err
	}

	// Create a working copy by cloning from the bare repo using the
	// in-memory dotgit filesystem (pure Go, no git CLI needed).
	// go-git's PlainClone with a file:// URL still shells out to git,
	// so we init + manually set up the repo instead.
	repo, err := ggit.PlainInit(workDir, false)
	if err != nil {
		return err
	}

	// Copy all objects from the bare repo's object store
	bareStorer := bareRepo.Storer
	workStorer := repo.Storer

	refs, err := bareStorer.IterReferences()
	if err != nil {
		return err
	}

	err = refs.ForEach(func(ref *plumbing.Reference) error {
		return workStorer.SetReference(ref)
	})
	if err != nil {
		return err
	}

	objIter, err := bareRepo.Objects()
	if err != nil {
		return err
	}

	err = objIter.ForEach(func(obj object.Object) error {
		eo, encErr := bareStorer.EncodedObject(obj.Type(), obj.ID())
		if encErr != nil {
			return encErr
		}

		_, setErr := workStorer.SetEncodedObject(eo)

		return setErr
	})
	if err != nil {
		return err
	}

	workTree, err := repo.Worktree()
	if err != nil {
		return err
	}

	return workTree.Checkout(&ggit.CheckoutOptions{
		Hash: targetHash,
	})
}

// resolveRef resolves a PKGBUILD fragment (commit, tag, branch) to a commit hash.
func resolveRef(repo *ggit.Repository, refKey, refValue string) (plumbing.Hash, error) {
	switch refKey {
	case "commit":
		return plumbing.NewHash(refValue), nil
	case "tag":
		ref, err := repo.Tag(refValue)
		if err != nil {
			return plumbing.ZeroHash, err
		}

		return ref.Hash(), nil
	case "branch":
		ref, err := repo.Reference(plumbing.NewBranchReferenceName(refValue), true)
		if err != nil {
			return plumbing.ZeroHash, err
		}

		return ref.Hash(), nil
	default:
		head, err := repo.Head()
		if err != nil {
			return plumbing.ZeroHash, err
		}

		return head.Hash(), nil
	}
}

// IsBareRepo checks if the given path is a bare git repository
// (contains HEAD file at the top level with no .git subdirectory).
func IsBareRepo(path string) bool {
	// A bare repo has HEAD at the top level, not inside .git/
	if _, err := os.Stat(filepath.Join(path, "HEAD")); err != nil {
		return false
	}

	if _, err := os.Stat(filepath.Join(path, ".git")); err == nil {
		return false // Has .git dir → not bare
	}

	return true
}

// checkoutCommit checks out a specific commit hash.
func checkoutCommit(repo *ggit.Repository, hash string) error {
	workTree, err := repo.Worktree()
	if err != nil {
		return err
	}

	return workTree.Checkout(&ggit.CheckoutOptions{
		Hash: plumbing.NewHash(hash),
	})
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
		return errors.Wrap(err, errors.ErrTypeBuild,
			fmt.Sprintf(i18n.T("errors.git.remote_branch_not_found"), branchName)).
			WithOperation("checkoutBranch")
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
