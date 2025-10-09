package rpm

import (
	"testing"
)

func TestRPMGroupConstants(t *testing.T) {
	expectedConstants := map[string]string{
		"Communications": "Applications/Communications",
		"Engineering":    "Applications/Engineering",
		"Internet":       "Applications/Internet",
		"Multimedia":     "Applications/Multimedia",
		"Tools":          "Development/Tools",
	}

	actualConstants := map[string]string{
		"Communications": Communications,
		"Engineering":    Engineering,
		"Internet":       Internet,
		"Multimedia":     Multimedia,
		"Tools":          Tools,
	}

	for name, expected := range expectedConstants {
		if actual := actualConstants[name]; actual != expected {
			t.Errorf("Constant %s = %q, want %q", name, actual, expected)
		}
	}
}

func TestRPMGroups(t *testing.T) {
	if RPMGroups == nil {
		t.Fatal("RPMGroups is nil")
	}

	expectedGroups := []string{
		"admin", "devel", "libs", "games", "graphics", "net", "text", "web",
	}

	for _, group := range expectedGroups {
		if _, exists := RPMGroups[group]; !exists {
			t.Errorf("RPMGroups missing expected group: %s", group)
		}
	}

	for group, rpmGroup := range RPMGroups {
		if rpmGroup == "" {
			t.Errorf("RPMGroups[%s] is empty", group)
		}
	}

	if RPMGroups["comm"] != Communications {
		t.Errorf("RPMGroups[comm] should map to Communications constant")
	}

	if RPMGroups["devel"] != Tools {
		t.Errorf("RPMGroups[devel] should map to Tools constant")
	}

	if RPMGroups["graphics"] != Multimedia {
		t.Errorf("RPMGroups[graphics] should map to Multimedia constant")
	}
}

func TestRPMDistros(t *testing.T) {
	if RPMDistros == nil {
		t.Fatal("RPMDistros is nil")
	}

	expectedDistros := []string{
		"almalinux", "amzn", "fedora", "ol", "rhel", "rocky",
	}

	for _, distro := range expectedDistros {
		if _, exists := RPMDistros[distro]; !exists {
			t.Errorf("RPMDistros missing expected distro: %s", distro)
		}
	}

	for distro, suffix := range RPMDistros {
		if suffix == "" {
			t.Errorf("RPMDistros[%s] is empty", distro)
		}

		if suffix[0] != '.' {
			t.Errorf("RPMDistros[%s] suffix %q should start with '.'", distro, suffix)
		}
	}

	expectedSuffixes := map[string]string{
		"almalinux": ".el",
		"amzn":      ".amzn",
		"fedora":    ".fc",
		"ol":        ".ol",
		"rhel":      ".el",
		"rocky":     ".el",
	}

	for distro, expectedSuffix := range expectedSuffixes {
		if actual := RPMDistros[distro]; actual != expectedSuffix {
			t.Errorf("RPMDistros[%s] = %q, want %q", distro, actual, expectedSuffix)
		}
	}
}

func TestRPMGroupMappingConsistency(t *testing.T) {
	multimediaGroups := []string{"graphics", "sound", "video"}
	for _, group := range multimediaGroups {
		if RPMGroups[group] != Multimedia {
			t.Errorf("RPMGroups[%s] should map to Multimedia, got %q", group, RPMGroups[group])
		}
	}

	internetGroups := []string{"httpd", "net", "web"}
	for _, group := range internetGroups {
		if RPMGroups[group] != Internet {
			t.Errorf("RPMGroups[%s] should map to Internet, got %q", group, RPMGroups[group])
		}
	}

	engineeringGroups := []string{"electronics", "embedded", "science"}
	for _, group := range engineeringGroups {
		if RPMGroups[group] != Engineering {
			t.Errorf("RPMGroups[%s] should map to Engineering, got %q", group, RPMGroups[group])
		}
	}
}

func TestRPMDistroSuffixConsistency(t *testing.T) {
	elDistros := []string{"almalinux", "rhel", "rocky"}
	for _, distro := range elDistros {
		if RPMDistros[distro] != ".el" {
			t.Errorf("RPMDistros[%s] should be .el, got %q", distro, RPMDistros[distro])
		}
	}
}
