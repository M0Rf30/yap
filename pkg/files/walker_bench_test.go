package files

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// BenchmarkWalkPackageDir500Files benchmarks the file walker on a directory with 500 files.
func BenchmarkWalkPackageDir500Files(b *testing.B) {
	// Setup: create a temporary directory with 500 empty files
	tmpDir := b.TempDir()
	createTestFiles(b, tmpDir, 500)

	// Create walker with standard options
	walker := NewWalker(tmpDir, WalkOptions{
		SkipDotFiles: true,
		BackupFiles:  []string{"/etc/config"},
		SkipPatterns: []string{"*.tmp", "*.bak"},
	})

	// Reset timer after setup
	b.ResetTimer()

	// Benchmark: walk the directory N times
	for range b.N {
		_, err := walker.Walk()
		if err != nil {
			b.Fatalf("Walk failed: %v", err)
		}
	}
}

// BenchmarkWalkPackageDir5000Files benchmarks the file walker on a directory with 5000 files.
func BenchmarkWalkPackageDir5000Files(b *testing.B) {
	// Setup: create a temporary directory with 5000 empty files
	tmpDir := b.TempDir()
	createTestFiles(b, tmpDir, 5000)

	// Create walker with standard options
	walker := NewWalker(tmpDir, WalkOptions{
		SkipDotFiles: true,
		BackupFiles:  []string{"/etc/config"},
		SkipPatterns: []string{"*.tmp", "*.bak"},
	})

	// Reset timer after setup
	b.ResetTimer()

	// Benchmark: walk the directory N times
	for range b.N {
		_, err := walker.Walk()
		if err != nil {
			b.Fatalf("Walk failed: %v", err)
		}
	}
}

// createTestFiles creates N empty files in the given directory.
func createTestFiles(b *testing.B, dir string, count int) {
	b.Helper()

	for i := range count {
		filePath := filepath.Join(dir, fmt.Sprintf("file_%d.txt", i))

		f, err := os.Create(filePath)
		if err != nil {
			b.Fatalf("Failed to create test file: %v", err)
		}

		_ = f.Close()
	}
}
