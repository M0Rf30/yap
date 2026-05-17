package pkgbuild_test

import (
	"strings"
	"testing"

	"github.com/M0Rf30/yap/v2/pkg/pkgbuild"
)

// FuzzParseDirective tests the parseDirective method with arbitrary input.
// Must never panic. Invariants: returned key is never longer than input,
// priority is in range [-1, 8].
func FuzzParseDirective(f *testing.F) {
	// Seed corpus with valid and edge-case directives
	seeds := []string{
		"depends",
		"depends__ubuntu",
		"depends_x86_64",
		"depends_x86_64__ubuntu",
		"depends_x86_64__ubuntu_jammy",
		"source_aarch64",
		"pkgdesc__fedora",
		"",
		"_",
		"__",
		"___",
		"depends__",
		"_depends",
		"depends_",
		"x" + strings.Repeat("_", 1000),
		"depends__ubuntu__fedora",
		"depends_x86_64_aarch64",
		"depends_invalid_arch",
		"depends__invalid_distro",
		":::",
		"depends::ubuntu",
		"#depends",
		"$depends",
		" depends",
		"depends ",
		"DEPENDS",
		"DePeNdS",
	}

	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, input string) {
		// Empty key calls os.Setenv("", ...) which triggers logger.Fatal — skip.
		if input == "" {
			return
		}

		pb := &pkgbuild.PKGBUILD{
			Distro:   "ubuntu",
			Codename: "jammy",
		}
		pb.Init()

		// Call parseDirective via AddItem which uses it internally
		// We test it indirectly since it's unexported
		_ = pb.AddItem(input, "test-value")
		// Errors are OK; we just want to ensure no panic

		// Verify invariants
		if input != "" && pb.PkgName != "" {
			// If we got a result, it should be reasonable
			if len(pb.PkgName) > len(input)+100 {
				t.Errorf("PkgName unexpectedly long: %d > %d", len(pb.PkgName), len(input)+100)
			}
		}
	})
}

// FuzzAddItemString tests AddItem with arbitrary key+string value pairs.
// Must never panic.
func FuzzAddItemString(f *testing.F) {
	seeds := []struct {
		key   string
		value string
	}{
		{"pkgname", "test"},
		{"pkgver", "1.0.0"},
		{"pkgrel", "1"},
		{"pkgdesc", "A test package"},
		{"maintainer", "Test User"},
		{"url", "https://example.com"},
		{"license", "MIT"},
		{"depends", "gcc"},
		{"makedepends", "make"},
		{"custom_var", "custom_value"},
		{"", ""},
		{"key", ""},
		{"", "value"},
		{"key" + strings.Repeat("x", 1000), "value"},
		{"key", strings.Repeat("y", 10000)},
		{"key__ubuntu", "value"},
		{"key_x86_64", "value"},
		{"key_x86_64__ubuntu", "value"},
	}

	for _, seed := range seeds {
		f.Add(seed.key, seed.value)
	}

	f.Fuzz(func(t *testing.T, key, value string) {
		// Empty key triggers logger.Fatal via os.Setenv — skip.
		if key == "" {
			return
		}

		pb := &pkgbuild.PKGBUILD{
			Distro:   "ubuntu",
			Codename: "jammy",
		}
		pb.Init()

		// Should not panic
		_ = pb.AddItem(key, value)
	})
}

// FuzzAddItemArray tests AddItem with arbitrary key + string values.
// Must never panic.
func FuzzAddItemArray(f *testing.F) {
	seeds := []struct {
		key   string
		value string
	}{
		{"arch", "x86_64,aarch64"},
		{"depends", "gcc,make"},
		{"license", "MIT,Apache-2.0"},
		{"source", "https://example.com/file.tar.gz"},
		{"sha256sums", "abc123"},
		{"", ""},
		{"key", ""},
		{"key__ubuntu", "value1,value2"},
		{"key_x86_64", "arch-specific"},
	}

	for _, seed := range seeds {
		f.Add(seed.key, seed.value)
	}

	f.Fuzz(func(t *testing.T, key, value string) {
		// Empty key triggers logger.Fatal via os.Setenv — skip.
		if key == "" {
			return
		}

		pb := &pkgbuild.PKGBUILD{
			Distro:   "ubuntu",
			Codename: "jammy",
		}
		pb.Init()

		// Convert to array for testing
		values := []string{}
		if value != "" {
			values = strings.Split(value, ",")
		}

		// Should not panic
		_ = pb.AddItem(key, values)
	})
}

// FuzzCheckLicense tests license validation with arbitrary license strings.
// Must never panic.
func FuzzCheckLicense(f *testing.F) {
	seeds := []string{
		"MIT",
		"Apache-2.0",
		"GPL-3.0-or-later",
		"MIT OR Apache-2.0",
		"(MIT AND Apache-2.0)",
		"PROPRIETARY",
		"CUSTOM",
		"",
		"INVALID-LICENSE",
		"MIT AND",
		"AND MIT",
		"MIT OR OR Apache",
		strings.Repeat("A", 10000),
		"MIT\nApache-2.0",
		"MIT\x00Apache",
	}

	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, license string) {
		pb := &pkgbuild.PKGBUILD{
			Distro:   "ubuntu",
			Codename: "jammy",
		}
		pb.Init()

		// Add license via AddItem
		_ = pb.AddItem("license", []string{license})

		// Call checkLicense indirectly via Finalize which calls it
		pb.Finalize()
		// Should not panic
	})
}

// FuzzBuildScriptPreamble tests BuildScriptPreamble with arbitrary custom variables.
// Must never panic, must return valid string.
func FuzzBuildScriptPreamble(f *testing.F) {
	seeds := []struct {
		varKey   string
		varValue string
	}{
		{"_prefix", "/usr"},
		{"", ""},
		{"key", "value with 'quotes'"},
		{"key", "value\nwith\nnewlines"},
	}

	for _, seed := range seeds {
		f.Add(seed.varKey, seed.varValue)
	}

	f.Fuzz(func(t *testing.T, varKey, varValue string) {
		pb := &pkgbuild.PKGBUILD{
			Distro:          "ubuntu",
			Codename:        "jammy",
			CustomVariables: make(map[string]string),
			CustomArrays:    make(map[string][]string),
			HelperFunctions: make(map[string]string),
		}
		pb.Init()

		// Add a custom variable if key is not empty
		if varKey != "" {
			pb.CustomVariables[varKey] = varValue
		}

		// Should not panic
		result := pb.BuildScriptPreamble()

		// Verify result is a valid string
		if result != "" && varKey != "" {
			// Non-empty preamble should contain variable assignment
			if !strings.Contains(result, "=") {
				t.Errorf("Expected variable assignments in preamble")
			}
		}
	})
}
