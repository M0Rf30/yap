// SPDX-FileCopyrightText: 2021-2026 Gianluca Boiano
// SPDX-License-Identifier: GPL-3.0-only

package command

import "github.com/spf13/cobra"

// BuildCommand returns the `yap build` cobra command. It lets other packages
// (notably pkg/mcp tests) validate argv against the real flag definitions.
func BuildCommand() *cobra.Command { return buildCmd }

// PrepareCommand returns the `yap prepare` cobra command.
func PrepareCommand() *cobra.Command { return prepareCmd }

// RootCommand returns the root cobra command (persistent flags live here).
func RootCommand() *cobra.Command { return rootCmd }
