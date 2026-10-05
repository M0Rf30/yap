package signing

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/M0Rf30/yap/v2/pkg/errors"
	"github.com/M0Rf30/yap/v2/pkg/i18n"
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

	if _, err := os.Stat(absPath); err != nil { // #nosec G703 -- user-supplied key path by design
		return "", errors.Wrap(err, errors.ErrTypeFileSystem,
			"key file not found").
			WithOperation("resolveAndValidateKeyPath").
			WithContext("source", source).
			WithContext("key_path", absPath)
	}

	logger.Debug(i18n.T("logger.signing.debug.resolved_signing_key"),
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
// Per-format selection is applied later by ForFormat (called by NewSigner).
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
		logger.Debug(i18n.T("logger.signing.debug.resolved_signing_key_default"), "key_path", defaultPath)

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
		logger.Debug(i18n.T("logger.signing.debug.resolved_passphrase_cli_flag"))
		return flagPass
	}

	// Priority 2: Global env var (YAP_SIGN_PASSPHRASE) - skip format-specific vars
	if envVal := os.Getenv("YAP_SIGN_PASSPHRASE"); envVal != "" {
		logger.Debug(i18n.T("logger.signing.debug.resolved_passphrase_global_env"), "env_var", "YAP_SIGN_PASSPHRASE")

		return envVal
	}

	// Priority 3: Project config
	if configPass != "" {
		logger.Debug(i18n.T("logger.signing.debug.resolved_passphrase_project_config"))
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
		logger.Debug(i18n.T("logger.signing.debug.failed_get_home_directory"), "error", err)
		return "", false
	}

	keysDir := filepath.Join(homeDir, ".config", "yap", "keys")

	// Check if keys directory exists
	if _, err := os.Stat(keysDir); err != nil {
		logger.Debug(i18n.T("logger.signing.debug.default_keys_directory_not"), "path", keysDir)

		return "", false
	}

	// Try default.rsa first (APK uses RSA)
	defaultRSA := filepath.Join(keysDir, "default.rsa")
	if _, err := os.Stat(defaultRSA); err == nil {
		logger.Debug(i18n.T("logger.signing.debug.found_default_rsa_key"), "path", defaultRSA)

		return defaultRSA, true
	}

	// Fall back to default.gpg (DEB/RPM/Pacman use GPG)
	defaultGPG := filepath.Join(keysDir, "default.gpg")
	if _, err := os.Stat(defaultGPG); err == nil {
		logger.Debug(i18n.T("logger.signing.debug.found_default_gpg_key"), "path", defaultGPG)

		return defaultGPG, true
	}

	logger.Debug(i18n.T("logger.signing.debug.no_default_key_found"), "keys_dir", keysDir)

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
		logger.Debug(i18n.T("logger.signing.debug.resolved_signing_key_default"), "key_path", defaultPath)

		return defaultPath, nil
	}

	// No key found; signing is disabled
	return "", nil
}

// resolvePassphrase applies the priority order for passphrase resolution.
func resolvePassphrase(format Format, flagPass, configPass string) string {
	// Priority 1: CLI flag
	if flagPass != "" {
		logger.Debug(i18n.T("logger.signing.debug.resolved_passphrase_cli_flag"))
		return flagPass
	}

	// Priority 2: Format-specific env var (e.g., YAP_DEB_PASSPHRASE)
	envKey := fmt.Sprintf("YAP_%s_PASSPHRASE", strings.ToUpper(string(format)))
	if envVal := os.Getenv(envKey); envVal != "" {
		logger.Debug(i18n.T("logger.signing.debug.resolved_passphrase_format_specific"), "env_var", envKey)

		return envVal
	}

	// Priority 3: Global env var (YAP_SIGN_PASSPHRASE)
	if envVal := os.Getenv("YAP_SIGN_PASSPHRASE"); envVal != "" {
		logger.Debug(i18n.T("logger.signing.debug.resolved_passphrase_global_env"), "env_var", "YAP_SIGN_PASSPHRASE")

		return envVal
	}

	// Priority 4: Project config
	if configPass != "" {
		logger.Debug(i18n.T("logger.signing.debug.resolved_passphrase_project_config"))
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
		logger.Debug(i18n.T("logger.signing.debug.failed_get_home_directory"), "error", err)
		return "", false
	}

	keysDir := filepath.Join(homeDir, ".config", "yap", "keys")

	// Check if keys directory exists
	if _, err := os.Stat(keysDir); err != nil {
		logger.Debug(i18n.T("logger.signing.debug.default_keys_directory_not"), "path", keysDir)

		return "", false
	}

	// Determine algorithm for this format
	algo := algorithmForFormat(format)

	// Try format-specific file first (e.g., apk.rsa, deb.gpg)
	formatSpecificFile := filepath.Join(keysDir,
		fmt.Sprintf("%s.%s", string(format), string(algo)))
	if _, err := os.Stat(formatSpecificFile); err == nil {
		logger.Debug(i18n.T("logger.signing.debug.found_format_specific_key"), "path", formatSpecificFile)

		return formatSpecificFile, true
	}

	// Fall back to default.<algo> (e.g., default.rsa, default.gpg)
	defaultFile := filepath.Join(keysDir,
		fmt.Sprintf("default.%s", string(algo)))
	if _, err := os.Stat(defaultFile); err == nil {
		logger.Debug(i18n.T("logger.signing.debug.found_default_key_file"), "path", defaultFile)

		return defaultFile, true
	}

	logger.Debug(i18n.T("logger.signing.debug.no_default_key_found_format"), "format", string(format), "keys_dir", keysDir)

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

// defaultKeysDir returns ~/.config/yap/keys, or "" when the home directory
// cannot be determined.
func defaultKeysDir() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	return filepath.Join(homeDir, ".config", "yap", "keys")
}

// isGenericDefaultKey reports whether keyPath is one of the keys that
// findGenericDefaultKey auto-discovers (default.rsa / default.gpg).
func isGenericDefaultKey(keyPath string) bool {
	dir := defaultKeysDir()
	if dir == "" || keyPath == "" {
		return false
	}

	clean := filepath.Clean(keyPath)

	return clean == filepath.Join(dir, "default."+string(AlgorithmRSA)) ||
		clean == filepath.Join(dir, "default."+string(AlgorithmGPG))
}

// isGlobalEnvKey reports whether keyPath is the key named by YAP_SIGN_KEY.
func isGlobalEnvKey(keyPath string) bool {
	envVal := os.Getenv("YAP_SIGN_KEY")
	if envVal == "" || keyPath == "" {
		return false
	}

	abs, err := filepath.Abs(envVal)
	if err != nil {
		return false
	}

	return abs == filepath.Clean(keyPath)
}

// ForFormat adapts a generic Config (see ResolveGeneric) to a concrete package
// format. Keys that were chosen implicitly (auto-discovered defaults or the
// global YAP_SIGN_KEY) are replaced by the format-specific selection:
//
//   - YAP_<FORMAT>_KEY / YAP_<FORMAT>_PASSPHRASE take precedence over the
//     global values;
//   - an auto-discovered default key is re-resolved with findDefaultKey so
//     that e.g. a .deb never receives default.rsa (and vice versa).
//
// Keys given explicitly (CLI flag, project config) are left untouched. If an
// auto-discovered default has no counterpart for the format, signing is
// disabled for that artifact.
func ForFormat(format Format, cfg Config) (Config, error) {
	if !cfg.Enabled || cfg.KeyPath == "" {
		return cfg, nil
	}

	implicit := isGenericDefaultKey(cfg.KeyPath) || isGlobalEnvKey(cfg.KeyPath)
	if !implicit {
		return cfg, nil
	}

	envKey := fmt.Sprintf("YAP_%s_KEY", strings.ToUpper(string(format)))
	if envVal := os.Getenv(envKey); envVal != "" {
		keyPath, err := resolveAndValidateKeyPath(envVal, envKey, "format-specific env var")
		if err != nil {
			return cfg, err
		}

		cfg.KeyPath = keyPath
	} else if isGenericDefaultKey(cfg.KeyPath) {
		keyPath, found := findDefaultKey(format)
		if !found {
			cfg.Clear()

			return cfg, nil
		}

		cfg.KeyPath = keyPath
	}

	passEnv := fmt.Sprintf("YAP_%s_PASSPHRASE", strings.ToUpper(string(format)))
	if envVal := os.Getenv(passEnv); envVal != "" &&
		(cfg.Passphrase == "" || cfg.Passphrase == os.Getenv("YAP_SIGN_PASSPHRASE")) {
		logger.Debug(i18n.T("logger.signing.debug.resolved_passphrase_format_specific"),
			"env_var", passEnv)

		cfg.Passphrase = envVal
	}

	return cfg, nil
}
