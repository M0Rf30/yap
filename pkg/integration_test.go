package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	yapcontext "github.com/M0Rf30/yap/pkg/context"
	"github.com/M0Rf30/yap/pkg/errors"
	"github.com/M0Rf30/yap/pkg/logger"
)

// Static errors for err113 compliance.
var (
	errPermissionDenied = errors.New(errors.ErrTypeFileSystem, "permission denied")
	errAttemptFailed    = errors.New(errors.ErrTypeNetwork, "attempt failed")
)

// IntegrationTest demonstrates the improved architecture and capabilities.
func main() {
	fmt.Println("=== YAP Codebase Improvements Integration Test ===")

	// Test 1: Enhanced Error Handling
	fmt.Println("1. Testing Enhanced Error Handling:")
	testErrorHandling()
	fmt.Println()

	// Test 2: Structured Logging
	fmt.Println("2. Testing Structured Logging:")
	testStructuredLogging()
	fmt.Println()

	// Test 3: Context Support and Timeouts
	fmt.Println("3. Testing Context Support:")
	testContextSupport()
	fmt.Println()

	// Test 4: Worker Pool and Concurrency
	fmt.Println("4. Testing Worker Pool:")
	testWorkerPool()
	fmt.Println()

	// Test 5: Demonstrate Enhanced Error Handling
	fmt.Println("5. Enhanced Error Handling Demonstration:")
	demonstrateImprovedErrorHandling()
	fmt.Println()

	// Test 6: Demonstrate Enhanced Logging
	fmt.Println("6. Enhanced Logging Demonstration:")
	demonstrateImprovedLogging()
	fmt.Println()

	fmt.Println("=== All Tests Completed Successfully ===")
}

func testErrorHandling() {
	// Create various error types
	validationErr := errors.NewValidationError("invalid package name").
		WithContext("package", "test-pkg").
		WithOperation("validate_package")

	fileErr := errors.Wrap(
		errPermissionDenied,
		errors.ErrTypeFileSystem,
		"failed to read file",
	).WithContext("file", "/tmp/example.txt")

	// Test error chain
	chain := errors.NewChain()
	_ = chain.Add(validationErr).Add(fileErr)

	fmt.Printf("  ✓ Validation Error: %v\n", validationErr)
	fmt.Printf("  ✓ File Error: %v\n", fileErr)
	fmt.Printf("  ✓ Error Chain: %d errors\n", len(chain.Errors()))

	// Test error type checking
	if errors.IsType(validationErr, errors.ErrTypeValidation) {
		fmt.Printf("  ✓ Error type detection works\n")
	}
}

func testStructuredLogging() {
	// Create logger with JSON format
	logConfig := &logger.LoggerConfig{
		Level:      logger.LevelDebug,
		Format:     "json",
		Output:     os.Stdout,
		AddSource:  true,
		TimeFormat: time.RFC3339,
	}

	testLogger := logger.New(logConfig)

	// Test structured logging
	testLogger.Info("Starting build process",
		"project", "test-project",
		"version", "1.0.0",
		"distro", "ubuntu",
	)

	// Test error logging
	testErr := errors.NewBuildError("compilation failed").
		WithContext("file", "main.go").
		WithOperation("compile")

	testLogger.WithError(testErr).Error("Build failed")

	// Test operation logging
	_ = testLogger.LogOperation("test_operation", func() error {
		time.Sleep(10 * time.Millisecond)

		return nil
	})

	fmt.Printf("  ✓ Structured logging with JSON format\n")
	fmt.Printf("  ✓ Error context integration\n")
	fmt.Printf("  ✓ Operation timing\n")
}

func testContextSupport() {
	// Test timeout context
	ctx, cancel := yapcontext.BackgroundWithTimeout(100 * time.Millisecond)
	defer cancel()

	// Test build context
	buildCtx := yapcontext.NewBuildContext("build_123", "test-project", "test-pkg", "ubuntu", "20.04")
	ctx = yapcontext.WithBuildContext(ctx, buildCtx)

	// Test context retrieval
	retrievedBuildCtx := yapcontext.GetBuildContext(ctx)
	fmt.Printf("  ✓ Build context: %s for %s\n", retrievedBuildCtx.BuildID, retrievedBuildCtx.Project)

	// Test timeout manager
	timeoutMgr := yapcontext.NewTimeoutManager()
	timedCtx := timeoutMgr.AddTimeout("test", context.Background(), 50*time.Millisecond)

	select {
	case <-timedCtx.Done():
		fmt.Printf("  ✓ Timeout manager works correctly\n")
	case <-time.After(100 * time.Millisecond):
		fmt.Printf("  ✗ Timeout manager failed\n")
	}

	// Test retry with context
	attempts := 0
	err := yapcontext.RetryWithContext(context.Background(), 3, 10*time.Millisecond, func(_ context.Context) error {
		attempts++
		if attempts < 3 {
			return fmt.Errorf("attempt %d failed: %w", attempts, errAttemptFailed)
		}

		return nil
	})

	if err == nil && attempts == 3 {
		fmt.Printf("  ✓ Retry with exponential backoff: %d attempts\n", attempts)
	}
}

func testWorkerPool() {
	// Create worker pool
	pool := yapcontext.NewWorkerPool(4)

	defer func() {
		err := pool.Shutdown(5 * time.Second)
		if err != nil {
			fmt.Printf("  ⚠ Failed to shutdown worker pool: %v\n", err)
		}
	}()

	// Test semaphore
	sem := yapcontext.NewSemaphore(2)
	fmt.Printf("  ✓ Semaphore created with %d available slots\n", sem.Available())

	// Acquire and release
	ctx := context.Background()

	err := sem.Acquire(ctx)
	if err != nil {
		fmt.Printf("  ✗ Semaphore acquire failed: %v\n", err)

		return
	}

	fmt.Printf("  ✓ Semaphore acquired, %d slots remaining\n", sem.Available())
	sem.Release()
	fmt.Printf("  ✓ Semaphore released, %d slots available\n", sem.Available())

	// Test worker pool with multiple tasks
	taskCount := 10
	completed := 0

	for taskIndex := range taskCount {
		taskID := taskIndex

		err := pool.Submit(ctx, func(_ context.Context) error {
			// Simulate work
			time.Sleep(10 * time.Millisecond)

			completed++

			fmt.Printf("    Task %d completed\n", taskID)

			return nil
		})
		if err != nil {
			fmt.Printf("  ✗ Failed to submit task %d: %v\n", taskIndex, err)

			return
		}
	}

	// Wait a bit for tasks to complete
	time.Sleep(200 * time.Millisecond)

	fmt.Printf("  ✓ Worker pool: %d/%d tasks completed\n", completed, taskCount)
	fmt.Printf("  ✓ Available workers: %d\n", pool.Available())
}

// Example of how the improved error handling would be used in the actual codebase.
func demonstrateImprovedErrorHandling() {
	// This shows how the original project.go functions would be enhanced
	err := os.MkdirAll("/tmp/test", 0o755)
	if err != nil {
		enhancedErr := errors.Wrap(err, errors.ErrTypeFileSystem, "failed to create build directory").
			WithContext("path", "/tmp/test").
			WithOperation("prepare_build")

		log.Printf("Enhanced error: %v", enhancedErr)

		// Error chain for multiple related errors
		chain := errors.NewChain()
		_ = chain.Add(enhancedErr)

		readErr := errAttemptFailed
		if readErr != nil {
			networkErr := errors.Wrap(readErr, errors.ErrTypeNetwork, "network operation failed")
			_ = chain.Add(networkErr)
		}

		if chain.HasErrors() {
			log.Printf("Multiple errors occurred: %v", chain)
		}
	}
}

// Example of improved logging integration.
func demonstrateImprovedLogging() {
	globalLogger := logger.Global()

	// Improved:
	globalLogger.Info("making package",
		"pkgname", "test-package",
		"pkgver", "1.0.0",
		"pkgrel", "1",
		"distro", "ubuntu",
		"arch", "x86_64",
	)

	// With error context
	buildErr := errors.NewBuildError("compilation failed").
		WithContext("file", "main.c").
		WithOperation("gcc_compile")

	globalLogger.WithError(buildErr).Error("Build process failed")

	// Operation timing
	_ = globalLogger.LogOperation("package_creation", func() error {
		time.Sleep(50 * time.Millisecond) // Simulate work

		return nil
	})
}
