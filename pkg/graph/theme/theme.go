// Package theme provides visual styling configurations for dependency graphs.
package theme

import "github.com/M0Rf30/yap/v2/pkg/graph"

// GetTheme returns a theme configuration by name.
func GetTheme(themeName string) graph.Theme {
	switch themeName {
	case "dark":
		return graph.Theme{
			Background:   "#1a1a1a",
			NodeInternal: "#4CAF50",
			NodeExternal: "#FF9800",
			NodePopular:  "#2196F3",
			EdgeRuntime:  "#ffffff",
			EdgeMake:     "#ffbb33",
			EdgeCheck:    "#ff6b6b",
			EdgeOptional: "#888888",
			TextColor:    "#ffffff",
			BorderColor:  "#333333",
			GridColor:    "#333333",
		}
	case "classic":
		return graph.Theme{
			Background:   "#ffffff",
			NodeInternal: "#2E7D32",
			NodeExternal: "#F57C00",
			NodePopular:  "#1976D2",
			EdgeRuntime:  "#424242",
			EdgeMake:     "#FF9800",
			EdgeCheck:    "#E53935",
			EdgeOptional: "#757575",
			TextColor:    "#212121",
			BorderColor:  "#BDBDBD",
			GridColor:    "#EEEEEE",
		}
	default: // modern
		return graph.Theme{
			Background:   "linear-gradient(135deg, #667eea 0%, #764ba2 100%)",
			NodeInternal: "#00C851",
			NodeExternal: "#ffbb33",
			NodePopular:  "#0099CC",
			EdgeRuntime:  "#ffffff",
			EdgeMake:     "#ffbb33",
			EdgeCheck:    "#ff4444",
			EdgeOptional: "#E0E0E0",
			TextColor:    "#ffffff",
			BorderColor:  "#ffffff",
			GridColor:    "rgba(255,255,255,0.1)",
		}
	}
}
