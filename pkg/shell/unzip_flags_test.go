// SPDX-FileCopyrightText: 2021-2026 Gianluca Boiano
// SPDX-License-Identifier: GPL-3.0-only

package shell

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"
)

func TestParseUnzipArgs(t *testing.T) {
	tests := []struct {
		name string
		args []string
		ok   bool
		want unzipArgs
	}{
		{"plain", []string{"unzip", "a.zip"}, true,
			unzipArgs{destDir: "/w", archivePath: "a.zip"}},
		{"overwrite quiet dest", []string{"unzip", "-oq", "a.zip", "-d", "out"}, true,
			unzipArgs{destDir: "out", archivePath: "a.zip"}},
		{"attached dest", []string{"unzip", "-dout", "a.zip"}, true,
			unzipArgs{destDir: "out", archivePath: "a.zip"}},
		{"filters", []string{"unzip", "a.zip", "conf/*"}, true,
			unzipArgs{destDir: "/w", archivePath: "a.zip", filters: []string{"conf/*"}}},
		{"list", []string{"unzip", "-l", "a.zip"}, false, unzipArgs{}},
		{"test", []string{"unzip", "-t", "a.zip"}, false, unzipArgs{}},
		{"pipe", []string{"unzip", "-p", "a.zip", "f"}, false, unzipArgs{}},
		{"verbose", []string{"unzip", "-v", "a.zip"}, false, unzipArgs{}},
		{"exclude", []string{"unzip", "a.zip", "-x", "*.txt"}, false, unzipArgs{}},
		{"junk", []string{"unzip", "-j", "a.zip"}, false, unzipArgs{}},
		{"dangling -d", []string{"unzip", "a.zip", "-d"}, false, unzipArgs{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseUnzipArgs(tt.args, "/w")
			assert.Equal(t, tt.ok, ok)

			if tt.ok {
				assert.Equal(t, tt.want.destDir, got.destDir)
				assert.Equal(t, tt.want.archivePath, got.archivePath)
				assert.Equal(t, tt.want.filters, got.filters)
			}
		})
	}
}

var errFellThrough = errors.New("fell through to next handler")

func TestHandleUnzip_UnsupportedFlagsFallThrough(t *testing.T) {
	dir := t.TempDir()
	createTestZip(t, filepath.Join(dir, "a.zip"), "hello.txt", "hi")

	for _, flags := range [][]string{
		{"unzip", "-l", "a.zip"},
		{"unzip", "-p", "a.zip", "hello.txt"},
		{"unzip", "a.zip", "-x", "*.txt"},
	} {
		called := false
		next := func(_ context.Context, _ []string) error {
			called = true

			return errFellThrough
		}

		runner, err := interp.New(
			interp.Dir(dir),
			interp.ExecHandlers(func(h interp.ExecHandlerFunc) interp.ExecHandlerFunc {
				return func(ctx context.Context, args []string) error {
					return handleUnzip(ctx, args, next)
				}
			}),
		)
		require.NoError(t, err)

		parsed, err := syntax.NewParser(syntax.Variant(syntax.LangBash)).
			Parse(strings.NewReader(strings.Join(flags, " ")), "test")
		require.NoError(t, err)

		require.ErrorIs(t, runner.Run(context.Background(), parsed), errFellThrough)
		assert.True(t, called, "%v should defer to the real binary", flags)

		_, statErr := os.Stat(filepath.Join(dir, "hello.txt"))
		assert.True(t, os.IsNotExist(statErr), "%v must not extract", flags)
	}
}
