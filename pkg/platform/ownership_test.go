package platform

import (
	"os"
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
