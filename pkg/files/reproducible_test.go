package files

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestResolveSourceDateEpoch_WithValidEnvVar(t *testing.T) {
	// Test with a valid SOURCE_DATE_EPOCH environment variable
	testEpoch := int64(1609459200) // 2021-01-01 00:00:00 UTC
	t.Setenv("SOURCE_DATE_EPOCH", strconv.FormatInt(testEpoch, 10))

	result, err := ResolveSourceDateEpoch(t.TempDir())
	if err != nil {
		t.Fatalf("ResolveSourceDateEpoch() returned error: %v", err)
	}

	expected := time.Unix(testEpoch, 0).UTC()
	if !result.Equal(expected) {
		t.Errorf("ResolveSourceDateEpoch() = %v, want %v", result, expected)
	}
}

func TestResolveSourceDateEpoch_WithInvalidEnvVar(t *testing.T) {
	// Test with an invalid SOURCE_DATE_EPOCH environment variable
	t.Setenv("SOURCE_DATE_EPOCH", "not-a-number")

	_, err := ResolveSourceDateEpoch(t.TempDir())
	if err == nil {
		t.Fatal("ResolveSourceDateEpoch() returned nil error, want error for invalid epoch")
	}

	// Verify error message contains useful context
	if err.Error() == "" {
		t.Error("Error message is empty")
	}
}

func TestResolveSourceDateEpoch_WithPKGBUILDFile(t *testing.T) {
	// Test with no env var but PKGBUILD file exists
	t.Setenv("SOURCE_DATE_EPOCH", "")

	tmpDir := t.TempDir()
	pkgbuildPath := filepath.Join(tmpDir, "PKGBUILD")

	// Create PKGBUILD file
	if err := os.WriteFile(pkgbuildPath, []byte("# test"), 0o644); err != nil {
		t.Fatalf("Failed to create PKGBUILD: %v", err)
	}

	// Set specific mtime
	testTime := time.Date(2020, 6, 15, 10, 30, 45, 0, time.UTC)
	if err := os.Chtimes(pkgbuildPath, testTime, testTime); err != nil {
		t.Fatalf("Failed to set PKGBUILD mtime: %v", err)
	}

	result, err := ResolveSourceDateEpoch(tmpDir)
	if err != nil {
		t.Fatalf("ResolveSourceDateEpoch() returned error: %v", err)
	}

	// Compare timestamps (allow small tolerance for filesystem precision)
	if result.Unix() != testTime.Unix() {
		t.Errorf("ResolveSourceDateEpoch() = %v, want %v", result, testTime)
	}

	// Verify SOURCE_DATE_EPOCH was exported to environment
	envValue := os.Getenv("SOURCE_DATE_EPOCH")
	if envValue == "" {
		t.Error("SOURCE_DATE_EPOCH was not exported to environment")
	}

	parsedEpoch, err := strconv.ParseInt(envValue, 10, 64)
	if err != nil {
		t.Fatalf("Failed to parse exported SOURCE_DATE_EPOCH: %v", err)
	}

	if parsedEpoch != testTime.Unix() {
		t.Errorf("Exported SOURCE_DATE_EPOCH = %d, want %d", parsedEpoch, testTime.Unix())
	}
}

func TestResolveSourceDateEpoch_NoPKGBUILDFallback(t *testing.T) {
	// Test with no env var and no PKGBUILD file - should fallback to time.Now()
	t.Setenv("SOURCE_DATE_EPOCH", "")

	tmpDir := t.TempDir()
	// Don't create PKGBUILD file

	beforeCall := time.Now()
	result, err := ResolveSourceDateEpoch(tmpDir)
	afterCall := time.Now()

	if err != nil {
		t.Fatalf("ResolveSourceDateEpoch() returned error: %v", err)
	}

	// Result should be close to now (within a few seconds)
	if result.Before(beforeCall) || result.After(afterCall.Add(5*time.Second)) {
		t.Errorf("ResolveSourceDateEpoch() = %v, want time close to now (between %v and %v)",
			result, beforeCall, afterCall)
	}

	// SOURCE_DATE_EPOCH should NOT be exported when falling back to time.Now()
	// (since we can't set a future time)
	// This is acceptable behavior - the function returns time.Now() without error
}

func TestResolveSourceDateEpoch_EnvVarTakesPrecedence(t *testing.T) {
	// Test that SOURCE_DATE_EPOCH env var takes precedence over PKGBUILD mtime
	envEpoch := int64(1609459200) // 2021-01-01
	t.Setenv("SOURCE_DATE_EPOCH", strconv.FormatInt(envEpoch, 10))

	tmpDir := t.TempDir()
	pkgbuildPath := filepath.Join(tmpDir, "PKGBUILD")

	// Create PKGBUILD with different mtime
	if err := os.WriteFile(pkgbuildPath, []byte("# test"), 0o644); err != nil {
		t.Fatalf("Failed to create PKGBUILD: %v", err)
	}

	differentTime := time.Date(2019, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := os.Chtimes(pkgbuildPath, differentTime, differentTime); err != nil {
		t.Fatalf("Failed to set PKGBUILD mtime: %v", err)
	}

	result, err := ResolveSourceDateEpoch(tmpDir)
	if err != nil {
		t.Fatalf("ResolveSourceDateEpoch() returned error: %v", err)
	}

	// Should use env var, not PKGBUILD mtime
	expected := time.Unix(envEpoch, 0).UTC()
	if !result.Equal(expected) {
		t.Errorf("ResolveSourceDateEpoch() = %v, want %v (env var should take precedence)",
			result, expected)
	}
}

func TestSourceDateEpochFromEnv_WithValidEnvVar(t *testing.T) {
	// Test with valid SOURCE_DATE_EPOCH in environment
	testEpoch := int64(1609459200) // 2021-01-01 00:00:00 UTC
	t.Setenv("SOURCE_DATE_EPOCH", strconv.FormatInt(testEpoch, 10))

	result := SourceDateEpochFromEnv()

	expected := time.Unix(testEpoch, 0).UTC()
	if !result.Equal(expected) {
		t.Errorf("SourceDateEpochFromEnv() = %v, want %v", result, expected)
	}
}

func TestSourceDateEpochFromEnv_WithoutEnvVar(t *testing.T) {
	// Test with no SOURCE_DATE_EPOCH in environment - should return time.Now()
	t.Setenv("SOURCE_DATE_EPOCH", "")

	beforeCall := time.Now()
	result := SourceDateEpochFromEnv()
	afterCall := time.Now()

	// Result should be close to now (within a few seconds)
	if result.Before(beforeCall) || result.After(afterCall.Add(5*time.Second)) {
		t.Errorf("SourceDateEpochFromEnv() = %v, want time close to now (between %v and %v)",
			result, beforeCall, afterCall)
	}
}

func TestSourceDateEpochFromEnv_WithInvalidEnvVar(t *testing.T) {
	// Test with invalid SOURCE_DATE_EPOCH - should fallback to time.Now()
	t.Setenv("SOURCE_DATE_EPOCH", "invalid-value")

	beforeCall := time.Now()
	result := SourceDateEpochFromEnv()
	afterCall := time.Now()

	// Should fallback to time.Now() without error
	if result.Before(beforeCall) || result.After(afterCall.Add(5*time.Second)) {
		t.Errorf("SourceDateEpochFromEnv() = %v, want time close to now (between %v and %v)",
			result, beforeCall, afterCall)
	}
}

func TestSourceDateEpochFromEnv_WithZeroEpoch(t *testing.T) {
	// Test with SOURCE_DATE_EPOCH set to 0 (Unix epoch)
	t.Setenv("SOURCE_DATE_EPOCH", "0")

	result := SourceDateEpochFromEnv()

	expected := time.Unix(0, 0).UTC()
	if !result.Equal(expected) {
		t.Errorf("SourceDateEpochFromEnv() = %v, want %v", result, expected)
	}
}

func TestSourceDateEpochFromEnv_WithNegativeEpoch(t *testing.T) {
	// Test with negative epoch (before Unix epoch)
	testEpoch := int64(-86400) // 1 day before Unix epoch
	t.Setenv("SOURCE_DATE_EPOCH", strconv.FormatInt(testEpoch, 10))

	result := SourceDateEpochFromEnv()

	expected := time.Unix(testEpoch, 0).UTC()
	if !result.Equal(expected) {
		t.Errorf("SourceDateEpochFromEnv() = %v, want %v", result, expected)
	}
}
