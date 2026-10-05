// SPDX-FileCopyrightText: 2021-2026 Gianluca Boiano
// SPDX-License-Identifier: GPL-3.0-only

package shell

import (
	"bytes"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScriptOutput_ConcurrentWrites(t *testing.T) {
	var dst bytes.Buffer

	out := newScriptOutput(NewPackageDecoratedWriter(&dst, "pkg"))

	var wg sync.WaitGroup

	for range 8 {
		wg.Go(func() {
			for range 200 {
				_, err := out.Write([]byte("some output line\n"))
				assert.NoError(t, err)
			}
		})
	}

	wg.Wait()

	assert.Equal(t, 8*200, strings.Count(out.String(), "some output line\n"))
	assert.Equal(t, 8*200, strings.Count(dst.String(), "some output line\n"))
}

func TestScriptOutput_BoundedTail(t *testing.T) {
	var dst bytes.Buffer

	out := newScriptOutput(&dst)
	line := strings.Repeat("x", 99) + "\n"

	total := (maxCapturedOutput / len(line)) * 5
	for i := range total {
		_, err := out.Write([]byte(line))
		require.NoError(t, err)

		if i == total-1 {
			_, err = out.Write([]byte("LAST ERROR\n"))
			require.NoError(t, err)
		}
	}

	got := out.String()

	assert.LessOrEqual(t, len(got), maxCapturedOutput)
	assert.True(t, strings.HasSuffix(got, "LAST ERROR\n"))
	// Leading partial line must be dropped.
	assert.True(t, strings.HasPrefix(got, "x"))
	assert.Equal(t, 0, strings.Index(got, strings.Repeat("x", 99)+"\n"))
	// Everything still reaches the downstream writer.
	assert.Greater(t, dst.Len(), maxCapturedOutput*4)
}

func TestScriptOutput_LargeSingleWrite(t *testing.T) {
	out := newScriptOutput(&bytes.Buffer{})
	big := strings.Repeat("a\n", maxCapturedOutput)

	_, err := out.Write([]byte(big))
	require.NoError(t, err)

	assert.LessOrEqual(t, len(out.String()), maxCapturedOutput)
}
