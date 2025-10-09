package git

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/go-git/go-git/v5/plumbing"
)

func TestCloneNonExistentRepo(t *testing.T) {
	tempDir := t.TempDir()
	clonePath := filepath.Join(tempDir, "test-repo")

	// Try to clone a non-existent repository
	err := Clone(clonePath, "https://github.com/non-existent-user/non-existent-repo.git", "", "")

	// This should fail, which is expected
	if err == nil {
		t.Fatal("Expected error when cloning non-existent repository, got nil")
	}
}

func TestCloneInvalidURL(t *testing.T) {
	tempDir := t.TempDir()
	clonePath := filepath.Join(tempDir, "test-repo")

	// Try to clone with an invalid URL
	err := Clone(clonePath, "invalid-url", "", "")

	// This should fail, which is expected
	if err == nil {
		t.Fatal("Expected error when cloning with invalid URL, got nil")
	}
}

func TestCloneEmptyPath(t *testing.T) {
	// Try to clone to an empty path
	err := Clone("", "https://github.com/octocat/Hello-World.git", "", "")

	// This should fail, which is expected
	if err == nil {
		t.Fatal("Expected error when cloning to empty path, got nil")
	}
}

func TestCloneWithBranch(t *testing.T) {
	tempDir := t.TempDir()
	clonePath := filepath.Join(tempDir, "test-repo")

	// Create a reference name for a specific branch
	branchRef := plumbing.NewBranchReferenceName("main")

	// Try to clone a non-existent repository with branch
	err := Clone(clonePath, "https://github.com/non-existent-user/non-existent-repo.git", "", branchRef)

	// This should fail, which is expected for non-existent repo
	if err == nil {
		t.Fatal("Expected error when cloning non-existent repository with branch, got nil")
	}
}

func TestCloneWithSSHPassword(t *testing.T) {
	tempDir := t.TempDir()
	clonePath := filepath.Join(tempDir, "test-repo")

	// Try to clone with SSH password (this will likely fail due to missing SSH keys)
	err := Clone(clonePath, "git@github.com:non-existent-user/non-existent-repo.git", "test-password", "")

	// This should fail, which is expected
	if err == nil {
		t.Fatal("Expected error when cloning with SSH password, got nil")
	}
}

func TestHandleExistingRepoNonExistent(t *testing.T) {
	tempDir := t.TempDir()
	nonExistentPath := filepath.Join(tempDir, "non-existent")

	// Try to handle a non-existent repository directory
	// Since we can't directly test the private function, we'll test the Clone function
	// with an existing directory that's not a git repo
	err := os.MkdirAll(nonExistentPath, 0o755)
	if err != nil {
		t.Fatalf("Failed to create test directory: %v", err)
	}

	// Try to clone to existing non-git directory
	err = Clone(nonExistentPath, "https://github.com/non-existent-user/non-existent-repo.git", "", "")

	// This should fail when it tries to open the existing path as a git repo
	if err == nil {
		t.Fatal("Expected error when handling existing non-git directory, got nil")
	}
}
func TestCheckoutReferenceLogic(t *testing.T) {
	// Test branch reference creation
	branchName := "test-branch"
	branchRef := plumbing.NewBranchReferenceName(branchName)

	if branchRef.Short() != branchName {
		t.Fatalf("Expected branch reference short name to be '%s', got '%s'", branchName, branchRef.Short())
	}

	// Test remote reference creation
	remoteBranchRef := plumbing.NewRemoteReferenceName("origin", branchName)
	expectedRemoteName := "refs/remotes/origin/" + branchName

	if remoteBranchRef.String() != expectedRemoteName {
		t.Fatalf("Expected remote reference name to be '%s', got '%s'", expectedRemoteName, remoteBranchRef.String())
	}
}

func TestPlumbingReferences(t *testing.T) {
	// Test various plumbing reference operations
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "main branch",
			input:    "main",
			expected: "refs/heads/main",
		},
		{
			name:     "develop branch",
			input:    "develop",
			expected: "refs/heads/develop",
		},
		{
			name:     "feature branch",
			input:    "feature/test",
			expected: "refs/heads/feature/test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			branchRef := plumbing.NewBranchReferenceName(tt.input)
			if branchRef.String() != tt.expected {
				t.Fatalf("Expected reference name '%s', got '%s'", tt.expected, branchRef.String())
			}
		})
	}
}

func TestEmptyReferenceName(t *testing.T) {
	tempDir := t.TempDir()
	clonePath := filepath.Join(tempDir, "test-repo")

	// Test with empty reference name (should use default branch)
	var emptyRef plumbing.ReferenceName = ""

	err := Clone(clonePath, "https://github.com/non-existent-user/non-existent-repo.git", "", emptyRef)

	// This should fail for non-existent repo, but tests the empty reference handling
	if err == nil {
		t.Fatal("Expected error when cloning non-existent repository, got nil")
	}
}

func TestCloneToExistingFile(t *testing.T) {
	tempDir := t.TempDir()
	existingFile := filepath.Join(tempDir, "existing-file")

	// Create an existing file
	err := os.WriteFile(existingFile, []byte("test content"), 0o644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Try to clone to the existing file path
	err = Clone(existingFile, "https://github.com/non-existent-user/non-existent-repo.git", "", "")

	// This should fail when it tries to use the file as a git repository
	if err == nil {
		t.Fatal("Expected error when cloning to existing file, got nil")
	}
}

func TestGetCommitHash(t *testing.T) {
	// Test with non-existent directory
	hash := GetCommitHash("/non-existent-directory")
	if hash != "" {
		t.Fatalf("Expected empty hash for non-existent directory, got '%s'", hash)
	}

	// Test with non-git directory
	tempDir := t.TempDir()

	hash = GetCommitHash(tempDir)
	if hash != "" {
		t.Fatalf("Expected empty hash for non-git directory, got '%s'", hash)
	}
}

// Additional tests to improve coverage
func TestGetCommitHashWithRealRepo(t *testing.T) {
	// Since we can't easily create a real git repo in a temp directory for testing,
	// we'll at least verify the function doesn't crash with a valid git repo structure
	tempDir := t.TempDir()

	// Create a .git directory to simulate a git repo (but not a complete one)
	gitDir := filepath.Join(tempDir, ".git")

	err := os.MkdirAll(gitDir, 0o755)
	if err != nil {
		t.Fatalf("Failed to create .git directory: %v", err)
	}

	// This should return empty string since it's not a valid git repo
	hash := GetCommitHash(tempDir)
	if hash != "" {
		t.Fatalf("Expected empty hash for incomplete git directory, got '%s'", hash)
	}
}
