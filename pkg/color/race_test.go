// SPDX-FileCopyrightText: 2021-2026 Gianluca Boiano
// SPDX-License-Identifier: GPL-3.0-only

package color_test

import (
	"sync"
	"testing"

	"github.com/M0Rf30/yap/v2/pkg/color"
)

// TestDisableEnableConcurrent exercises Disable/Enable against concurrent
// color calls; it is meaningful under -race.
func TestDisableEnableConcurrent(t *testing.T) {
	wasDisabled := color.IsDisabled()

	t.Cleanup(func() {
		if wasDisabled {
			color.Disable()
		} else {
			color.Enable()
		}
	})

	var wg sync.WaitGroup

	for range 8 {
		wg.Add(2)

		go func() {
			defer wg.Done()

			for range 200 {
				color.Disable()
				color.Enable()
			}
		}()

		go func() {
			defer wg.Done()

			for range 200 {
				_ = color.Red("x")
				_ = color.IsDisabled()
			}
		}()
	}

	wg.Wait()
}
