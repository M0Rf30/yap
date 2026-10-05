// SPDX-FileCopyrightText: 2021-2026 Gianluca Boiano
// SPDX-License-Identifier: GPL-3.0-only

package loader

// Export unexported functions for testing purposes.
var (
	ParseDependencyLineExported  = parseDependencyLine
	CleanDependencyNameExported  = cleanDependencyName
	ParseDependencyArrayExported = parseDependencyArray
	KahnLongestPathExported      = kahnLongestPath
	BuildInternalGraphExported   = buildInternalGraph
)
