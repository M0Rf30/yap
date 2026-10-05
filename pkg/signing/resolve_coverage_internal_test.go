// SPDX-FileCopyrightText: 2021-2026 Gianluca Boiano
// SPDX-License-Identifier: GPL-3.0-only

package signing

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestAlgorithmForFormatUnknown verifies that an unknown format falls through
// to the default branch and returns AlgorithmGPG.
func TestAlgorithmForFormatUnknown(t *testing.T) {
	algo := algorithmForFormat("unknown_format")
	assert.Equal(t, AlgorithmGPG, algo)
}
