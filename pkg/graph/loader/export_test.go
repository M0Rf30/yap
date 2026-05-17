package loader

// Export unexported functions for testing purposes.
var (
	ParseDependencyLineExported  = parseDependencyLine
	CleanDependencyNameExported  = cleanDependencyName
	ParseDependencyArrayExported = parseDependencyArray
	KahnLongestPathExported      = kahnLongestPath
	BuildInternalGraphExported   = buildInternalGraph
)
