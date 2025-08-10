// Package dependencies provides common dependency processing functionality.
package dependencies

import (
	"regexp"
	"strings"

	"github.com/google/rpmpack"
)

// Processor handles dependency processing for different package formats.
type Processor struct {
	pattern *regexp.Regexp
}

// NewProcessor creates a new dependency processor.
func NewProcessor() *Processor {
	pattern := regexp.MustCompile(`(?m)(<|<=|>=|=|>|<)`)
	return &Processor{pattern: pattern}
}

// FormatForDeb processes dependencies for Debian package format.
// Converts "package>=1.0" to "package (>= 1.0)".
func (p *Processor) FormatForDeb(depends []string) []string {
	processed := make([]string, len(depends))
	for index, depend := range depends {
		processed[index] = p.formatSingleDependency(depend, "deb")
	}
	return processed
}

// FormatForRPM processes dependencies for RPM package format.
// Converts "package>=1.0" to "package >= 1.0".
func (p *Processor) FormatForRPM(depends []string) []string {
	processed := make([]string, len(depends))
	for index, depend := range depends {
		processed[index] = p.formatSingleDependency(depend, "rpm")
	}
	return processed
}

// ProcessForRPMRelations converts a slice of dependency strings to rpmpack.Relations.
func (p *Processor) ProcessForRPMRelations(depends []string) rpmpack.Relations {
	relations := make(rpmpack.Relations, 0)

	for _, depend := range depends {
		formatted := p.formatSingleDependency(depend, "rpm")
		err := relations.Set(formatted)
		if err != nil {
			// If there's an error setting the relation, return nil
			// This matches the original behavior in rpm.go
			return nil
		}
	}

	return relations
}

// formatSingleDependency processes a single dependency string according to the specified format.
func (p *Processor) formatSingleDependency(depend, format string) string {
	result := p.pattern.Split(depend, -1)
	if len(result) != 2 {
		// No version operator found, return as-is
		return depend
	}

	name := result[0]
	operator := strings.Trim(depend, result[0]+result[1])
	version := result[1]

	switch format {
	case "deb":
		return name + " (" + operator + " " + version + ")"
	case "rpm":
		return name + " " + operator + " " + version
	default:
		return depend
	}
}

// NormalizeBackupFiles ensures all backup file paths have a leading slash.
// This is used by multiple package managers for config file handling.
func NormalizeBackupFiles(backupFiles []string) []string {
	normalized := make([]string, len(backupFiles))
	for i, filePath := range backupFiles {
		if !strings.HasPrefix(filePath, "/") {
			normalized[i] = "/" + filePath
		} else {
			normalized[i] = filePath
		}
	}
	return normalized
}
