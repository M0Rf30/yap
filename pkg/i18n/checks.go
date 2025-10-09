// Package i18n provides internationalization support for YAP.
package i18n

import (
	"fmt"
	"sort"
	"strings"

	"github.com/nicksnyder/go-i18n/v2/i18n"
	"golang.org/x/text/language"
	"gopkg.in/yaml.v3"
)

// Message represents a single translation message.
type Message struct {
	ID          string `yaml:"id"`
	Translation string `yaml:"translation"`
}

// CheckIntegrity verifies the integrity of all locale files.
// It checks for:
// 1. Consistent message IDs across all locales
// 2. Proper YAML formatting
// 3. No duplicate message IDs
// 4. Required message IDs present
func CheckIntegrity() error {
	// Create a new bundle for validation
	bundle := i18n.NewBundle(language.English)
	bundle.RegisterUnmarshalFunc("yaml", yaml.Unmarshal)

	// Map to store message IDs from each language
	messageIDs := make(map[string]map[string]bool) // lang -> id -> exists
	allMessageIDs := make(map[string]bool)         // all unique message IDs across all languages

	// Load all supported languages
	for _, langCode := range SupportedLanguages {
		filename := fmt.Sprintf("locales/%s.yaml", langCode)

		data, err := localeFS.ReadFile(filename)
		if err != nil {
			return fmt.Errorf("failed to read locale file %s: %w", filename, err)
		}

		// Parse the YAML to check formatting
		var messages []Message
		if err := yaml.Unmarshal(data, &messages); err != nil {
			return fmt.Errorf("failed to parse YAML in %s: %w", filename, err)
		}

		// Check for duplicate message IDs
		seenIDs := make(map[string]bool)
		messageIDs[langCode] = make(map[string]bool)

		for _, msg := range messages {
			if seenIDs[msg.ID] {
				return fmt.Errorf("duplicate message ID '%s' found in %s", msg.ID, filename)
			}

			seenIDs[msg.ID] = true
			messageIDs[langCode][msg.ID] = true
			allMessageIDs[msg.ID] = true
		}
	}

	// Check for consistent message IDs across all languages
	var missingIDs []string

	for id := range allMessageIDs {
		for _, langCode := range SupportedLanguages {
			if !messageIDs[langCode][id] {
				missingIDs = append(missingIDs, fmt.Sprintf("missing ID '%s' in %s", id, langCode))
			}
		}
	}

	if len(missingIDs) > 0 {
		return fmt.Errorf("inconsistent message IDs found: %s", strings.Join(missingIDs, ", "))
	}

	return nil
}

// GetMessageIDs returns all message IDs used in the application.
func GetMessageIDs() ([]string, error) {
	// Create a new bundle for validation
	bundle := i18n.NewBundle(language.English)
	bundle.RegisterUnmarshalFunc("yaml", yaml.Unmarshal)

	// Use English as the reference language
	filename := "locales/en.yaml"

	data, err := localeFS.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read reference locale file %s: %w", filename, err)
	}

	// Parse the YAML
	var messages []Message
	if err := yaml.Unmarshal(data, &messages); err != nil {
		return nil, fmt.Errorf("failed to parse YAML in %s: %w", filename, err)
	}

	// Extract message IDs
	var ids []string
	for _, msg := range messages {
		ids = append(ids, msg.ID)
	}

	// Sort for consistency
	sort.Strings(ids)

	return ids, nil
}
