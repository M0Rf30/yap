// Package i18n provides internationalization support for YAP.
package i18n

import (
	"embed"
	"fmt"
	"os"
	"strings"

	"github.com/nicksnyder/go-i18n/v2/i18n"
	"golang.org/x/text/language"
	"gopkg.in/yaml.v3"
)

//go:embed locales/*
var localeFS embed.FS

var (
	bundle    *i18n.Bundle
	localizer *i18n.Localizer
)

// SupportedLanguages lists all supported language codes.
var SupportedLanguages = []string{"en", "it"}

// Init initializes the i18n system with the given language preference.
// If lang is empty, it will try to detect the system language.
func Init(lang string) error {
	// Create a new bundle
	bundle = i18n.NewBundle(language.English)
	bundle.RegisterUnmarshalFunc("yaml", yaml.Unmarshal)

	// Load all supported languages from embedded files
	for _, langCode := range SupportedLanguages {
		filename := fmt.Sprintf("locales/%s.yaml", langCode)

		data, err := localeFS.ReadFile(filename)
		if err != nil {
			// Skip missing locale files during development
			continue
		}

		_, err = bundle.ParseMessageFileBytes(data, filename)
		if err != nil {
			return fmt.Errorf("failed to parse locale file %s: %w", filename, err)
		}
	}

	// Determine the language to use
	if lang == "" {
		lang = detectSystemLanguage()
	}

	// Create localizer with fallback
	langs := []string{lang, "en"} // Always fallback to English
	localizer = i18n.NewLocalizer(bundle, langs...)

	return nil
}

// detectSystemLanguage attempts to detect the system language from environment variables.
func detectSystemLanguage() string {
	// Check LANG environment variable first
	if lang := os.Getenv("LANG"); lang != "" {
		// Extract language code (e.g., "it_IT.UTF-8" -> "it")
		parts := strings.Split(lang, "_")
		if len(parts) > 0 {
			langCode := strings.ToLower(parts[0])
			// Check if we support this language
			for _, supported := range SupportedLanguages {
				if langCode == supported {
					return langCode
				}
			}
		}
	}

	// Check other environment variables
	for _, env := range []string{"LC_ALL", "LC_MESSAGES", "LANGUAGE"} {
		if lang := os.Getenv(env); lang != "" {
			parts := strings.Split(lang, "_")
			if len(parts) > 0 {
				langCode := strings.ToLower(parts[0])
				for _, supported := range SupportedLanguages {
					if langCode == supported {
						return langCode
					}
				}
			}
		}
	}

	// Default to English
	return "en"
}

// T translates a message using the provided ID and optional template data.
func T(messageID string, templateData ...map[string]any) string {
	if localizer == nil {
		// Fallback if i18n is not initialized
		return messageID
	}

	config := &i18n.LocalizeConfig{
		MessageID: messageID,
	}

	if len(templateData) > 0 {
		config.TemplateData = templateData[0]
	}

	translated, err := localizer.Localize(config)
	if err != nil {
		// Return the message ID if translation fails
		return messageID
	}

	return translated
}
