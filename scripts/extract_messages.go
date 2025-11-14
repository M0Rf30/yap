// Package main provides a tool for extracting i18n.T() messages from Go source code.
package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Message represents a single translation message.
type Message struct {
	ID          string `yaml:"id"`
	Translation string `yaml:"translation"`
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run extract.go <source_directory>")
		os.Exit(1)
	}

	sourceDir := os.Args[1]

	// Extract messages from source code
	messages, err := extractMessages(sourceDir)
	if err != nil {
		fmt.Printf("Error extracting messages: %v\n", err)
		os.Exit(1)
	}

	// Read existing English translations
	existingMessages, err := readExistingMessages()
	if err != nil {
		fmt.Printf("Error reading existing messages: %v\n", err)
		os.Exit(1)
	}

	// Merge extracted messages with existing translations
	mergedMessages := mergeMessages(messages, existingMessages)

	// Write to English locale file
	if err := writeMessagesToFile(mergedMessages, "pkg/i18n/locales/en.yaml"); err != nil {
		fmt.Printf("Error writing messages: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Extracted %d messages and updated en.yaml\n", len(mergedMessages))
}

// extractMessages scans Go source files and extracts i18n.T() calls.
func extractMessages(dir string) ([]Message, error) {
	var messages []Message

	messageSet := make(map[string]bool) // To avoid duplicates

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Only process .go files
		if !strings.HasSuffix(path, ".go") {
			return nil
		}

		// Skip test files
		if strings.HasSuffix(path, "_test.go") {
			return nil
		}

		// Parse the Go file
		fset := token.NewFileSet()

		node, err := parser.ParseFile(fset, path, nil, parser.AllErrors)
		if err != nil {
			return err
		}

		// Walk the AST to find i18n.T() calls
		ast.Inspect(node, func(n ast.Node) bool {
			callExpr, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}

			// Check if it's a call to i18n.T
			if isI18nTCall(callExpr) {
				extractMessageFromCall(callExpr, &messages, messageSet)
			}

			return true
		})

		return nil
	})
	if err != nil {
		return nil, err
	}

	// Sort messages by ID for consistency
	sort.Slice(messages, func(i, j int) bool {
		return messages[i].ID < messages[j].ID
	})

	return messages, nil
}

// isI18nTCall checks if a call expression is i18n.T().
func isI18nTCall(callExpr *ast.CallExpr) bool {
	// Check for selector expression (i18n.T)
	selectorExpr, ok := callExpr.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}

	// Check if the selector is "T"
	if selectorExpr.Sel.Name != "T" {
		return false
	}

	// Check if the receiver is "i18n"
	ident, ok := selectorExpr.X.(*ast.Ident)
	if !ok {
		return false
	}

	return ident.Name == "i18n"
}

// readExistingMessages reads the existing English locale file.
func readExistingMessages() ([]Message, error) {
	data, err := os.ReadFile("en.yaml")
	if err != nil {
		// If the file doesn't exist, return empty slice
		if os.IsNotExist(err) {
			return []Message{}, nil
		}

		return nil, err
	}

	var messages []Message
	if err := yaml.Unmarshal(data, &messages); err != nil {
		return nil, fmt.Errorf("failed to parse YAML in en.yaml: %w", err)
	}

	return messages, nil
}

// mergeMessages combines extracted messages with existing translations.
func mergeMessages(extracted, existing []Message) []Message {
	// Create a map of existing messages for quick lookup
	existingMap := make(map[string]string)
	for _, msg := range existing {
		existingMap[msg.ID] = msg.Translation
	}

	// Merge extracted messages with existing translations
	var merged []Message

	for _, msg := range extracted {
		if translation, exists := existingMap[msg.ID]; exists {
			// Use existing translation
			merged = append(merged, Message{
				ID:          msg.ID,
				Translation: translation,
			})
		} else {
			// Use extracted message with ID as translation
			merged = append(merged, msg)
		}
	}

	return merged
}

// extractMessageFromCall extracts a message from an i18n.T call expression.
func extractMessageFromCall(
	callExpr *ast.CallExpr, messages *[]Message, messageSet map[string]bool,
) {
	if len(callExpr.Args) == 0 {
		return
	}

	// Extract the message ID (first argument)
	basicLit, ok := callExpr.Args[0].(*ast.BasicLit)
	if !ok || basicLit.Kind != token.STRING {
		return
	}

	// Remove quotes from the string literal
	messageID := strings.Trim(basicLit.Value, "\"`")
	if messageSet[messageID] {
		return
	}

	*messages = append(*messages, Message{
		ID:          messageID,
		Translation: messageID, // Default to ID as translation for source language
	})
	messageSet[messageID] = true
}

// writeMessagesToFile writes messages to a YAML file.
func writeMessagesToFile(messages []Message, filename string) error {
	// Add header comment
	header := "# YAP English Localization (Auto-generated)\n"

	// Marshal to YAML
	data, err := yaml.Marshal(messages)
	if err != nil {
		return err
	}

	// Write to file
	content := header + string(data)

	return os.WriteFile(filename, []byte(content), 0o600)
}
