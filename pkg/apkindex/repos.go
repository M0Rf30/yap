// SPDX-FileCopyrightText: 2021-2026 Gianluca Boiano
// SPDX-License-Identifier: GPL-3.0-only

package apkindex

import (
	"os"
	"runtime"
	"strings"

	pkgapk "github.com/M0Rf30/pkgformat/apk"

	"github.com/M0Rf30/yap/v2/pkg/constants"
	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
)

// LoadRepos parses /etc/apk/repositories and returns the list of repositories.
// Returns an error on non-Alpine systems or read errors.
func LoadRepos() ([]pkgapk.Repo, error) {
	f, err := os.Open("/etc/apk/repositories")
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	return pkgapk.ParseRepositories(f)
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
		logger.Warn(i18n.T("logger.apkindex.warn.could_not_detect_apk"), "goarch", runtime.GOARCH, "normalized", normalized)

		return ""
	}

	return apkArch
}
