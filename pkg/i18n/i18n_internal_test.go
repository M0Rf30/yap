// Package i18n provides internationalization support for YAP.
// This file contains whitebox tests that require access to unexported symbols.
package i18n

import "testing"

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
