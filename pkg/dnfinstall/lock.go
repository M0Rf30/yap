package dnfinstall

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/M0Rf30/yap/v2/pkg/errors"
)

// acquireLock takes an exclusive flock on <rootDir>/var/lib/yap/install.lock.
// Polls every 500ms for up to 30s. Honors ctx cancellation.
// Returns a release function that must be called to unlock and close the file.
func acquireLock(ctx context.Context, rootDir string) (release func() error, err error) {
	lockPath := filepath.Join(rootDir, "var/lib/yap/install.lock")

	// Ensure parent directory exists.
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		return nil, errors.Wrap(err, errors.ErrTypeFileSystem, "failed to create lock directory").
			WithOperation("acquireLock").
			WithContext("path", lockPath)
	}

	// Open or create the lock file.
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrTypeFileSystem, "failed to open lock file").
			WithOperation("acquireLock").
			WithContext("path", lockPath)
	}

	// Poll for the lock with a 30-second timeout.
	deadline := time.Now().Add(30 * time.Second)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		// Try to acquire the lock non-blocking.
		err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			// Lock acquired successfully.
			return func() error {
				_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
				return f.Close()
			}, nil
		}

		// Check if it's a "would block" error (lock held by another process).
		if err != syscall.EWOULDBLOCK {
			_ = f.Close()
			return nil, errors.Wrap(err, errors.ErrTypeFileSystem, "flock failed").
				WithOperation("acquireLock").
				WithContext("path", lockPath)
		}

		// Check timeout and context.
		select {
		case <-ctx.Done():
			_ = f.Close()
			return nil, errors.Wrap(ctx.Err(), errors.ErrTypeFileSystem, "context cancelled while waiting for lock").
				WithOperation("acquireLock")
		case <-ticker.C:
			if time.Now().After(deadline) {
				_ = f.Close()
				return nil, errors.New(errors.ErrTypeFileSystem, "lock acquisition timeout (30s)").
					WithOperation("acquireLock").
					WithContext("path", lockPath)
			}
		}
	}
}
