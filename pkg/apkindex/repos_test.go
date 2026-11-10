// SPDX-FileCopyrightText: 2021-2026 Gianluca Boiano
// SPDX-License-Identifier: GPL-3.0-only

package apkindex_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/M0Rf30/yap/v2/pkg/apkindex"
)

func TestLoadReposOnNonAlpineSystem(t *testing.T) {
	if _, err := os.Stat("/etc/apk/repositories"); err == nil {
		t.Skip("skipping: /etc/apk/repositories exists on this host")
	}

	_, err := apkindex.LoadRepos()
	assert.Error(t, err, "expected error when /etc/apk/repositories is absent")
}

func TestLoadReposOnAlpineSystem(t *testing.T) {
	if _, err := os.Stat("/etc/apk/repositories"); err != nil {
		t.Skip("skipping: /etc/apk/repositories not present (non-Alpine host)")
	}

	repos, err := apkindex.LoadRepos()
	require.NoError(t, err)

	for i, r := range repos {
		assert.NotEmpty(t, r.URL, "repo[%d] has empty URL", i)
	}
}

func TestDetectArchFromFile(t *testing.T) {
	data, err := os.ReadFile("/etc/apk/arch")
	if err != nil {
		t.Skip("skipping: /etc/apk/arch not present (non-Alpine host)")
	}

	assert.Equal(t, string(data[:len(data)-1]), apkindex.DetectArch())
}

func TestDetectArchFallsBackToGOARCH(t *testing.T) {
	if _, err := os.Stat("/etc/apk/arch"); err == nil {
		t.Skip("skipping: /etc/apk/arch present")
	}

	assert.NotEmpty(t, apkindex.DetectArch())
}
