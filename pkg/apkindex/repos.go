package apkindex

import (
	"os"
	"runtime"
	"strings"

	"github.com/M0Rf30/yap/v2/pkg/constants"
	"github.com/M0Rf30/yap/v2/pkg/logger"
)

// Repo represents an Alpine repository entry from /etc/apk/repositories.
type Repo struct {
	URL string // Base URL of the repository
	Tag string // Tag for tagged repos (e.g., "@edge"), empty for untagged
}

// LoadRepos parses /etc/apk/repositories and returns the list of repositories.
// Returns an empty slice on non-Alpine systems or read errors.
func LoadRepos() ([]Repo, error) {
	data, err := os.ReadFile("/etc/apk/repositories")
	if err != nil {
		return nil, err
	}

	var repos []Repo

	for _, line := range strings.SplitAfter(string(data), "\n") {
		line = strings.TrimSpace(line)

		// Skip empty lines and comments.
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		var tag string

		// Check for tagged repo: "@tag https://..."
		if strings.HasPrefix(line, "@") {
			parts := strings.SplitN(line, " ", 2)
			if len(parts) != 2 {
				continue
			}

			tag = strings.TrimPrefix(parts[0], "@")
			line = strings.TrimSpace(parts[1])
		}

		// Normalize URL: remove trailing slashes.
		url := strings.TrimRight(line, "/")
		if url != "" {
			repos = append(repos, Repo{URL: url, Tag: tag})
		}
	}

	return repos, nil
}

// DetectArch returns the APK architecture for this host.
// Prefers /etc/apk/arch; falls back to constants.GetArchMapping().
func DetectArch() string {
	// Try to read /etc/apk/arch first.
	if data, err := os.ReadFile("/etc/apk/arch"); err == nil {
		arch := strings.TrimSpace(string(data))
		if arch != "" {
			return arch
		}
	}

	// Fall back to mapping runtime.GOARCH to APK arch.
	normalized := constants.NormalizeArchitecture(runtime.GOARCH)
	apkArch := constants.GetArchMapping().TranslateArch(constants.FormatAPK, normalized)

	if apkArch == "" {
		logger.Warn("could not detect APK architecture",
			"goarch", runtime.GOARCH, "normalized", normalized)

		return ""
	}

	return apkArch
}
