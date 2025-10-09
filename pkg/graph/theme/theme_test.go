package theme

import (
	"testing"

	"github.com/M0Rf30/yap/v2/pkg/graph"
)

func TestGetTheme_Dark(t *testing.T) {
	theme := GetTheme("dark")

	expected := graph.Theme{
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

	if theme != expected {
		t.Errorf("Expected dark theme %+v, got %+v", expected, theme)
	}
}

func TestGetTheme_Classic(t *testing.T) {
	theme := GetTheme("classic")

	expected := graph.Theme{
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

	if theme != expected {
		t.Errorf("Expected classic theme %+v, got %+v", expected, theme)
	}
}

func TestGetTheme_Default(t *testing.T) {
	// Test default theme (should be "modern")
	theme := GetTheme("")

	expected := graph.Theme{
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

	if theme != expected {
		t.Errorf("Expected default theme %+v, got %+v", expected, theme)
	}
}

func TestGetTheme_Modern(t *testing.T) {
	// Test that "modern" theme is the same as default
	defaultTheme := GetTheme("")
	modernTheme := GetTheme("modern")

	if defaultTheme != modernTheme {
		t.Errorf("Expected modern theme to be same as default, got %+v vs %+v", defaultTheme, modernTheme)
	}
}

func TestGetTheme_CaseSensitivity(t *testing.T) {
	// Test that theme names are case-sensitive
	theme1 := GetTheme("dark")
	theme2 := GetTheme("DARK")

	if theme1 == theme2 {
		t.Errorf("Expected case-sensitive behavior, but 'dark' and 'DARK' returned same theme")
	}
}

func TestGetTheme_UnknownTheme(t *testing.T) {
	// Test that unknown theme names return default theme
	unknownTheme := GetTheme("unknown-theme")
	defaultTheme := GetTheme("")

	if unknownTheme != defaultTheme {
		t.Errorf("Expected unknown theme to return default, got %+v vs %+v", unknownTheme, defaultTheme)
	}
}

func TestGetTheme_ThemeValues(t *testing.T) {
	// Test all themes to ensure they have appropriate values
	themes := []string{"dark", "classic", "modern", "unknown"}

	for _, themeName := range themes {
		theme := GetTheme(themeName)

		// Check that all fields are populated
		if theme.Background == "" {
			t.Errorf("Theme %s has empty Background", themeName)
		}

		if theme.NodeInternal == "" {
			t.Errorf("Theme %s has empty NodeInternal", themeName)
		}

		if theme.NodeExternal == "" {
			t.Errorf("Theme %s has empty NodeExternal", themeName)
		}

		if theme.TextColor == "" {
			t.Errorf("Theme %s has empty TextColor", themeName)
		}
	}
}

func TestGetTheme_ColorFormat(t *testing.T) {
	// Test that color values are in expected formats
	theme := GetTheme("dark")

	// Check that colors look like hex values or other valid formats
	if len(theme.Background) < 4 && theme.Background[0] != '#' {
		t.Errorf("Background color '%s' doesn't look like a valid color", theme.Background)
	}

	if len(theme.NodeInternal) < 4 && theme.NodeInternal[0] != '#' {
		t.Errorf("NodeInternal color '%s' doesn't look like a valid color", theme.NodeInternal)
	}
}

func TestGetTheme_ThemeConsistency(t *testing.T) {
	// Test that each theme returns consistent values on multiple calls
	for i := range 5 {
		theme1 := GetTheme("dark")
		theme2 := GetTheme("dark")

		if theme1 != theme2 {
			t.Errorf("Theme is not consistent across calls, iteration %d", i)
		}
	}
}
