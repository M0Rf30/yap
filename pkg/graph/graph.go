// Package graph provides core graph data structures and types for dependency visualization.
package graph

// Node represents a package in the dependency graph.
type Node struct {
	Name          string
	PkgName       string // Actual package name from PKGBUILD
	Version       string
	Release       string
	X, Y          float64
	Width, Height float64 // Dynamic node dimensions
	IsExternal    bool
	IsPopular     bool
	Dependencies  []string
	Level         int
}

// Edge represents a dependency relationship between packages.
type Edge struct {
	From string
	To   string
	Type string // "runtime", "make", "check", "opt"
}

// Data represents the complete graph structure.
type Data struct {
	Nodes map[string]*Node
	Edges []Edge
	Theme Theme
}

// Bounds represents the calculated dimensions for the graph.
type Bounds struct {
	MinX, MinY, MaxX, MaxY float64
	Width, Height          float64
	Padding                float64
}

// Theme represents the visual styling configuration.
type Theme struct {
	Background   string
	NodeInternal string
	NodeExternal string
	NodePopular  string
	EdgeRuntime  string
	EdgeMake     string
	EdgeCheck    string
	EdgeOptional string
	TextColor    string
	BorderColor  string
	GridColor    string
}

// Options represents configuration options for graph generation.
type Options struct {
	Output       string
	Format       string
	Theme        string
	ShowExternal bool
}
