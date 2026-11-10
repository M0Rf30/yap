// SPDX-FileCopyrightText: 2021-2026 Gianluca Boiano
// SPDX-License-Identifier: GPL-3.0-only

package apthost_test

import (
	"runtime"
	"testing"

	"github.com/M0Rf30/pkgformat/deb/apt"
	"github.com/stretchr/testify/assert"

	"github.com/M0Rf30/yap/v2/pkg/apthost"
)

func TestAllowUnverified(t *testing.T) {
	t.Setenv("YAP_ALLOW_UNVERIFIED_REPOS", "")
	apthost.SetAllowUnverified(false)
	assert.False(t, apthost.AllowUnverified())

	for _, v := range []string{"1", "true", "YES", " on "} {
		t.Setenv("YAP_ALLOW_UNVERIFIED_REPOS", v)
		assert.True(t, apthost.AllowUnverified(), v)
	}

	t.Setenv("YAP_ALLOW_UNVERIFIED_REPOS", "0")
	assert.False(t, apthost.AllowUnverified())

	apthost.SetAllowUnverified(true)
	t.Cleanup(func() { apthost.SetAllowUnverified(false) })
	assert.True(t, apthost.AllowUnverified(), "the CLI flag wins over the environment")
}

func TestSetCacheReplacesTheProcessCache(t *testing.T) {
	prev := apthost.Cache()

	t.Cleanup(func() { apthost.SetCache(prev) })

	c := apt.NewCache("")
	c.AddEntry(&apt.PackageInfo{Name: "zlib1g", Architecture: apthost.HostArch(), HasCandidate: true})
	apthost.SetCache(c)

	assert.Same(t, c, apthost.Cache())

	info, ok := apthost.Cache().Lookup("zlib1g")
	assert.True(t, ok)
	assert.Equal(t, "zlib1g", info.Name)
}

func TestHostArchFollowsGOARCH(t *testing.T) {
	assert.Equal(t, apt.ArchFromGoarch(runtime.GOARCH), apthost.HostArch())
}

func TestDownloadOptionsHaveNoOverallTimeout(t *testing.T) {
	opts := apthost.DownloadOptions()

	if assert.NotNil(t, opts.Client) {
		assert.Zero(t, opts.Client.HTTP().Timeout)
	}
}
