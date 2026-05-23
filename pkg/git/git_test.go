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
