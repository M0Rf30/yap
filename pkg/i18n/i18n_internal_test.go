// Package i18n provides internationalization support for YAP.
// This file contains whitebox tests that require access to unexported symbols.
package i18n

import (
	"sync"
	"testing"
)

// TestT_BeforeInit verifies that T works even before Init is called.
// The documented fallback in i18n.go is to return the messageID when localizer is nil.
func TestT_BeforeInit(t *testing.T) {
	// Whitebox test: directly mutates the package-level localizer.
	// Requires sequential test execution (-p 1); not safe for t.Parallel().
	origLocalizer := localizer
	localizer = nil

	defer func() { localizer = origLocalizer }()

	key := "any.key"
	got := T(key)

	if got != key {
		t.Errorf("T(%q) before Init = %q, want key echoed back", key, got)
	}
}

// TestDetectSystemLanguagePrecedence verifies POSIX/GNU locale precedence.
func TestDetectSystemLanguagePrecedence(t *testing.T) {
	tests := []struct {
		name     string
		env      map[string]string
		expected string
	}{
		{"LANG only italian", map[string]string{"LANG": "it_IT.UTF-8"}, "it"},
		{"LC_ALL beats LANG", map[string]string{"LANG": "it_IT.UTF-8", "LC_ALL": "en_US.UTF-8"}, "en"},
		{"LC_ALL=C beats LANG", map[string]string{"LANG": "it_IT.UTF-8", "LC_ALL": "C"}, "en"},
		{"LC_MESSAGES beats LANG", map[string]string{"LANG": "it_IT", "LC_MESSAGES": "en_GB"}, "en"},
		{"LC_ALL beats LC_MESSAGES", map[string]string{"LC_ALL": "it_IT", "LC_MESSAGES": "en_GB"}, "it"},
		{"LANGUAGE beats LANG", map[string]string{"LANG": "en_US.UTF-8", "LANGUAGE": "fr:it:en"}, "it"},
		{"LANGUAGE ignored for C", map[string]string{"LC_ALL": "C", "LANGUAGE": "it"}, "en"},
		{"unsupported falls back", map[string]string{"LANG": "fr_FR.UTF-8"}, "en"},
		{"modifier stripped", map[string]string{"LANG": "it@euro"}, "it"},
		{"nothing set", map[string]string{}, "en"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for _, k := range []string{"LC_ALL", "LC_MESSAGES", "LANG", "LANGUAGE"} {
				t.Setenv(k, "")
			}

			for k, v := range tc.env {
				t.Setenv(k, v)
			}

			if got := detectSystemLanguage(); got != tc.expected {
				t.Errorf("detectSystemLanguage() = %q, want %q", got, tc.expected)
			}
		})
	}
}

// TestInitConcurrentWithT exercises Init/T concurrently (run with -race).
func TestInitConcurrentWithT(t *testing.T) {
	var wg sync.WaitGroup

	for range 4 {
		wg.Go(func() {
			for range 20 {
				_ = Init("en")
			}
		})
		wg.Go(func() {
			for range 200 {
				_ = T("any.key")
			}
		})
	}

	wg.Wait()
}
