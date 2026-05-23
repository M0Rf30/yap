package signing

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Tests for internal functions in resolve.go that need package-level access.
// These tests are in the signing package (not signing_test) to access unexported functions.

// TestResolveAndValidateKeyPath tests the resolveAndValidateKeyPath function.
func TestResolveAndValidateKeyPath(t *testing.T) {
	tmpDir := t.TempDir()
	validKeyPath := filepath.Join(tmpDir, "valid.key")

	// Create a valid key file
	err := os.WriteFile(validKeyPath, []byte("test key content"), 0o600)
	require.NoError(t, err)

	tests := []struct {
		name        string
		rawPath     string
		source      string
		sourceLabel string
		wantErr     bool
		wantPath    string
	}{
		{
			name:        "valid key file",
			rawPath:     validKeyPath,
			source:      "test",
			sourceLabel: "test source",
			wantErr:     false,
			wantPath:    validKeyPath,
		},
		{
			name:        "nonexistent key file",
			rawPath:     filepath.Join(tmpDir, "nonexistent.key"),
			source:      "test",
			sourceLabel: "test source",
			wantErr:     true,
		},
		{
			name:        "relative path converted to absolute",
			rawPath:     ".",
			source:      "test",
			sourceLabel: "test source",
			wantErr:     false,
			wantPath:    "", // Will be current directory, just check no error
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, err := resolveAndValidateKeyPath(tt.rawPath, tt.source, tt.sourceLabel)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)

				if tt.wantPath != "" {
					assert.Equal(t, tt.wantPath, path)
				}
				// Verify path is absolute
				assert.True(t, filepath.IsAbs(path))
			}
		})
	}
}

// TestResolveKeyPathComprehensive tests the resolveKeyPath function with comprehensive priority order.
func TestResolveKeyPathComprehensive(t *testing.T) {
	tmpDir := t.TempDir()
	cliKeyPath := filepath.Join(tmpDir, "cli.key")
	envKeyPath := filepath.Join(tmpDir, "env.key")
	configKeyPath := filepath.Join(tmpDir, "config.key")

	// Create all key files
	for _, path := range []string{cliKeyPath, envKeyPath, configKeyPath} {
		err := os.WriteFile(path, []byte("test key"), 0o600)
		require.NoError(t, err)
	}

	tests := []struct {
		name      string
		format    Format
		flagKey   string
		configKey string
		envVars   map[string]string
		wantPath  string
		wantErr   bool
	}{
		{
			name:      "CLI flag takes priority over all",
			format:    FormatDEB,
			flagKey:   cliKeyPath,
			configKey: configKeyPath,
			envVars: map[string]string{
				"YAP_DEB_KEY":  envKeyPath,
				"YAP_SIGN_KEY": envKeyPath,
			},
			wantPath: cliKeyPath,
			wantErr:  false,
		},
		{
			name:      "format-specific env var takes priority over global env",
			format:    FormatDEB,
			flagKey:   "",
			configKey: configKeyPath,
			envVars: map[string]string{
				"YAP_DEB_KEY":  envKeyPath,
				"YAP_SIGN_KEY": filepath.Join(tmpDir, "other.key"),
			},
			wantPath: envKeyPath,
			wantErr:  false,
		},
		{
			name:      "global env var used when no format-specific var",
			format:    FormatRPM,
			flagKey:   "",
			configKey: configKeyPath,
			envVars: map[string]string{
				"YAP_SIGN_KEY": envKeyPath,
			},
			wantPath: envKeyPath,
			wantErr:  false,
		},
		{
			name:      "config key used when no env vars",
			format:    FormatAPK,
			flagKey:   "",
			configKey: configKeyPath,
			envVars:   map[string]string{},
			wantPath:  configKeyPath,
			wantErr:   false,
		},
		{
			name:      "no key found returns empty string",
			format:    FormatPacman,
			flagKey:   "",
			configKey: "",
			envVars:   map[string]string{},
			wantPath:  "",
			wantErr:   false,
		},
		{
			name:      "invalid CLI flag returns error",
			format:    FormatDEB,
			flagKey:   filepath.Join(tmpDir, "nonexistent.key"),
			configKey: configKeyPath,
			envVars:   map[string]string{},
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment variables
			for key, val := range tt.envVars {
				t.Setenv(key, val)
			}

			// Clear any other env vars that might interfere
			if _, ok := tt.envVars["YAP_DEB_KEY"]; !ok {
				t.Setenv("YAP_DEB_KEY", "")
			}

			if _, ok := tt.envVars["YAP_RPM_KEY"]; !ok {
				t.Setenv("YAP_RPM_KEY", "")
			}

			if _, ok := tt.envVars["YAP_APK_KEY"]; !ok {
				t.Setenv("YAP_APK_KEY", "")
			}

			if _, ok := tt.envVars["YAP_PACMAN_KEY"]; !ok {
				t.Setenv("YAP_PACMAN_KEY", "")
			}

			if _, ok := tt.envVars["YAP_SIGN_KEY"]; !ok {
				t.Setenv("YAP_SIGN_KEY", "")
			}

			path, err := resolveKeyPath(tt.format, tt.flagKey, tt.configKey)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantPath, path)
			}
		})
	}
}

// TestResolvePassphraseComprehensive tests the resolvePassphrase function with comprehensive priority order.
func TestResolvePassphraseComprehensive(t *testing.T) {
	tests := []struct {
		name       string
		format     Format
		flagPass   string
		configPass string
		envVars    map[string]string
		wantPass   string
	}{
		{
			name:       "CLI flag takes priority",
			format:     FormatDEB,
			flagPass:   "cli-pass",
			configPass: "config-pass",
			envVars: map[string]string{
				"YAP_DEB_PASSPHRASE":  "env-pass",
				"YAP_SIGN_PASSPHRASE": "global-pass",
			},
			wantPass: "cli-pass",
		},
		{
			name:       "format-specific env var takes priority over global",
			format:     FormatDEB,
			flagPass:   "",
			configPass: "config-pass",
			envVars: map[string]string{
				"YAP_DEB_PASSPHRASE":  "env-pass",
				"YAP_SIGN_PASSPHRASE": "global-pass",
			},
			wantPass: "env-pass",
		},
		{
			name:       "global env var used when no format-specific var",
			format:     FormatRPM,
			flagPass:   "",
			configPass: "config-pass",
			envVars: map[string]string{
				"YAP_SIGN_PASSPHRASE": "global-pass",
			},
			wantPass: "global-pass",
		},
		{
			name:       "config passphrase used when no env vars",
			format:     FormatAPK,
			flagPass:   "",
			configPass: "config-pass",
			envVars:    map[string]string{},
			wantPass:   "config-pass",
		},
		{
			name:       "empty string when no passphrase found",
			format:     FormatPacman,
			flagPass:   "",
			configPass: "",
			envVars:    map[string]string{},
			wantPass:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment variables
			for key, val := range tt.envVars {
				t.Setenv(key, val)
			}

			// Clear any other env vars that might interfere
			if _, ok := tt.envVars["YAP_DEB_PASSPHRASE"]; !ok {
				t.Setenv("YAP_DEB_PASSPHRASE", "")
			}

			if _, ok := tt.envVars["YAP_RPM_PASSPHRASE"]; !ok {
				t.Setenv("YAP_RPM_PASSPHRASE", "")
			}

			if _, ok := tt.envVars["YAP_APK_PASSPHRASE"]; !ok {
				t.Setenv("YAP_APK_PASSPHRASE", "")
			}

			if _, ok := tt.envVars["YAP_PACMAN_PASSPHRASE"]; !ok {
				t.Setenv("YAP_PACMAN_PASSPHRASE", "")
			}

			if _, ok := tt.envVars["YAP_SIGN_PASSPHRASE"]; !ok {
				t.Setenv("YAP_SIGN_PASSPHRASE", "")
			}

			pass := resolvePassphrase(tt.format, tt.flagPass, tt.configPass)
			assert.Equal(t, tt.wantPass, pass)
		})
	}
}

// TestFindDefaultKeyForFormat tests the findDefaultKey function for each format.
func TestFindDefaultKeyForFormat(t *testing.T) {
	// Create a temporary keys directory structure
	tmpDir := t.TempDir()
	keysDir := filepath.Join(tmpDir, ".config", "yap", "keys")
	err := os.MkdirAll(keysDir, 0o755)
	require.NoError(t, err)

	// Create test key files
	defaultRSA := filepath.Join(keysDir, "default.rsa")
	defaultGPG := filepath.Join(keysDir, "default.gpg")
	apkRSA := filepath.Join(keysDir, "apk.rsa")
	debGPG := filepath.Join(keysDir, "deb.gpg")

	err = os.WriteFile(defaultRSA, []byte("rsa key"), 0o600)
	require.NoError(t, err)
	err = os.WriteFile(defaultGPG, []byte("gpg key"), 0o600)
	require.NoError(t, err)
	err = os.WriteFile(apkRSA, []byte("apk rsa key"), 0o600)
	require.NoError(t, err)
	err = os.WriteFile(debGPG, []byte("deb gpg key"), 0o600)
	require.NoError(t, err)

	tests := []struct {
		name     string
		format   Format
		wantPath string
		wantOK   bool
		setup    func()
	}{
		{
			name:     "format-specific key takes priority (APK)",
			format:   FormatAPK,
			wantPath: apkRSA,
			wantOK:   true,
			setup:    func() {},
		},
		{
			name:     "format-specific key takes priority (DEB)",
			format:   FormatDEB,
			wantPath: debGPG,
			wantOK:   true,
			setup:    func() {},
		},
		{
			name:     "default RSA used for APK when no format-specific key",
			format:   FormatAPK,
			wantPath: defaultRSA,
			wantOK:   true,
			setup: func() {
				// Remove format-specific key
				_ = os.Remove(apkRSA)
			},
		},
		{
			name:     "default GPG used for DEB when no format-specific key",
			format:   FormatDEB,
			wantPath: defaultGPG,
			wantOK:   true,
			setup: func() {
				// Remove format-specific key
				_ = os.Remove(debGPG)
			},
		},
		{
			name:     "no key found returns false",
			format:   FormatRPM,
			wantPath: "",
			wantOK:   false,
			setup: func() {
				// Remove all keys
				_ = os.Remove(defaultRSA)
				_ = os.Remove(defaultGPG)
				_ = os.Remove(apkRSA)
				_ = os.Remove(debGPG)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset keys directory for each test
			require.NoError(t, os.RemoveAll(keysDir))
			require.NoError(t, os.MkdirAll(keysDir, 0o755))
			require.NoError(t, os.WriteFile(defaultRSA, []byte("rsa key"), 0o600))
			require.NoError(t, os.WriteFile(defaultGPG, []byte("gpg key"), 0o600))
			require.NoError(t, os.WriteFile(apkRSA, []byte("apk rsa key"), 0o600))
			require.NoError(t, os.WriteFile(debGPG, []byte("deb gpg key"), 0o600))

			tt.setup()

			// Mock home directory
			oldHome := os.Getenv("HOME")

			t.Setenv("HOME", tmpDir)

			defer func() {
				if oldHome != "" {
					t.Setenv("HOME", oldHome)
				}
			}()

			path, ok := findDefaultKey(tt.format)
			assert.Equal(t, tt.wantOK, ok)

			if tt.wantOK {
				assert.Equal(t, tt.wantPath, path)
			}
		})
	}
}

// TestFindGenericDefaultKey tests the findGenericDefaultKey function.
func TestFindGenericDefaultKey(t *testing.T) {
	tmpDir := t.TempDir()
	keysDir := filepath.Join(tmpDir, ".config", "yap", "keys")
	err := os.MkdirAll(keysDir, 0o755)
	require.NoError(t, err)

	tests := []struct {
		name     string
		wantPath string
		wantOK   bool
		setup    func()
	}{
		{
			name:     "default.rsa found and returned",
			wantPath: filepath.Join(keysDir, "default.rsa"),
			wantOK:   true,
			setup: func() {
				_ = os.WriteFile(filepath.Join(keysDir, "default.rsa"), []byte("rsa key"), 0o600)
			},
		},
		{
			name:     "default.gpg used when default.rsa not found",
			wantPath: filepath.Join(keysDir, "default.gpg"),
			wantOK:   true,
			setup: func() {
				_ = os.WriteFile(filepath.Join(keysDir, "default.gpg"), []byte("gpg key"), 0o600)
			},
		},
		{
			name:     "default.rsa takes priority over default.gpg",
			wantPath: filepath.Join(keysDir, "default.rsa"),
			wantOK:   true,
			setup: func() {
				_ = os.WriteFile(filepath.Join(keysDir, "default.rsa"), []byte("rsa key"), 0o600)
				_ = os.WriteFile(filepath.Join(keysDir, "default.gpg"), []byte("gpg key"), 0o600)
			},
		},
		{
			name:     "no key found returns false",
			wantPath: "",
			wantOK:   false,
			setup:    func() {},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset keys directory
			require.NoError(t, os.RemoveAll(keysDir))
			require.NoError(t, os.MkdirAll(keysDir, 0o755))

			tt.setup()

			// Mock home directory
			oldHome := os.Getenv("HOME")

			t.Setenv("HOME", tmpDir)

			defer func() {
				if oldHome != "" {
					t.Setenv("HOME", oldHome)
				}
			}()

			path, ok := findGenericDefaultKey()
			assert.Equal(t, tt.wantOK, ok)

			if tt.wantOK {
				assert.Equal(t, tt.wantPath, path)
			}
		})
	}
}

// TestResolveGenericKeyPath tests the resolveGenericKeyPath function.
func TestResolveGenericKeyPath(t *testing.T) {
	tmpDir := t.TempDir()
	cliKeyPath := filepath.Join(tmpDir, "cli.key")
	configKeyPath := filepath.Join(tmpDir, "config.key")

	// Create key files
	for _, path := range []string{cliKeyPath, configKeyPath} {
		err := os.WriteFile(path, []byte("test key"), 0o600)
		require.NoError(t, err)
	}

	tests := []struct {
		name      string
		flagKey   string
		configKey string
		envVars   map[string]string
		wantPath  string
		wantErr   bool
	}{
		{
			name:      "CLI flag takes priority",
			flagKey:   cliKeyPath,
			configKey: configKeyPath,
			envVars: map[string]string{
				"YAP_SIGN_KEY": filepath.Join(tmpDir, "other.key"),
			},
			wantPath: cliKeyPath,
			wantErr:  false,
		},
		{
			name:      "global env var used when no CLI flag",
			flagKey:   "",
			configKey: configKeyPath,
			envVars: map[string]string{
				"YAP_SIGN_KEY": cliKeyPath,
			},
			wantPath: cliKeyPath,
			wantErr:  false,
		},
		{
			name:      "config key used when no env vars",
			flagKey:   "",
			configKey: configKeyPath,
			envVars:   map[string]string{},
			wantPath:  configKeyPath,
			wantErr:   false,
		},
		{
			name:      "no key found returns empty string",
			flagKey:   "",
			configKey: "",
			envVars:   map[string]string{},
			wantPath:  "",
			wantErr:   false,
		},
		{
			name:      "invalid CLI flag returns error",
			flagKey:   filepath.Join(tmpDir, "nonexistent.key"),
			configKey: configKeyPath,
			envVars:   map[string]string{},
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment variables
			for key, val := range tt.envVars {
				t.Setenv(key, val)
			}

			// Clear YAP_SIGN_KEY if not in envVars
			if _, ok := tt.envVars["YAP_SIGN_KEY"]; !ok {
				t.Setenv("YAP_SIGN_KEY", "")
			}

			path, err := resolveGenericKeyPath(tt.flagKey, tt.configKey)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantPath, path)
			}
		})
	}
}

// TestResolveGenericPassphrase tests the resolveGenericPassphrase function.
func TestResolveGenericPassphrase(t *testing.T) {
	tests := []struct {
		name       string
		flagPass   string
		configPass string
		envVars    map[string]string
		wantPass   string
	}{
		{
			name:       "CLI flag takes priority",
			flagPass:   "cli-pass",
			configPass: "config-pass",
			envVars: map[string]string{
				"YAP_SIGN_PASSPHRASE": "global-pass",
			},
			wantPass: "cli-pass",
		},
		{
			name:       "global env var used when no CLI flag",
			flagPass:   "",
			configPass: "config-pass",
			envVars: map[string]string{
				"YAP_SIGN_PASSPHRASE": "global-pass",
			},
			wantPass: "global-pass",
		},
		{
			name:       "config passphrase used when no env vars",
			flagPass:   "",
			configPass: "config-pass",
			envVars:    map[string]string{},
			wantPass:   "config-pass",
		},
		{
			name:       "empty string when no passphrase found",
			flagPass:   "",
			configPass: "",
			envVars:    map[string]string{},
			wantPass:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment variables
			for key, val := range tt.envVars {
				t.Setenv(key, val)
			}

			// Clear YAP_SIGN_PASSPHRASE if not in envVars
			if _, ok := tt.envVars["YAP_SIGN_PASSPHRASE"]; !ok {
				t.Setenv("YAP_SIGN_PASSPHRASE", "")
			}

			pass := resolveGenericPassphrase(tt.flagPass, tt.configPass)
			assert.Equal(t, tt.wantPass, pass)
		})
	}
}

// TestResolvePublicAPI tests the public Resolve function with various scenarios.
func TestResolvePublicAPI(t *testing.T) {
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "test.key")
	err := os.WriteFile(keyPath, []byte("test key"), 0o600)
	require.NoError(t, err)

	tests := []struct {
		name           string
		format         Format
		flagKey        string
		flagPass       string
		configKey      string
		configPass     string
		envVars        map[string]string
		wantEnabled    bool
		wantKeyPath    string
		wantPassphrase string
		wantErr        bool
	}{
		{
			name:           "signing enabled with all parameters",
			format:         FormatDEB,
			flagKey:        keyPath,
			flagPass:       "test-pass",
			configKey:      "",
			configPass:     "",
			envVars:        map[string]string{},
			wantEnabled:    true,
			wantKeyPath:    keyPath,
			wantPassphrase: "test-pass",
			wantErr:        false,
		},
		{
			name:           "signing disabled when no key found",
			format:         FormatAPK,
			flagKey:        "",
			flagPass:       "test-pass",
			configKey:      "",
			configPass:     "",
			envVars:        map[string]string{},
			wantEnabled:    false,
			wantKeyPath:    "",
			wantPassphrase: "",
			wantErr:        false,
		},
		{
			name:           "passphrase cleared when signing disabled",
			format:         FormatRPM,
			flagKey:        "",
			flagPass:       "should-be-cleared",
			configKey:      "",
			configPass:     "",
			envVars:        map[string]string{},
			wantEnabled:    false,
			wantKeyPath:    "",
			wantPassphrase: "",
			wantErr:        false,
		},
		{
			name:           "error when CLI key doesn't exist",
			format:         FormatPacman,
			flagKey:        filepath.Join(tmpDir, "nonexistent.key"),
			flagPass:       "",
			configKey:      "",
			configPass:     "",
			envVars:        map[string]string{},
			wantEnabled:    false,
			wantKeyPath:    "",
			wantPassphrase: "",
			wantErr:        true,
		},
		{
			name:           "config key and passphrase used",
			format:         FormatDEB,
			flagKey:        "",
			flagPass:       "",
			configKey:      keyPath,
			configPass:     "config-pass",
			envVars:        map[string]string{},
			wantEnabled:    true,
			wantKeyPath:    keyPath,
			wantPassphrase: "config-pass",
			wantErr:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment variables
			for key, val := range tt.envVars {
				t.Setenv(key, val)
			}

			// Clear format-specific env vars
			t.Setenv("YAP_DEB_KEY", "")
			t.Setenv("YAP_RPM_KEY", "")
			t.Setenv("YAP_APK_KEY", "")
			t.Setenv("YAP_PACMAN_KEY", "")
			t.Setenv("YAP_SIGN_KEY", "")
			t.Setenv("YAP_DEB_PASSPHRASE", "")
			t.Setenv("YAP_RPM_PASSPHRASE", "")
			t.Setenv("YAP_APK_PASSPHRASE", "")
			t.Setenv("YAP_PACMAN_PASSPHRASE", "")
			t.Setenv("YAP_SIGN_PASSPHRASE", "")

			cfg, err := Resolve(tt.format, tt.flagKey, tt.flagPass, tt.configKey, tt.configPass)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantEnabled, cfg.Enabled)
				assert.Equal(t, tt.wantKeyPath, cfg.KeyPath)
				assert.Equal(t, tt.wantPassphrase, cfg.Passphrase)
			}
		})
	}
}

// TestResolveGenericPublicAPI tests the public ResolveGeneric function.
func TestResolveGenericPublicAPI(t *testing.T) {
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "test.key")
	err := os.WriteFile(keyPath, []byte("test key"), 0o600)
	require.NoError(t, err)

	tests := []struct {
		name           string
		flagKey        string
		flagPass       string
		configKey      string
		configPass     string
		envVars        map[string]string
		wantEnabled    bool
		wantKeyPath    string
		wantPassphrase string
		wantErr        bool
	}{
		{
			name:           "signing enabled with CLI key",
			flagKey:        keyPath,
			flagPass:       "test-pass",
			configKey:      "",
			configPass:     "",
			envVars:        map[string]string{},
			wantEnabled:    true,
			wantKeyPath:    keyPath,
			wantPassphrase: "test-pass",
			wantErr:        false,
		},
		{
			name:           "signing disabled when no key found",
			flagKey:        "",
			flagPass:       "test-pass",
			configKey:      "",
			configPass:     "",
			envVars:        map[string]string{},
			wantEnabled:    false,
			wantKeyPath:    "",
			wantPassphrase: "",
			wantErr:        false,
		},
		{
			name:           "global env var used",
			flagKey:        "",
			flagPass:       "",
			configKey:      "",
			configPass:     "",
			envVars:        map[string]string{"YAP_SIGN_KEY": keyPath},
			wantEnabled:    true,
			wantKeyPath:    keyPath,
			wantPassphrase: "",
			wantErr:        false,
		},
		{
			name:           "error when CLI key doesn't exist",
			flagKey:        filepath.Join(tmpDir, "nonexistent.key"),
			flagPass:       "",
			configKey:      "",
			configPass:     "",
			envVars:        map[string]string{},
			wantEnabled:    false,
			wantKeyPath:    "",
			wantPassphrase: "",
			wantErr:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear global env vars first
			t.Setenv("YAP_SIGN_KEY", "")
			t.Setenv("YAP_SIGN_PASSPHRASE", "")

			// Then set environment variables from test case
			for key, val := range tt.envVars {
				t.Setenv(key, val)
			}

			cfg, err := ResolveGeneric(tt.flagKey, tt.flagPass, tt.configKey, tt.configPass)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantEnabled, cfg.Enabled)
				assert.Equal(t, tt.wantKeyPath, cfg.KeyPath)
				assert.Equal(t, tt.wantPassphrase, cfg.Passphrase)
			}
		})
	}
}
