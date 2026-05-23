package pacmandb

import (
	"os"
	"strings"

	"github.com/M0Rf30/yap/v2/pkg/errors"
)

// Config represents the parsed pacman.conf configuration.
type Config struct {
	Architecture string
	Repos        []Repo
}

// Repo represents a pacman repository configuration.
type Repo struct {
	Name    string
	Servers []string // Resolved Server = entries (including from Include)
}

// ParseConfig parses /etc/pacman.conf and returns the configuration.
func ParseConfig(path string) (*Config, error) {
	return parseConfigWithIncludes(path, make(map[string]bool))
}

// parseConfigWithIncludes parses a pacman.conf file, handling Include directives recursively.
// Detects circular includes and returns an error if found.
func parseConfigWithIncludes(
	path string,
	seenIncludes map[string]bool,
) (*Config, error) {
	if seenIncludes[path] {
		return nil, errors.New(errors.ErrTypeConfiguration, "circular Include directive detected").
			WithOperation("parseConfigWithIncludes").
			WithContext("path", path)
	}

	seenIncludes[path] = true

	data, err := os.ReadFile(path) //nolint:gosec
	if err != nil {
		return nil, err
	}

	cfg := &Config{}

	var curRepo *Repo

	for _, raw := range strings.SplitAfter(string(data), "\n") {
		raw = strings.TrimSuffix(raw, "\n")

		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Section header [name].
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			curRepo = handleSectionHeader(cfg, line)
			continue
		}

		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}

		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)

		if err := handleConfigKeyValue(cfg, curRepo, key, val, seenIncludes); err != nil {
			return nil, err
		}
	}

	return cfg, nil
}

// handleSectionHeader processes a [section] header line and updates the current repo.
// Returns the new curRepo pointer (nil for [options], pointer to new Repo otherwise).
func handleSectionHeader(cfg *Config, line string) *Repo {
	name := strings.TrimSuffix(strings.TrimPrefix(line, "["), "]")
	if name == "options" {
		return nil
	}

	cfg.Repos = append(cfg.Repos, Repo{Name: name})

	return &cfg.Repos[len(cfg.Repos)-1]
}

// handleConfigKeyValue processes a key=value line in the config.
// Handles Architecture, Server, and Include directives.
func handleConfigKeyValue(cfg *Config, curRepo *Repo, key, val string, seenIncludes map[string]bool) error {
	switch {
	case curRepo == nil && key == "Architecture":
		cfg.Architecture = val
	case curRepo != nil && key == "Server":
		curRepo.Servers = append(curRepo.Servers, val)
	case curRepo != nil && key == "Include":
		servers, err := parseMirrorlist(val)
		if err != nil {
			// Try as a full pacman.conf include (rare but possible).
			sub, err2 := parseConfigWithIncludes(val, seenIncludes)
			if err2 != nil {
				// Propagate circular include or other errors
				return err2
			}
			// Merge sub.Repos? Not typical. Just skip if not a mirrorlist.
			_ = sub
		} else {
			curRepo.Servers = append(curRepo.Servers, servers...)
		}
	}

	return nil
}

func parseMirrorlist(path string) ([]string, error) {
	data, err := os.ReadFile(path) //nolint:gosec
	if err != nil {
		return nil, err
	}

	var servers []string

	for _, raw := range strings.SplitAfter(string(data), "\n") {
		raw = strings.TrimSuffix(raw, "\n")

		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if k, v, ok := strings.Cut(line, "="); ok &&
			strings.TrimSpace(k) == "Server" {
			servers = append(servers, strings.TrimSpace(v))
		}
	}

	return servers, nil
}
