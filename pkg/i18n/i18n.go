// SPDX-FileCopyrightText: 2021-2026 Gianluca Boiano
// SPDX-License-Identifier: GPL-3.0-only

// Package i18n provides internationalization support for YAP.
package i18n

import (
	"embed"
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"

	"github.com/nicksnyder/go-i18n/v2/i18n"
	"golang.org/x/text/language"
	"gopkg.in/yaml.v3"

	"github.com/M0Rf30/yap/v2/pkg/errors"
)

//go:embed locales/*
var localeFS embed.FS

var (
	// mu guards localizer, which Init writes and T reads.
	mu        sync.RWMutex
	localizer *i18n.Localizer
)

// SupportedLanguages lists all supported language codes.
var SupportedLanguages = []string{"en", "it"}

// Init initializes the i18n system with the given language preference.
// If lang is empty, it will try to detect the system language.
func Init(lang string) error {
	// Create a new bundle
	b := i18n.NewBundle(language.English)
	b.RegisterUnmarshalFunc("yaml", yaml.Unmarshal)

	// Load all supported languages from embedded files
	for _, langCode := range SupportedLanguages {
		filename := fmt.Sprintf("locales/%s.yaml", langCode)

		data, err := localeFS.ReadFile(filename)
		if err != nil {
			// Skip missing locale files during development
			continue
		}

		_, err = b.ParseMessageFileBytes(data, filename)
		if err != nil {
			return errors.Wrap(err, errors.ErrTypeConfiguration,
				fmt.Sprintf("failed to parse locale file %s", filename)).
				WithOperation("Init")
		}
	}

	// Determine the language to use
	if lang == "" {
		lang = detectSystemLanguage()
	}

	// Create localizer with fallback
	langs := []string{lang, "en"} // Always fallback to English
	loc := i18n.NewLocalizer(b, langs...)

	mu.Lock()
	localizer = loc
	mu.Unlock()

	return nil
}

// detectSystemLanguage attempts to detect the system language from environment
// variables using POSIX/GNU gettext precedence: the effective locale is the
// first non-empty of LC_ALL, LC_MESSAGES, LANG; unless it is "C"/"POSIX",
// the colon-separated LANGUAGE list takes priority over it.
func detectSystemLanguage() string {
	locale := ""

	for _, env := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if v := os.Getenv(env); v != "" {
			locale = v

			break
		}
	}

	if locale == "C" || locale == "POSIX" {
		return "en"
	}

	for entry := range strings.SplitSeq(os.Getenv("LANGUAGE"), ":") {
		if code := localeLanguageCode(entry); slices.Contains(SupportedLanguages, code) {
			return code
		}
	}

	if code := localeLanguageCode(locale); slices.Contains(SupportedLanguages, code) {
		return code
	}

	// Default to English
	return "en"
}

// localeLanguageCode extracts the lowercase language code from a locale name
// such as "it_IT.UTF-8@euro" -> "it". It returns "" for empty input.
func localeLanguageCode(locale string) string {
	if i := strings.IndexAny(locale, "_.@-"); i >= 0 {
		locale = locale[:i]
	}

	return strings.ToLower(locale)
}

// T translates a message using the provided ID and optional template data.
func T(messageID string, templateData ...map[string]any) string {
	mu.RLock()

	loc := localizer

	mu.RUnlock()

	if loc == nil {
		// Fallback if i18n is not initialized
		return messageID
	}

	config := &i18n.LocalizeConfig{
		MessageID: messageID,
	}

	if len(templateData) > 0 {
		config.TemplateData = templateData[0]
	}

	translated, err := loc.Localize(config)
	if err != nil {
		// Return the message ID if translation fails
		return messageID
	}

	return translated
}
