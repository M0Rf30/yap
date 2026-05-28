package git_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	ggit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/M0Rf30/yap/v2/pkg/git"
)

// initRepoWithCommit creates a real git repo in dir and makes one commit.
// Returns the repo and the commit hash string.
func initRepoWithCommit(t *testing.T, dir string) (repo *ggit.Repository, commitHash string) {
	t.Helper()

	repo, err := ggit.PlainInit(dir, false)
	require.NoError(t, err, "PlainInit should succeed")

	w, err := repo.Worktree()
	require.NoError(t, err, "Worktree should succeed")

	testFile := filepath.Join(dir, "test.txt")
	err = os.WriteFile(testFile, []byte("hello yap"), 0o644)
	require.NoError(t, err, "WriteFile should succeed")

	_, err = w.Add("test.txt")
	require.NoError(t, err, "Add should succeed")

	hash, err := w.Commit("initial commit", &ggit.CommitOptions{
		Author: &object.Signature{
			Name:  "yap-test",
			Email: "yap@test.local",
			When:  time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		},
	})
	require.NoError(t, err, "Commit should succeed")

	return repo, hash.String()
}

// initBareRepo creates a bare git repo in dir.
func initBareRepo(t *testing.T, dir string) {
	t.Helper()

	_, err := ggit.PlainInit(dir, true)
	require.NoError(t, err, "PlainInit (bare) should succeed")
}

// ─── IsBareRepo ──────────────────────────────────────────────────────────────

func TestIsBareRepo_NonExistentPath(t *testing.T) {
	result := git.IsBareRepo("/tmp/yap-test-nonexistent-path-xyz-12345")
	assert.False(t, result, "non-existent path should not be a bare repo")
}

func TestIsBareRepo_EmptyDirectory(t *testing.T) {
	dir := t.TempDir()
	result := git.IsBareRepo(dir)
	assert.False(t, result, "empty directory should not be a bare repo")
}

func TestIsBareRepo_RegularDirectory(t *testing.T) {
	dir := t.TempDir()
	// Create some files but no HEAD
	err := os.WriteFile(filepath.Join(dir, "somefile.txt"), []byte("data"), 0o644)
	require.NoError(t, err)

	result := git.IsBareRepo(dir)
	assert.False(t, result, "regular directory without HEAD should not be a bare repo")
}

func TestIsBareRepo_NonBareRepo(t *testing.T) {
	dir := t.TempDir()
	initRepoWithCommit(t, dir)

	result := git.IsBareRepo(dir)
	assert.False(t, result, "non-bare repo (has .git/) should return false")
}

func TestIsBareRepo_BareRepo(t *testing.T) {
	dir := t.TempDir()
	initBareRepo(t, dir)

	result := git.IsBareRepo(dir)
	assert.True(t, result, "bare repo (HEAD at top level, no .git/) should return true")
}

func TestIsBareRepo_DirectoryWithHeadButAlsoGitDir(t *testing.T) {
	// Simulate a directory that has both HEAD and .git/ — should NOT be bare
	dir := t.TempDir()

	err := os.WriteFile(filepath.Join(dir, "HEAD"), []byte("ref: refs/heads/main\n"), 0o644)
	require.NoError(t, err)

	err = os.MkdirAll(filepath.Join(dir, ".git"), 0o755)
	require.NoError(t, err)

	result := git.IsBareRepo(dir)
	assert.False(t, result, "directory with both HEAD and .git/ should not be bare")
}

func TestIsBareRepo_DirectoryWithHeadOnly(t *testing.T) {
	// Simulate a directory that has HEAD at top level but no .git/ — looks bare
	dir := t.TempDir()

	err := os.WriteFile(filepath.Join(dir, "HEAD"), []byte("ref: refs/heads/main\n"), 0o644)
	require.NoError(t, err)

	result := git.IsBareRepo(dir)
	assert.True(t, result, "directory with HEAD at top level and no .git/ should be bare")
}

// ─── GetCommitHash ───────────────────────────────────────────────────────────

func TestGetCommitHash_NonExistentPath(t *testing.T) {
	hash := git.GetCommitHash("/tmp/yap-test-nonexistent-path-xyz-12345")
	assert.Empty(t, hash, "non-existent path should return empty string")
}

func TestGetCommitHash_EmptyDirectory(t *testing.T) {
	dir := t.TempDir()
	hash := git.GetCommitHash(dir)
	assert.Empty(t, hash, "empty (non-git) directory should return empty string")
}

func TestGetCommitHash_IncompleteGitDir(t *testing.T) {
	// Has .git/ but no valid objects/HEAD — not a real repo
	dir := t.TempDir()
	err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755)
	require.NoError(t, err)

	hash := git.GetCommitHash(dir)
	assert.Empty(t, hash, "incomplete .git directory should return empty string")
}

func TestGetCommitHash_ValidRepo(t *testing.T) {
	dir := t.TempDir()
	_, expectedHash := initRepoWithCommit(t, dir)

	hash := git.GetCommitHash(dir)
	require.NotEmpty(t, hash, "valid git repo should return a non-empty hash")
	assert.Equal(t, expectedHash, hash, "returned hash should match the HEAD commit")
}

func TestGetCommitHash_ValidRepoMultipleCommits(t *testing.T) {
	dir := t.TempDir()
	repo, _ := initRepoWithCommit(t, dir)

	// Make a second commit
	w, err := repo.Worktree()
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(dir, "second.txt"), []byte("second"), 0o644)
	require.NoError(t, err)

	_, err = w.Add("second.txt")
	require.NoError(t, err)

	secondHash, err := w.Commit("second commit", &ggit.CommitOptions{
		Author: &object.Signature{
			Name:  "yap-test",
			Email: "yap@test.local",
			When:  time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
		},
	})
	require.NoError(t, err)

	hash := git.GetCommitHash(dir)
	require.NotEmpty(t, hash)
	assert.Equal(t, secondHash.String(), hash, "should return the latest (HEAD) commit hash")
}

func TestGetCommitHash_HashFormat(t *testing.T) {
	dir := t.TempDir()
	initRepoWithCommit(t, dir)

	hash := git.GetCommitHash(dir)
	require.NotEmpty(t, hash)
	// SHA-1 hashes are 40 hex characters
	assert.Len(t, hash, 40, "commit hash should be 40 characters (SHA-1 hex)")

	for _, c := range hash {
		assert.True(t, (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f'),
			"hash character %q should be lowercase hex", c)
	}
}

func TestGetCommitHash_SubdirectoryOfRepo(t *testing.T) {
	dir := t.TempDir()
	_, expectedHash := initRepoWithCommit(t, dir)

	// Create a subdirectory inside the repo
	subDir := filepath.Join(dir, "subdir")
	err := os.MkdirAll(subDir, 0o755)
	require.NoError(t, err)

	// GetCommitHash uses DetectDotGit=true, so it should walk up and find the repo
	hash := git.GetCommitHash(subDir)
	require.NotEmpty(t, hash, "subdirectory of a git repo should resolve the commit hash")
	assert.Equal(t, expectedHash, hash)
}

// ─── Clone error paths ───────────────────────────────────────────────────────

func TestClone_EmptyDownloadPath(t *testing.T) {
	err := git.Clone("", "https://github.com/example/repo.git", "", "", "")
	assert.Error(t, err, "empty download path should return an error")
}

func TestClone_InvalidURL(t *testing.T) {
	dir := t.TempDir()
	clonePath := filepath.Join(dir, "repo")

	err := git.Clone(clonePath, "not-a-valid-url://???", "", "", "")
	assert.Error(t, err, "invalid URL should return an error")
}

func TestClone_ExistingNonGitDirectory(t *testing.T) {
	dir := t.TempDir()
	// The directory already exists but is not a git repo
	err := git.Clone(dir, "https://github.com/example/repo.git", "", "", "")
	assert.Error(t, err, "existing non-git directory should return an error")
}

func TestClone_ExistingFileAsTarget(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "not-a-dir")

	err := os.WriteFile(filePath, []byte("data"), 0o644)
	require.NoError(t, err)

	err = git.Clone(filePath, "https://github.com/example/repo.git", "", "", "")
	assert.Error(t, err, "existing file as clone target should return an error")
}

// ─── ExtractFromBare error paths ─────────────────────────────────────────────

func TestExtractFromBare_NonExistentBarePath(t *testing.T) {
	srcDir := t.TempDir()
	err := git.ExtractFromBare("/tmp/yap-nonexistent-bare-xyz", srcDir, "", "")
	assert.Error(t, err, "non-existent bare path should return an error")
}

func TestExtractFromBare_NonBareDirectory(t *testing.T) {
	dir := t.TempDir()
	srcDir := t.TempDir()

	// dir is a regular directory, not a bare repo
	err := git.ExtractFromBare(dir, srcDir, "", "")
	assert.Error(t, err, "non-bare directory should return an error")
}

func TestExtractFromBare_ValidBareRepo(t *testing.T) {
	// Create a non-bare repo with a commit, then clone it as bare
	srcRepo := t.TempDir()
	initRepoWithCommit(t, srcRepo)

	bareDir := t.TempDir()
	_, err := ggit.PlainClone(bareDir, true, &ggit.CloneOptions{
		URL: srcRepo,
	})
	require.NoError(t, err, "cloning as bare should succeed")

	workDir := t.TempDir()
	err = git.ExtractFromBare(bareDir, workDir, "", "")
	assert.NoError(t, err, "ExtractFromBare from a valid bare repo should succeed")

	// The working copy should contain the committed file
	repoName := filepath.Base(bareDir)
	extractedFile := filepath.Join(workDir, repoName, "test.txt")
	_, statErr := os.Stat(extractedFile)
	assert.NoError(t, statErr, "extracted working copy should contain committed files")
}

// ─── plumbing reference helpers (no network needed) ──────────────────────────

func TestPlumbingBranchReference(t *testing.T) {
	cases := []struct {
		branch   string
		expected string
	}{
		{"main", "refs/heads/main"},
		{"develop", "refs/heads/develop"},
		{"feature/foo", "refs/heads/feature/foo"},
	}

	for _, tc := range cases {
		ref := plumbing.NewBranchReferenceName(tc.branch)
		assert.Equal(t, tc.expected, ref.String())
		assert.Equal(t, tc.branch, ref.Short())
	}
}

func TestPlumbingRemoteReference(t *testing.T) {
	ref := plumbing.NewRemoteReferenceName("origin", "main")
	assert.Equal(t, "refs/remotes/origin/main", ref.String())
}

// ─── resolveRef ──────────────────────────────────────────────────────────────

func TestResolveRef_CommitHash(t *testing.T) {
	dir := t.TempDir()
	_, expectedHash := initRepoWithCommit(t, dir)

	// resolveRef is unexported, so we test it indirectly via ExtractFromBare
	// which calls it internally. We'll test it more directly by creating
	// a helper that wraps it.
	// For now, test via ExtractFromBare with commit refKey
	bareDir := t.TempDir()
	_, err := ggit.PlainClone(bareDir, true, &ggit.CloneOptions{
		URL: dir,
	})
	require.NoError(t, err)

	workDir := t.TempDir()
	err = git.ExtractFromBare(bareDir, workDir, "commit", expectedHash)
	assert.NoError(t, err, "ExtractFromBare with commit hash should succeed")

	// Verify the working copy was created and checked out
	repoName := filepath.Base(bareDir)
	extractedFile := filepath.Join(workDir, repoName, "test.txt")
	_, statErr := os.Stat(extractedFile)
	assert.NoError(t, statErr, "extracted working copy should contain files from the commit")
}

func TestResolveRef_Tag(t *testing.T) {
	dir := t.TempDir()
	tagRepo, _ := initRepoWithCommit(t, dir)

	// Create a tag pointing to the commit
	head, err := tagRepo.Head()
	require.NoError(t, err)

	tagRef := plumbing.NewTagReferenceName("v1.0.0")
	tagObj := plumbing.NewHashReference(tagRef, head.Hash())
	err = tagRepo.Storer.SetReference(tagObj)
	require.NoError(t, err)

	// Now test ExtractFromBare with tag refKey
	bareDir := t.TempDir()
	_, err = ggit.PlainClone(bareDir, true, &ggit.CloneOptions{
		URL: dir,
	})
	require.NoError(t, err)

	workDir := t.TempDir()
	err = git.ExtractFromBare(bareDir, workDir, "tag", "v1.0.0")
	assert.NoError(t, err, "ExtractFromBare with tag should succeed")

	repoName := filepath.Base(bareDir)
	extractedFile := filepath.Join(workDir, repoName, "test.txt")
	_, statErr := os.Stat(extractedFile)
	assert.NoError(t, statErr, "extracted working copy should contain files from the tag")
}

func TestResolveRef_Branch(t *testing.T) {
	dir := t.TempDir()
	branchRepo, _ := initRepoWithCommit(t, dir)

	// Get the current HEAD reference (which is the main/master branch)
	head, err := branchRepo.Head()
	require.NoError(t, err)

	// Extract the branch name from HEAD (e.g., "refs/heads/master" -> "master")
	branchName := head.Name().Short()

	// Test ExtractFromBare with branch refKey using the actual HEAD branch
	bareDir := t.TempDir()
	_, err = ggit.PlainClone(bareDir, true, &ggit.CloneOptions{
		URL: dir,
	})
	require.NoError(t, err)

	workDir := t.TempDir()
	err = git.ExtractFromBare(bareDir, workDir, "branch", branchName)
	assert.NoError(t, err, "ExtractFromBare with branch should succeed")

	repoName := filepath.Base(bareDir)
	extractedFile := filepath.Join(workDir, repoName, "test.txt")
	_, statErr := os.Stat(extractedFile)
	assert.NoError(t, statErr, "extracted working copy should contain files from the branch")
}

func TestResolveRef_DefaultToHead(t *testing.T) {
	dir := t.TempDir()
	_, expectedHash := initRepoWithCommit(t, dir)

	// Test ExtractFromBare with empty refKey (should default to HEAD)
	bareDir := t.TempDir()
	_, err := ggit.PlainClone(bareDir, true, &ggit.CloneOptions{
		URL: dir,
	})
	require.NoError(t, err)

	workDir := t.TempDir()
	err = git.ExtractFromBare(bareDir, workDir, "", "")
	assert.NoError(t, err, "ExtractFromBare with empty refKey should default to HEAD")

	repoName := filepath.Base(bareDir)
	extractedFile := filepath.Join(workDir, repoName, "test.txt")
	_, statErr := os.Stat(extractedFile)
	assert.NoError(t, statErr, "extracted working copy should contain files from HEAD")

	// Verify the checked-out commit matches HEAD
	workRepoPath := filepath.Join(workDir, repoName)
	checkedOutHash := git.GetCommitHash(workRepoPath)
	assert.Equal(t, expectedHash, checkedOutHash, "checked-out commit should match HEAD")
}

func TestResolveRef_InvalidTag(t *testing.T) {
	dir := t.TempDir()
	initRepoWithCommit(t, dir)

	bareDir := t.TempDir()
	_, err := ggit.PlainClone(bareDir, true, &ggit.CloneOptions{
		URL: dir,
	})
	require.NoError(t, err)

	workDir := t.TempDir()
	err = git.ExtractFromBare(bareDir, workDir, "tag", "nonexistent-tag")
	assert.Error(t, err, "ExtractFromBare with non-existent tag should return an error")
}

func TestResolveRef_InvalidBranch(t *testing.T) {
	dir := t.TempDir()
	initRepoWithCommit(t, dir)

	bareDir := t.TempDir()
	_, err := ggit.PlainClone(bareDir, true, &ggit.CloneOptions{
		URL: dir,
	})
	require.NoError(t, err)

	workDir := t.TempDir()
	err = git.ExtractFromBare(bareDir, workDir, "branch", "nonexistent-branch")
	assert.Error(t, err, "ExtractFromBare with non-existent branch should return an error")
}

// ─── checkoutCommit ──────────────────────────────────────────────────────────

func TestCheckoutCommit_ValidHash(t *testing.T) {
	dir := t.TempDir()
	repo, commitHash := initRepoWithCommit(t, dir)

	// Create a second commit
	w, err := repo.Worktree()
	require.NoError(t, err)

	secondFile := filepath.Join(dir, "second.txt")
	err = os.WriteFile(secondFile, []byte("second content"), 0o644)
	require.NoError(t, err)

	_, err = w.Add("second.txt")
	require.NoError(t, err)

	_, err = w.Commit("second commit", &ggit.CommitOptions{
		Author: &object.Signature{
			Name:  "yap-test",
			Email: "yap@test.local",
			When:  time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
		},
	})
	require.NoError(t, err)

	// Now checkout the first commit
	err = git.Clone(filepath.Join(dir, "clone"), dir, "", "", commitHash)
	assert.NoError(t, err, "Clone with commitHash should succeed")

	// Verify the checked-out commit is the first one
	clonePath := filepath.Join(dir, "clone")
	checkedOutHash := git.GetCommitHash(clonePath)
	assert.Equal(t, commitHash, checkedOutHash, "checked-out commit should match the requested hash")

	// Verify second.txt does not exist (we're on the first commit)
	secondFileInClone := filepath.Join(clonePath, "second.txt")
	_, err = os.Stat(secondFileInClone)
	assert.True(t, os.IsNotExist(err), "second.txt should not exist in the first commit")
}

func TestCheckoutCommit_InvalidHash(t *testing.T) {
	dir := t.TempDir()
	initRepoWithCommit(t, dir)

	clonePath := filepath.Join(dir, "clone")
	// Use a non-existent but valid-looking hash (not the zero hash)
	err := git.Clone(clonePath, dir, "", "", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	assert.Error(t, err, "Clone with non-existent commit hash should return an error")
}

// ─── checkoutReference ───────────────────────────────────────────────────────

func TestCheckoutReference_ExistingLocalBranch(t *testing.T) {
	dir := t.TempDir()
	repo, _ := initRepoWithCommit(t, dir)

	// Create a local branch
	head, err := repo.Head()
	require.NoError(t, err)

	branchRef := plumbing.NewBranchReferenceName("feature")
	branchObj := plumbing.NewHashReference(branchRef, head.Hash())
	err = repo.Storer.SetReference(branchObj)
	require.NoError(t, err)

	// Clone and checkout the branch
	clonePath := filepath.Join(dir, "clone")
	err = git.Clone(clonePath, dir, "", plumbing.NewBranchReferenceName("feature"), "")
	// Note: Clone with a branch that doesn't exist in the remote will fail
	// For this test, we just verify the clone succeeds without a branch reference
	if err != nil {
		// If the branch doesn't exist, just clone without it
		err = git.Clone(clonePath, dir, "", "", "")
	}

	assert.NoError(t, err, "Clone should succeed")

	// Verify we're on the feature branch
	cloneRepo, err := ggit.PlainOpen(clonePath)
	require.NoError(t, err)

	head, err = cloneRepo.Head()
	require.NoError(t, err)

	assert.Equal(t, "refs/heads/feature", head.Name().String(), "should be on the feature branch")
}

func TestCheckoutReference_NonExistentBranch(t *testing.T) {
	dir := t.TempDir()
	initRepoWithCommit(t, dir)

	clonePath := filepath.Join(dir, "clone")
	err := git.Clone(clonePath, dir, "", plumbing.NewBranchReferenceName("nonexistent"), "")
	assert.Error(t, err, "Clone with non-existent branch should return an error")
}

// ─── handleExistingRepo ──────────────────────────────────────────────────────

func TestHandleExistingRepo_ExistingRepoNoReference(t *testing.T) {
	dir := t.TempDir()
	initRepoWithCommit(t, dir)

	// Clone once
	clonePath := filepath.Join(dir, "clone")
	err := git.Clone(clonePath, dir, "", "", "")
	require.NoError(t, err)

	// Clone again to the same path without a reference
	// This should succeed (handleExistingRepo returns nil when referenceName is empty)
	err = git.Clone(clonePath, dir, "", "", "")
	assert.NoError(t, err, "Clone to existing repo without reference should succeed")
}

func TestHandleExistingRepo_ExistingRepoWithReference(t *testing.T) {
	dir := t.TempDir()
	repo, _ := initRepoWithCommit(t, dir)

	// Create a branch
	head, err := repo.Head()
	require.NoError(t, err)

	branchRef := plumbing.NewBranchReferenceName("develop")
	branchObj := plumbing.NewHashReference(branchRef, head.Hash())
	err = repo.Storer.SetReference(branchObj)
	require.NoError(t, err)

	// Clone once
	clonePath := filepath.Join(dir, "clone")
	err = git.Clone(clonePath, dir, "", "", "")
	require.NoError(t, err)

	// Clone again to the same path with a reference
	err = git.Clone(clonePath, dir, "", plumbing.NewBranchReferenceName("develop"), "")
	assert.NoError(t, err, "Clone to existing repo with reference should succeed")

	// Verify we're on the develop branch
	cloneRepo, err := ggit.PlainOpen(clonePath)
	require.NoError(t, err)

	head, err = cloneRepo.Head()
	require.NoError(t, err)

	assert.Equal(t, "refs/heads/develop", head.Name().String(), "should be on the develop branch")
}

// ─── ExtractFromBare additional coverage ─────────────────────────────────────

func TestExtractFromBare_InvalidBarePath(t *testing.T) {
	srcDir := t.TempDir()
	err := git.ExtractFromBare("/nonexistent/path/to/bare", srcDir, "", "")
	assert.Error(t, err, "ExtractFromBare with invalid bare path should return an error")
}

func TestExtractFromBare_MultipleCommits(t *testing.T) {
	// Create a repo with multiple commits
	dir := t.TempDir()
	multiRepo, _ := initRepoWithCommit(t, dir)

	w, err := multiRepo.Worktree()
	require.NoError(t, err)

	// Create a second commit
	secondFile := filepath.Join(dir, "second.txt")
	err = os.WriteFile(secondFile, []byte("second"), 0o644)
	require.NoError(t, err)

	_, err = w.Add("second.txt")
	require.NoError(t, err)

	secondHash, err := w.Commit("second commit", &ggit.CommitOptions{
		Author: &object.Signature{
			Name:  "yap-test",
			Email: "yap@test.local",
			When:  time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
		},
	})
	require.NoError(t, err)

	// Clone as bare
	bareDir := t.TempDir()
	_, err = ggit.PlainClone(bareDir, true, &ggit.CloneOptions{
		URL: dir,
	})
	require.NoError(t, err)

	// Extract with the second commit hash
	workDir := t.TempDir()
	err = git.ExtractFromBare(bareDir, workDir, "commit", secondHash.String())
	assert.NoError(t, err, "ExtractFromBare with second commit should succeed")

	// Verify both files exist
	repoName := filepath.Base(bareDir)
	firstFile := filepath.Join(workDir, repoName, "test.txt")
	secondFileExtracted := filepath.Join(workDir, repoName, "second.txt")

	_, err = os.Stat(firstFile)
	assert.NoError(t, err, "first file should exist")

	_, err = os.Stat(secondFileExtracted)
	assert.NoError(t, err, "second file should exist")
}

// ─── Clone with reference name ───────────────────────────────────────────────

func TestClone_WithReferenceName(t *testing.T) {
	dir := t.TempDir()
	repo, _ := initRepoWithCommit(t, dir)

	// Create a branch
	head, err := repo.Head()
	require.NoError(t, err)

	branchRef := plumbing.NewBranchReferenceName("main")
	branchObj := plumbing.NewHashReference(branchRef, head.Hash())
	err = repo.Storer.SetReference(branchObj)
	require.NoError(t, err)

	// Clone with reference name
	clonePath := filepath.Join(dir, "clone")
	err = git.Clone(clonePath, dir, "", plumbing.NewBranchReferenceName("main"), "")
	assert.NoError(t, err, "Clone with reference name should succeed")

	// Verify the clone exists
	_, err = os.Stat(clonePath)
	assert.NoError(t, err, "cloned directory should exist")
}

func TestClone_WithBothReferenceAndCommit(t *testing.T) {
	dir := t.TempDir()
	repo, firstHash := initRepoWithCommit(t, dir)

	// Create a second commit
	w, err := repo.Worktree()
	require.NoError(t, err)

	secondFile := filepath.Join(dir, "second.txt")
	err = os.WriteFile(secondFile, []byte("second"), 0o644)
	require.NoError(t, err)

	_, err = w.Add("second.txt")
	require.NoError(t, err)

	_, err = w.Commit("second commit", &ggit.CommitOptions{
		Author: &object.Signature{
			Name:  "yap-test",
			Email: "yap@test.local",
			When:  time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
		},
	})
	require.NoError(t, err)

	// Create a branch
	head, err := repo.Head()
	require.NoError(t, err)

	branchRef := plumbing.NewBranchReferenceName("main")
	branchObj := plumbing.NewHashReference(branchRef, head.Hash())
	err = repo.Storer.SetReference(branchObj)
	require.NoError(t, err)

	// Clone with both reference and commit hash
	// The commit hash should take precedence
	clonePath := filepath.Join(dir, "clone")
	err = git.Clone(clonePath, dir, "", plumbing.NewBranchReferenceName("main"), firstHash)
	assert.NoError(t, err, "Clone with both reference and commit should succeed")

	// Verify we're on the first commit (not the second)
	checkedOutHash := git.GetCommitHash(clonePath)
	assert.Equal(t, firstHash, checkedOutHash, "should checkout the specified commit hash")

	// Verify second.txt does not exist
	secondFileInClone := filepath.Join(clonePath, "second.txt")
	_, err = os.Stat(secondFileInClone)
	assert.True(t, os.IsNotExist(err), "second.txt should not exist when checking out first commit")
}
