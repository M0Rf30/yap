// SPDX-FileCopyrightText: 2021-2026 Gianluca Boiano
// SPDX-License-Identifier: GPL-3.0-only

package command

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInitializeGensumDescriptions(t *testing.T) {
	InitializeGensumDescriptions()
	assert.NotEmpty(t, gensumCmd.Short, "gensumCmd.Short should be non-empty after InitializeGensumDescriptions")
}

func TestGensumCommandDefinition(t *testing.T) {
	assert.Equal(t, "gensum <path>", gensumCmd.Use)
	assert.NotNil(t, gensumCmd.RunE)
}
