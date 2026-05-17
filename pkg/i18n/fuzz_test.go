package i18n_test

import (
	"os"
	"strings"
	"testing"

	"github.com/M0Rf30/yap/v2/pkg/i18n"
)

// FuzzTranslate tests i18n.T with arbitrary message IDs.
// Must never panic. Returns messageID if translation fails (which may be empty).
func FuzzTranslate(f *testing.F) {
	// Initialize i18n once for all fuzz tests
	_ = i18n.Init("en")

	// Seed corpus with various message IDs
	f.Add("errors.source.unsupported_source_type")
	f.Add("logger.skip_integrity_check_for")
	f.Add("logger.integrity_check_for")
	f.Add("errors.source.failed_to_open_file_for_hash")
	f.Add("errors.source.directory_not_supported")
	f.Add("errors.source.unsupported_hash_length")
	f.Add("errors.source.failed_to_copy_file")
	f.Add("errors.source.hash_verification_failed")
	f.Add("nonexistent.message.id")
	f.Add("message.with.many.dots.in.id")
	f.Add("message_with_underscores")
	f.Add("message-with-dashes")
	f.Add("UPPERCASE_MESSAGE_ID")
	f.Add("MixedCaseMessageId")
	f.Add("message with spaces")
	f.Add("message\nwith\nnewlines")
	f.Add("message\twith\ttabs")
	f.Add("message with special chars: !@#$%^&*()")
	f.Add(strings.Repeat("a", 10000))

	f.Fuzz(func(t *testing.T, messageID string) {
		// Skip empty messageID as it returns empty string
		if messageID == "" {
			t.Skip("Skipping empty messageID")
		}

		// Should not panic
		result := i18n.T(messageID)

		// Invariant: Must return non-empty string for non-empty messageID
		if result == "" {
			t.Errorf("T returned empty string for messageID: %q", messageID)
		}

		// If translation fails, should return the messageID itself
		if result != messageID {
			// Translation succeeded, result should be a valid string
			if strings.Contains(result, "\x00") {
				t.Errorf("T returned string with null bytes: %q", result)
			}
		}
	})
}

// FuzzTranslateWithData tests i18n.T with template data.
// Must never panic. Returns messageID if translation fails (which may be empty).
func FuzzTranslateWithData(f *testing.F) {
	_ = i18n.Init("en")

	f.Add("message.id", "key", "value")
	f.Add("test.message", "count", "42")
	f.Add("test.message", "name", "John Doe")
	f.Add("test.message", "path", "/very/long/path/with/many/segments")
	f.Add("nonexistent.message", "key", "value")

	f.Fuzz(func(t *testing.T, messageID, key, value string) {
		// Skip empty messageID as it returns empty string
		if messageID == "" {
			t.Skip("Skipping empty messageID")
		}

		// Should not panic
		result := i18n.T(messageID, map[string]any{key: value})

		// Invariant: Must return non-empty string for non-empty messageID
		if result == "" {
			t.Errorf("T with data returned empty string for messageID: %q", messageID)
		}
	})
}

// FuzzDetectLanguage tests language detection with arbitrary LANG env values.
// Must never panic.
func FuzzDetectLanguage(f *testing.F) {
	// Seed corpus with various LANG values
	f.Add("en_US.UTF-8")
	f.Add("it_IT.UTF-8")
	f.Add("fr_FR.UTF-8")
	f.Add("de_DE.UTF-8")
	f.Add("es_ES.UTF-8")
	f.Add("pt_BR.UTF-8")
	f.Add("ja_JP.UTF-8")
	f.Add("zh_CN.UTF-8")
	f.Add("ru_RU.UTF-8")
	f.Add("en")
	f.Add("it")
	f.Add("C")
	f.Add("C.UTF-8")
	f.Add("POSIX")
	f.Add("")
	f.Add("invalid_lang")
	f.Add("en_US")
	f.Add("en_US.ISO-8859-1")
	f.Add("en_US.UTF-8@euro")
	f.Add("en_US_POSIX")
	f.Add(strings.Repeat("a", 1000))

	f.Fuzz(func(t *testing.T, langValue string) {
		// Save original LANG
		originalLang := os.Getenv("LANG")
		defer func() {
			if originalLang != "" {
				_ = os.Setenv("LANG", originalLang)
			} else {
				_ = os.Unsetenv("LANG")
			}
		}()

		// Set LANG to fuzz value
		if langValue != "" {
			_ = os.Setenv("LANG", langValue)
		} else {
			_ = os.Unsetenv("LANG")
		}

		// Should not panic
		err := i18n.Init("")

		// Init should not return an error for language detection
		if err != nil {
			t.Logf("Init returned error for LANG=%q: %v", langValue, err)
		}

		// After Init, T should still work
		result := i18n.T("test.message")
		if result == "" {
			t.Errorf("T returned empty string after Init with LANG=%q", langValue)
		}
	})
}

// FuzzInitWithLanguage tests Init with arbitrary language codes.
// Must never panic.
func FuzzInitWithLanguage(f *testing.F) {
	f.Add("en")
	f.Add("it")
	f.Add("fr")
	f.Add("de")
	f.Add("es")
	f.Add("")
	f.Add("invalid")
	f.Add("en_US")
	f.Add("en-US")
	f.Add("en_US.UTF-8")
	f.Add("C")
	f.Add("POSIX")
	f.Add("xx")
	f.Add("zz")
	f.Add(strings.Repeat("a", 1000))

	f.Fuzz(func(t *testing.T, lang string) {
		// Should not panic
		err := i18n.Init(lang)

		// Init may return an error, but should not panic
		if err != nil {
			t.Logf("Init returned error for lang=%q: %v", lang, err)
		}

		// After Init, T should work
		result := i18n.T("test.message")
		if result == "" {
			t.Errorf("T returned empty string after Init with lang=%q", lang)
		}
	})
}

// FuzzTranslateMultipleLanguages tests switching between languages.
// Must never panic.
func FuzzTranslateMultipleLanguages(f *testing.F) {
	f.Add("en", "it")
	f.Add("it", "en")
	f.Add("en", "en")
	f.Add("it", "it")
	f.Add("en", "invalid")
	f.Add("invalid", "en")
	f.Add("", "")

	f.Fuzz(func(t *testing.T, lang1, lang2 string) {
		// Should not panic
		_ = i18n.Init(lang1)
		result1 := i18n.T("test.message")

		_ = i18n.Init(lang2)
		result2 := i18n.T("test.message")

		// Both should return non-empty strings
		if result1 == "" {
			t.Errorf("T returned empty string for lang1=%q", lang1)
		}

		if result2 == "" {
			t.Errorf("T returned empty string for lang2=%q", lang2)
		}
	})
}
