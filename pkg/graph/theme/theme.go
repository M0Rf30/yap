// Package theme provides visual styling configurations for dependency graphs.
package theme

import "github.com/M0Rf30/yap/v2/pkg/graph"

const (
	colorOrange   = "#FF9800"
	colorWhite    = "#ffffff"
	colorYellow   = "#ffbb33"
	colorDarkGray = "#333333"
)

// GetTheme returns a theme configuration by name.
func GetTheme(themeName string) graph.Theme {
	switch themeName {
	case "dark":
		return graph.Theme{
			Background:   "#1a1a1a",
			NodeInternal: "#4CAF50",
			NodeExternal: colorOrange,
			NodePopular:  "#2196F3",
			EdgeRuntime:  colorWhite,
			EdgeMake:     colorYellow,
			EdgeCheck:    "#ff6b6b",
			EdgeOptional: "#888888",
			TextColor:    colorWhite,
			BorderColor:  colorDarkGray,
			GridColor:    colorDarkGray,
		}
	case "classic":
		return graph.Theme{
			Background:   colorWhite,
			NodeInternal: "#2E7D32",
			NodeExternal: "#F57C00",
			NodePopular:  "#1976D2",
			EdgeRuntime:  "#424242",
			EdgeMake:     colorOrange,
			EdgeCheck:    "#E53935",
			EdgeOptional: "#757575",
			TextColor:    "#212121",
			BorderColor:  "#BDBDBD",
			GridColor:    "#EEEEEE",
		}
	case "modern": // Backward compatibility alias
		return GetTheme("gradient")
	default: // gradient (default theme)
		return graph.Theme{
			Background:   "linear-gradient(135deg, #667eea 0%, #764ba2 100%)",
			NodeInternal: "#00C851",
			NodeExternal: colorYellow,
			NodePopular:  "#0099CC",
			EdgeRuntime:  colorWhite,
			EdgeMake:     colorYellow,
			EdgeCheck:    "#ff4444",
			EdgeOptional: "#E0E0E0",
			TextColor:    colorWhite,
			BorderColor:  colorWhite,
			GridColor:    "rgba(255,255,255,0.1)",
		}
	}
}
