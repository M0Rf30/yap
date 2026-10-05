// SPDX-FileCopyrightText: 2021-2026 Gianluca Boiano
// SPDX-License-Identifier: GPL-3.0-only

package aptrepo_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/M0Rf30/yap/v2/pkg/aptrepo"
)

func TestParseRelease_ValidUntil(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		field string
		want  time.Time
	}{
		{"utc zone name", "Valid-Until: Thu, 01 Jan 2026 12:00:00 UTC", time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)},
		{"numeric offset", "Valid-Until: Thu, 01 Jan 2026 12:00:00 +0000", time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)},
		{"absent", "Suite: x", time.Time{}},
		{"unparsable", "Valid-Until: soon", time.Time{}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rel, err := aptrepo.ParseReleaseBodyForTesting([]byte(tc.field + "\nSHA256:\n"))
			require.NoError(t, err)
			assert.True(t, tc.want.Equal(rel.ValidUntil), "got %v want %v", rel.ValidUntil, tc.want)
		})
	}
}

func TestCheckValidUntil(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

	require.ErrorIs(t, aptrepo.CheckValidUntilForTesting(
		&aptrepo.Release{ValidUntil: now.Add(-time.Second)}, now), aptrepo.ErrReleaseExpired)
	require.NoError(t, aptrepo.CheckValidUntilForTesting(
		&aptrepo.Release{ValidUntil: now.Add(time.Hour)}, now))
	require.NoError(t, aptrepo.CheckValidUntilForTesting(&aptrepo.Release{}, now),
		"a Release without Valid-Until never expires")
}

func TestParseAndCheckRelease_RejectsExpired(t *testing.T) {
	t.Parallel()

	_, err := aptrepo.ParseAndCheckReleaseForTesting(
		[]byte("Valid-Until: Thu, 01 Jan 2015 00:00:00 UTC\nSHA256:\n"), "http://x/", "s")
	require.ErrorIs(t, err, aptrepo.ErrReleaseExpired)

	rel, err := aptrepo.ParseAndCheckReleaseForTesting(
		[]byte("Valid-Until: Thu, 01 Jan 2999 00:00:00 UTC\nSHA256:\n"), "http://x/", "s")
	require.NoError(t, err)
	assert.NotNil(t, rel)
}
