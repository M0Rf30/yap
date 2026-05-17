package signing

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/M0Rf30/yap/v2/pkg/errors"
	"github.com/M0Rf30/yap/v2/pkg/logger"
)

// resolveAndValidateKeyPath resolves a key path to an absolute path and validates it exists.
func resolveAndValidateKeyPath(rawPath, source, sourceLabel string) (string, error) {
	absPath, err := filepath.Abs(rawPath)
	if err != nil {
		return "", errors.Wrap(err, errors.ErrTypeFileSystem,
			"failed to resolve absolute path for key").
			WithOperation("resolveAndValidateKeyPath").
			WithContext("source", source).
			WithContext("key_path", rawPath)
	}

	if _, err := os.Stat(absPath); err != nil {
		return "", errors.Wrap(err, errors.ErrTypeFileSystem,
			"key file not found").
			WithOperation("resolveAndValidateKeyPath").
			WithContext("source", source).
			WithContext("key_path", absPath)
	}

	logger.Debug("Resolved signing key from "+sourceLabel,
		source, sourceLabel, "key_path", absPath)

	return absPath, nil
}

// Resolve produces a final Config for a given Format, applying the priority:
// CLI flag > environment variable > project config > ~/.config/yap/keys/ default.
//
// For keys, the resolution order is:
//  1. flagKey (CLI --sign-key)
//  2. YAP_<FORMAT>_KEY env var (e.g., YAP_DEB_KEY)
//  3. YAP_SIGN_KEY env var
//  4. configKey (from yap.json signing.keyPath)
//  5. Default search in ~/.config/yap/keys/
//
// For passphrases, the resolution order is:
//  1. flagPass (CLI --sign-passphrase)
//  2. YAP_<FORMAT>_PASSPHRASE env var (e.g., YAP_DEB_PASSPHRASE)
//  3. YAP_SIGN_PASSPHRASE env var
//  4. configPass (from yap.json signing.passphrase)
//  5. Empty string (no passphrase)
//
// If signing is requested but no key can be resolved, an error is returned.
// If no key is found, signing is disabled and passphrase is cleared.
func Resolve(
	format Format,
	flagKey, flagPass, configKey, configPass string,
) (Config, error) {
	cfg := Config{}

	// Resolve key path
	keyPath, err := resolveKeyPath(format, flagKey, configKey)
	if err != nil {
		return cfg, err
	}

	cfg.KeyPath = keyPath
	cfg.Enabled = keyPath != ""

	// Only resolve passphrase if signing is enabled
	if cfg.Enabled {
		passphrase := resolvePassphrase(format, flagPass, configPass)
		cfg.Passphrase = passphrase
	}

	return cfg, nil
}

// ResolveGeneric produces a final Config without format-specific resolution.
// This is used at the project level where the actual artifact format is not yet known.
// Format-specific resolution happens later in signArtifact() for each artifact.
//
// For keys, the resolution order is:
//  1. flagKey (CLI --sign-key)
//  2. YAP_SIGN_KEY env var (global only, no format-specific vars)
//  3. configKey (from yap.json signing.keyPath)
//  4. Default search in ~/.config/yap/keys/ (tries both default.rsa and default.gpg)
//
// For passphrases, the resolution order is:
//  1. flagPass (CLI --sign-passphrase)
//  2. YAP_SIGN_PASSPHRASE env var (global only, no format-specific vars)
//  3. configPass (from yap.json signing.passphrase)
//  4. Empty string (no passphrase)
//
// If no key is found, signing is disabled and passphrase is cleared.
func ResolveGeneric(
	flagKey, flagPass, configKey, configPass string,
) (Config, error) {
	cfg := Config{}

	// Resolve key path (generic, no format-specific env vars)
	keyPath, err := resolveGenericKeyPath(flagKey, configKey)
	if err != nil {
		return cfg, err
	}

	cfg.KeyPath = keyPath
	cfg.Enabled = keyPath != ""

	// Only resolve passphrase if signing is enabled
	if cfg.Enabled {
		passphrase := resolveGenericPassphrase(flagPass, configPass)
		cfg.Passphrase = passphrase
	}

	return cfg, nil
}

// resolveGenericKeyPath applies the priority order for generic key path resolution
// (no format-specific env vars).
func resolveGenericKeyPath(flagKey, configKey string) (string, error) {
	// Priority 1: CLI flag
	if flagKey != "" {
		return resolveAndValidateKeyPath(flagKey, "CLI flag", "CLI flag")
	}

	// Priority 2: Global env var (YAP_SIGN_KEY) - skip format-specific vars
	if envVal := os.Getenv("YAP_SIGN_KEY"); envVal != "" {
		return resolveAndValidateKeyPath(envVal, "YAP_SIGN_KEY", "global env var")
	}

	// Priority 3: Project config
	if configKey != "" {
		return resolveAndValidateKeyPath(configKey, "yap.json", "project config")
	}

	// Priority 4: Default search in ~/.config/yap/keys/
	// Try both default.rsa and default.gpg since we don't know the format yet
	defaultPath, found := findGenericDefaultKey()
	if found {
		logger.Debug("Resolved signing key from default location",
			"key_path", defaultPath)

		return defaultPath, nil
	}

	// No key found; signing is disabled
	return "", nil
}

// resolveGenericPassphrase applies the priority order for generic passphrase resolution
// (no format-specific env vars).
func resolveGenericPassphrase(flagPass, configPass string) string {
	// Priority 1: CLI flag
	if flagPass != "" {
		logger.Debug("Resolved passphrase from CLI flag")
		return flagPass
	}

	// Priority 2: Global env var (YAP_SIGN_PASSPHRASE) - skip format-specific vars
	if envVal := os.Getenv("YAP_SIGN_PASSPHRASE"); envVal != "" {
		logger.Debug("Resolved passphrase from global env var",
			"env_var", "YAP_SIGN_PASSPHRASE")

		return envVal
	}

	// Priority 3: Project config
	if configPass != "" {
		logger.Debug("Resolved passphrase from project config")
		return configPass
	}

	// No passphrase found
	return ""
}

// findGenericDefaultKey searches ~/.config/yap/keys/ for a default key.
// Since format is unknown, it tries both default.rsa and default.gpg.
func findGenericDefaultKey() (string, bool) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		logger.Debug("Failed to get home directory", "error", err)
		return "", false
	}

	keysDir := filepath.Join(homeDir, ".config", "yap", "keys")

	// Check if keys directory exists
	if _, err := os.Stat(keysDir); err != nil {
		logger.Debug("Default keys directory not found",
			"path", keysDir)

		return "", false
	}

	// Try default.rsa first (APK uses RSA)
	defaultRSA := filepath.Join(keysDir, "default.rsa")
	if _, err := os.Stat(defaultRSA); err == nil {
		logger.Debug("Found default RSA key file",
			"path", defaultRSA)

		return defaultRSA, true
	}

	// Fall back to default.gpg (DEB/RPM/Pacman use GPG)
	defaultGPG := filepath.Join(keysDir, "default.gpg")
	if _, err := os.Stat(defaultGPG); err == nil {
		logger.Debug("Found default GPG key file",
			"path", defaultGPG)

		return defaultGPG, true
	}

	logger.Debug("No default key found",
		"keys_dir", keysDir)

	return "", false
}

// resolveKeyPath applies the priority order for key path resolution.
func resolveKeyPath(format Format, flagKey, configKey string) (string, error) {
	// Priority 1: CLI flag
	if flagKey != "" {
		return resolveAndValidateKeyPath(flagKey, "CLI flag", "CLI flag")
	}

	// Priority 2: Format-specific env var (e.g., YAP_DEB_KEY)
	envKey := fmt.Sprintf("YAP_%s_KEY", strings.ToUpper(string(format)))
	if envVal := os.Getenv(envKey); envVal != "" {
		return resolveAndValidateKeyPath(envVal, envKey, "format-specific env var")
	}

	// Priority 3: Global env var (YAP_SIGN_KEY)
	if envVal := os.Getenv("YAP_SIGN_KEY"); envVal != "" {
		return resolveAndValidateKeyPath(envVal, "YAP_SIGN_KEY", "global env var")
	}

	// Priority 4: Project config
	if configKey != "" {
		return resolveAndValidateKeyPath(configKey, "yap.json", "project config")
	}

	// Priority 5: Default search in ~/.config/yap/keys/
	defaultPath, found := findDefaultKey(format)
	if found {
		logger.Debug("Resolved signing key from default location",
			"key_path", defaultPath)

		return defaultPath, nil
	}

	// No key found; signing is disabled
	return "", nil
}

// resolvePassphrase applies the priority order for passphrase resolution.
func resolvePassphrase(format Format, flagPass, configPass string) string {
	// Priority 1: CLI flag
	if flagPass != "" {
		logger.Debug("Resolved passphrase from CLI flag")
		return flagPass
	}

	// Priority 2: Format-specific env var (e.g., YAP_DEB_PASSPHRASE)
	envKey := fmt.Sprintf("YAP_%s_PASSPHRASE", strings.ToUpper(string(format)))
	if envVal := os.Getenv(envKey); envVal != "" {
		logger.Debug("Resolved passphrase from format-specific env var",
			"env_var", envKey)

		return envVal
	}

	// Priority 3: Global env var (YAP_SIGN_PASSPHRASE)
	if envVal := os.Getenv("YAP_SIGN_PASSPHRASE"); envVal != "" {
		logger.Debug("Resolved passphrase from global env var",
			"env_var", "YAP_SIGN_PASSPHRASE")

		return envVal
	}

	// Priority 4: Project config
	if configPass != "" {
		logger.Debug("Resolved passphrase from project config")
		return configPass
	}

	// No passphrase found
	return ""
}

// findDefaultKey searches ~/.config/yap/keys/ for a key matching the format.
// It prefers format-specific files (e.g., apk.rsa, deb.gpg) and falls back
// to default.rsa or default.gpg based on the algorithm.
func findDefaultKey(format Format) (string, bool) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		logger.Debug("Failed to get home directory", "error", err)
		return "", false
	}

	keysDir := filepath.Join(homeDir, ".config", "yap", "keys")

	// Check if keys directory exists
	if _, err := os.Stat(keysDir); err != nil {
		logger.Debug("Default keys directory not found",
			"path", keysDir)

		return "", false
	}

	// Determine algorithm for this format
	algo := algorithmForFormat(format)

	// Try format-specific file first (e.g., apk.rsa, deb.gpg)
	formatSpecificFile := filepath.Join(keysDir,
		fmt.Sprintf("%s.%s", string(format), string(algo)))
	if _, err := os.Stat(formatSpecificFile); err == nil {
		logger.Debug("Found format-specific key file",
			"path", formatSpecificFile)

		return formatSpecificFile, true
	}

	// Fall back to default.<algo> (e.g., default.rsa, default.gpg)
	defaultFile := filepath.Join(keysDir,
		fmt.Sprintf("default.%s", string(algo)))
	if _, err := os.Stat(defaultFile); err == nil {
		logger.Debug("Found default key file",
			"path", defaultFile)

		return defaultFile, true
	}

	logger.Debug("No default key found for format",
		"format", string(format), "keys_dir", keysDir)

	return "", false
}

// algorithmForFormat returns the signing algorithm for a given format.
func algorithmForFormat(format Format) Algorithm {
	switch format {
	case FormatAPK:
		return AlgorithmRSA
	case FormatDEB, FormatRPM, FormatPacman:
		return AlgorithmGPG
	default:
		return AlgorithmGPG // Default to GPG for unknown formats
	}
}
