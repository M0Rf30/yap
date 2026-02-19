// Package i18n_test provides blackbox tests for the i18n package.
package i18n_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/M0Rf30/yap/v2/pkg/i18n"
)

// TestInit verifies that Init completes without error for known language codes.
func TestInit(t *testing.T) {
	tests := []struct {
		name string
		lang string
	}{
		{"english explicit", "en"},
		{"italian explicit", "it"},
		{"empty uses system default", ""},
		{"unsupported falls back to english", "zz"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := i18n.Init(tc.lang); err != nil {
				t.Errorf("Init(%q) returned error: %v", tc.lang, err)
			}
		})
	}
}

// TestT_KnownKey verifies that a known translation key returns its English translation.
func TestT_KnownKey(t *testing.T) {
	if err := i18n.Init("en"); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	tests := []struct {
		id       string
		wantSubs string // substring expected in the translation
	}{
		{"root.short", "Yet Another Packager"},
		{"groups.build", "Build"},
		{"messages.verbose_mode_enabled", "Verbose"},
		{"errors.build.build_stage_failed", "Build stage failed"},
		{"flags.verbose", "verbose"},
		{"logger.build.build_completed", "Build completed"},
		{"commands.build.short", "Build packages"},
	}

	for _, tc := range tests {
		t.Run(tc.id, func(t *testing.T) {
			got := i18n.T(tc.id)
			if got == "" {
				t.Errorf("T(%q) = empty string, want non-empty", tc.id)
			}

			if !strings.Contains(strings.ToLower(got), strings.ToLower(tc.wantSubs)) {
				t.Errorf("T(%q) = %q, want substring %q", tc.id, got, tc.wantSubs)
			}
		})
	}
}

// TestT_UnknownKey verifies that an unknown key returns the key itself as fallback.
// This is the documented behavior in i18n.go: "Return the message ID if translation fails".
func TestT_UnknownKey(t *testing.T) {
	if err := i18n.Init("en"); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	tests := []struct {
		key string
	}{
		{"this.key.does.not.exist"},
		{"unknown.message.id"},
		{"totally_bogus_key"},
	}

	for _, tc := range tests {
		t.Run(tc.key, func(t *testing.T) {
			got := i18n.T(tc.key)
			if got != tc.key {
				t.Errorf("T(%q) = %q, want key echoed back as fallback", tc.key, got)
			}
		})
	}
}

// TestT_EmptyKey verifies that an empty key returns itself (empty string) without panicking.
func TestT_EmptyKey(t *testing.T) {
	if err := i18n.Init("en"); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	got := i18n.T("")
	if got != "" {
		t.Errorf("T(\"\") = %q, want empty string", got)
	}
}

// TestT_WithTemplateData verifies that template data can be passed without panicking.
// The en.yaml translations use %s-style format strings (not Go templates), so the
// template data is ignored for most keys; the call must still succeed.
func TestT_WithTemplateData(t *testing.T) {
	if err := i18n.Init("en"); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	// Passing template data to a key without template placeholders should be safe.
	got := i18n.T("messages.verbose_mode_enabled", map[string]any{"Name": "test"})
	if got == "" {
		t.Error("T with template data returned empty string, want non-empty")
	}
}

// TestT_ItalianLocale verifies that translation works with Italian locale.
func TestT_ItalianLocale(t *testing.T) {
	if err := i18n.Init("it"); err != nil {
		t.Fatalf("Init(\"it\") failed: %v", err)
	}

	tests := []struct {
		id       string
		wantSubs string
	}{
		{"root.short", "Yet Another Packager"},         // brand name preserved
		{"groups.build", "Compilazione"},               // Italian translation
		{"commands.completion.short", "completamento"}, // Italian word for "completion"
	}

	for _, tc := range tests {
		t.Run(tc.id, func(t *testing.T) {
			got := i18n.T(tc.id)
			if got == "" {
				t.Errorf("T(%q) with Italian locale = empty string", tc.id)
			}

			if !strings.Contains(got, tc.wantSubs) {
				t.Errorf("T(%q) with Italian locale = %q, want substring %q", tc.id, got, tc.wantSubs)
			}
		})
	}

	// Restore English for other tests.
	if err := i18n.Init("en"); err != nil {
		t.Fatalf("could not restore English locale: %v", err)
	}
}

// TestCheckIntegrity verifies that all embedded locale files are self-consistent.
func TestCheckIntegrity(t *testing.T) {
	if err := i18n.CheckIntegrity(); err != nil {
		t.Errorf("CheckIntegrity() failed: %v", err)
	}
}

// TestGetMessageIDs verifies that GetMessageIDs returns a non-empty sorted list.
func TestGetMessageIDs(t *testing.T) {
	ids, err := i18n.GetMessageIDs()
	if err != nil {
		t.Fatalf("GetMessageIDs() error: %v", err)
	}

	if len(ids) == 0 {
		t.Error("GetMessageIDs() returned empty list")
	}

	// Verify sorted order.
	for i := 1; i < len(ids); i++ {
		if ids[i] < ids[i-1] {
			t.Errorf("GetMessageIDs() is not sorted at index %d: %q < %q", i, ids[i], ids[i-1])
		}
	}

	// Spot-check a known key is present.
	if !slices.Contains(ids, "root.short") {
		t.Error("GetMessageIDs() result missing known key 'root.short'")
	}
}

// TestSupportedLanguages verifies that the SupportedLanguages variable contains
// at least English and Italian.
func TestSupportedLanguages(t *testing.T) {
	hasEn, hasIt := false, false

	for _, lang := range i18n.SupportedLanguages {
		switch lang {
		case "en":
			hasEn = true
		case "it":
			hasIt = true
		}
	}

	if !hasEn {
		t.Error("SupportedLanguages does not contain 'en'")
	}

	if !hasIt {
		t.Error("SupportedLanguages does not contain 'it'")
	}
}
