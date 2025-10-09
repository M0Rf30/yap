package platform

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGetOriginalUser(t *testing.T) {
	// Test when not running under sudo
	originalUser, err := GetOriginalUser()
	if err != nil {
		t.Errorf("GetOriginalUser should not return error when not under sudo: %v", err)
	}

	// Should return nil when not under sudo
	if originalUser != nil {
		t.Errorf("GetOriginalUser should return nil when not under sudo")
	}
}

func TestGetOriginalUserWithSudo(t *testing.T) {
	// Simulate sudo environment
	_ = os.Setenv("SUDO_USER", "testuser")
	_ = os.Setenv("SUDO_UID", "1000")
	_ = os.Setenv("SUDO_GID", "1000")

	defer func() {
		_ = os.Unsetenv("SUDO_USER")
		_ = os.Unsetenv("SUDO_UID")
		_ = os.Unsetenv("SUDO_GID")
	}()

	originalUser, err := GetOriginalUser()
	if err != nil {
		t.Errorf("GetOriginalUser should not return error: %v", err)
	}

	if originalUser == nil {
		t.Errorf("GetOriginalUser should return user info when under sudo")
		return
	}

	if originalUser.Name != "testuser" {
		t.Errorf("Expected user name 'testuser', got '%s'", originalUser.Name)
	}

	if originalUser.UID != 1000 {
		t.Errorf("Expected UID 1000, got %d", originalUser.UID)
	}

	if originalUser.GID != 1000 {
		t.Errorf("Expected GID 1000, got %d", originalUser.GID)
	}
}

func TestIsRunningSudo(t *testing.T) {
	// Test when not running under sudo
	if IsRunningSudo() {
		t.Errorf("IsRunningSudo should return false when not under sudo")
	}

	// Simulate sudo environment
	_ = os.Setenv("SUDO_USER", "testuser")
	_ = os.Setenv("SUDO_UID", "1000")
	_ = os.Setenv("SUDO_GID", "1000")

	defer func() {
		_ = os.Unsetenv("SUDO_USER")
		_ = os.Unsetenv("SUDO_UID")
		_ = os.Unsetenv("SUDO_GID")
	}()

	if !IsRunningSudo() {
		t.Errorf("IsRunningSudo should return true when under sudo")
	}
}

func TestPreserveOwnership(t *testing.T) {
	// Test when not running under sudo
	err := PreserveOwnership("/tmp")
	if err != nil {
		t.Errorf("PreserveOwnership should not return error when not under sudo: %v", err)
	}

	// The function should not fail when not under sudo
}

func TestGetOriginalUserInvalidUID(t *testing.T) {
	// Test with invalid UID
	_ = os.Setenv("SUDO_USER", "testuser")
	_ = os.Setenv("SUDO_UID", "invalid")
	_ = os.Setenv("SUDO_GID", "1000")

	defer func() {
		_ = os.Unsetenv("SUDO_USER")
		_ = os.Unsetenv("SUDO_UID")
		_ = os.Unsetenv("SUDO_GID")
	}()

	originalUser, err := GetOriginalUser()
	if err == nil {
		t.Error("GetOriginalUser should return error for invalid UID")
	}

	if originalUser != nil {
		t.Error("GetOriginalUser should return nil when UID is invalid")
	}
}

func TestGetOriginalUserInvalidGID(t *testing.T) {
	// Test with invalid GID
	_ = os.Setenv("SUDO_USER", "testuser")
	_ = os.Setenv("SUDO_UID", "1000")
	_ = os.Setenv("SUDO_GID", "invalid")

	defer func() {
		_ = os.Unsetenv("SUDO_USER")
		_ = os.Unsetenv("SUDO_UID")
		_ = os.Unsetenv("SUDO_GID")
	}()

	originalUser, err := GetOriginalUser()
	if err == nil {
		t.Error("GetOriginalUser should return error for invalid GID")
	}

	if originalUser != nil {
		t.Error("GetOriginalUser should return nil when GID is invalid")
	}
}

func TestGetOriginalUserPartialEnv(t *testing.T) {
	tests := []struct {
		name      string
		sudoUser  string
		sudoUID   string
		sudoGID   string
		shouldErr bool
	}{
		{
			name:      "Missing SUDO_USER",
			sudoUser:  "",
			sudoUID:   "1000",
			sudoGID:   "1000",
			shouldErr: false, // Should return nil, not error
		},
		{
			name:      "Missing SUDO_UID",
			sudoUser:  "testuser",
			sudoUID:   "",
			sudoGID:   "1000",
			shouldErr: false, // Should return nil, not error
		},
		{
			name:      "Missing SUDO_GID",
			sudoUser:  "testuser",
			sudoUID:   "1000",
			sudoGID:   "",
			shouldErr: false, // Should return nil, not error
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment variables
			if tt.sudoUser != "" {
				_ = os.Setenv("SUDO_USER", tt.sudoUser)
			}

			if tt.sudoUID != "" {
				_ = os.Setenv("SUDO_UID", tt.sudoUID)
			}

			if tt.sudoGID != "" {
				_ = os.Setenv("SUDO_GID", tt.sudoGID)
			}

			defer func() {
				_ = os.Unsetenv("SUDO_USER")
				_ = os.Unsetenv("SUDO_UID")
				_ = os.Unsetenv("SUDO_GID")
			}()

			originalUser, err := GetOriginalUser()

			if tt.shouldErr && err == nil {
				t.Error("Expected error but got none")
			}

			if !tt.shouldErr && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			// Should return nil when any sudo env var is missing
			if originalUser != nil {
				t.Error("Expected nil user when sudo environment is incomplete")
			}
		})
	}
}

func TestOriginalUserChownToOriginalUser(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.txt")

	// Create test file
	err := os.WriteFile(testFile, []byte("test"), 0o644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Test with nil user (should be no-op)
	var nilUser *OriginalUser

	err = nilUser.ChownToOriginalUser(testFile)
	if err != nil {
		t.Errorf("ChownToOriginalUser with nil user should not error: %v", err)
	}

	// Test with valid user (will likely fail on most systems due to permissions)
	user := &OriginalUser{
		UID:  1000,
		GID:  1000,
		Name: "testuser",
		Home: "/home/testuser",
	}

	err = user.ChownToOriginalUser(testFile)
	// We expect this to fail on most test systems due to permissions
	// but we're testing that the function executes without panicking
	if err != nil {
		t.Logf("ChownToOriginalUser failed as expected in test environment: %v", err)
	}
}

func TestOriginalUserChownRecursiveToOriginalUser(t *testing.T) {
	tempDir := t.TempDir()
	subDir := filepath.Join(tempDir, "subdir")
	testFile := filepath.Join(subDir, "test.txt")

	// Create test structure
	err := os.MkdirAll(subDir, 0o755)
	if err != nil {
		t.Fatalf("Failed to create subdirectory: %v", err)
	}

	err = os.WriteFile(testFile, []byte("test"), 0o644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Test with nil user (should be no-op)
	var nilUser *OriginalUser

	err = nilUser.ChownRecursiveToOriginalUser(tempDir)
	if err != nil {
		t.Errorf("ChownRecursiveToOriginalUser with nil user should not error: %v", err)
	}

	// Test with valid user
	user := &OriginalUser{
		UID:  1000,
		GID:  1000,
		Name: "testuser",
		Home: "/home/testuser",
	}

	err = user.ChownRecursiveToOriginalUser(tempDir)
	// We expect this to potentially fail but should handle errors gracefully
	if err != nil {
		t.Logf("ChownRecursiveToOriginalUser failed as expected in test environment: %v", err)
	}
}

func TestPreserveOwnershipRecursive(t *testing.T) {
	tempDir := t.TempDir()

	// Test when not running under sudo
	err := PreserveOwnershipRecursive(tempDir)
	if err != nil {
		t.Errorf("PreserveOwnershipRecursive should not return error when not under sudo: %v", err)
	}

	// Test with simulated sudo environment
	_ = os.Setenv("SUDO_USER", "testuser")
	_ = os.Setenv("SUDO_UID", "1000")
	_ = os.Setenv("SUDO_GID", "1000")

	defer func() {
		_ = os.Unsetenv("SUDO_USER")
		_ = os.Unsetenv("SUDO_UID")
		_ = os.Unsetenv("SUDO_GID")
	}()

	err = PreserveOwnershipRecursive(tempDir)
	// This will likely fail due to permissions in test environment
	if err != nil {
		t.Logf("PreserveOwnershipRecursive failed as expected in test environment: %v", err)
	}
}
