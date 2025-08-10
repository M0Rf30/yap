// Package crypto provides cryptographic operations for package building.
package crypto

import (
	"crypto/sha256"
	"io"
	"os"
	"path/filepath"

	"github.com/M0Rf30/yap/v2/pkg/logger"
)

// CalculateSHA256 calculates the SHA-256 checksum of a file.
// This consolidates the SHA256 calculation logic from osutils and fileutils.
func CalculateSHA256(path string) ([]byte, error) {
	cleanFilePath := filepath.Clean(path)

	file, err := os.Open(cleanFilePath)
	if err != nil {
		return nil, err
	}

	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			logger.Warn("failed to close file during SHA256 calculation",
				"path", cleanFilePath, "error", closeErr)
		}
	}()

	hash := sha256.New()

	_, err = io.Copy(hash, file)
	if err != nil {
		return nil, err
	}

	return hash.Sum(nil), nil
}

// CalculateSHA256FromReader calculates SHA256 from an io.Reader.
// This is useful for calculating hashes of data streams.
func CalculateSHA256FromReader(reader io.Reader) ([]byte, error) {
	hash := sha256.New()

	_, err := io.Copy(hash, reader)
	if err != nil {
		return nil, err
	}

	return hash.Sum(nil), nil
}

// VerifySHA256 verifies that a file matches the given SHA256 hash.
func VerifySHA256(path string, expectedHash []byte) (bool, error) {
	actualHash, err := CalculateSHA256(path)
	if err != nil {
		return false, err
	}

	if len(actualHash) != len(expectedHash) {
		return false, nil
	}

	for i := range actualHash {
		if actualHash[i] != expectedHash[i] {
			return false, nil
		}
	}

	return true, nil
}
