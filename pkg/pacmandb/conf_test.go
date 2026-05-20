package pacmandb //nolint:testpackage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseConfig(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a simple pacman.conf
	confPath := filepath.Join(tmpDir, "pacman.conf")
	confContent := `[options]
Architecture = auto
HoldPkg = pacman glibc

[core]
Server = https://mirror1.example.org/$repo/os/$arch
Server = https://mirror2.example.org/$repo/os/$arch

[extra]
Server = https://mirror3.example.org/$repo/os/$arch
`

	err := os.WriteFile(confPath, []byte(confContent), 0o644)
	if err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	cfg, err := parseConfigWithIncludes(confPath, make(map[string]bool))
	if err != nil {
		t.Fatalf("ParseConfig failed: %v", err)
	}

	if cfg.Architecture != "auto" {
		t.Errorf("expected Architecture=auto, got %q", cfg.Architecture)
	}

	if len(cfg.Repos) != 2 {
		t.Errorf("expected 2 repos, got %d", len(cfg.Repos))
	}

	if cfg.Repos[0].Name != "core" {
		t.Errorf("expected first repo name=core, got %q", cfg.Repos[0].Name)
	}

	if len(cfg.Repos[0].Servers) != 2 {
		t.Errorf("expected 2 servers for core, got %d", len(cfg.Repos[0].Servers))
	}

	if cfg.Repos[1].Name != "extra" {
		t.Errorf("expected second repo name=extra, got %q", cfg.Repos[1].Name)
	}
}

func TestParseConfigWithInclude(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a mirrorlist file
	mirrorlistPath := filepath.Join(tmpDir, "mirrorlist")
	mirrorlistContent := `## Mirror list
## Generated on 2024-05-01

# Server = https://mirror1.example.org/$repo/os/$arch
Server = https://mirror2.example.org/$repo/os/$arch
Server = http://mirror3.example.org/$repo/os/$arch
`

	err := os.WriteFile(mirrorlistPath, []byte(mirrorlistContent), 0o644)
	if err != nil {
		t.Fatalf("failed to write mirrorlist: %v", err)
	}

	// Create pacman.conf with Include directive
	confPath := filepath.Join(tmpDir, "pacman.conf")
	confContent := `[options]
Architecture = x86_64

[core]
Include = ` + mirrorlistPath + `
`

	err = os.WriteFile(confPath, []byte(confContent), 0o644)
	if err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	cfg, err := parseConfigWithIncludes(confPath, make(map[string]bool))
	if err != nil {
		t.Fatalf("ParseConfig with Include failed: %v", err)
	}

	if cfg.Architecture != "x86_64" {
		t.Errorf("expected Architecture=x86_64, got %q", cfg.Architecture)
	}

	if len(cfg.Repos) != 1 {
		t.Errorf("expected 1 repo, got %d", len(cfg.Repos))
	}

	if cfg.Repos[0].Name != "core" {
		t.Errorf("expected repo name=core, got %q", cfg.Repos[0].Name)
	}

	if len(cfg.Repos[0].Servers) != 2 {
		t.Errorf("expected 2 servers for core, got %d", len(cfg.Repos[0].Servers))
	}

	if cfg.Repos[0].Servers[0] != "https://mirror2.example.org/$repo/os/$arch" {
		t.Errorf("unexpected server: %q", cfg.Repos[0].Servers[0])
	}
}

func TestParseMirrorlist(t *testing.T) {
	tmpDir := t.TempDir()

	mirrorlistPath := filepath.Join(tmpDir, "mirrorlist")
	mirrorlistContent := `## Mirror list
## Generated on 2024-05-01

# Server = https://commented.example.org/$repo/os/$arch
Server = https://mirror1.example.org/$repo/os/$arch
Server = https://mirror2.example.org/$repo/os/$arch

# Another comment
Server = https://mirror3.example.org/$repo/os/$arch
`

	err := os.WriteFile(mirrorlistPath, []byte(mirrorlistContent), 0o644)
	if err != nil {
		t.Fatalf("failed to write mirrorlist: %v", err)
	}

	servers, err := parseMirrorlist(mirrorlistPath)
	if err != nil {
		t.Fatalf("parseMirrorlist failed: %v", err)
	}

	if len(servers) != 3 {
		t.Errorf("expected 3 servers, got %d", len(servers))
	}

	if servers[0] != "https://mirror1.example.org/$repo/os/$arch" {
		t.Errorf("unexpected first server: %q", servers[0])
	}
}

func TestSubstituteVars(t *testing.T) {
	tests := []struct {
		name     string
		server   string
		repo     string
		arch     string
		expected string
	}{
		{
			name:     "both placeholders",
			server:   "https://mirror.example.org/$repo/os/$arch",
			repo:     "core",
			arch:     "x86_64",
			expected: "https://mirror.example.org/core/os/x86_64",
		},
		{
			name:     "only repo placeholder",
			server:   "https://mirror.example.org/$repo",
			repo:     "extra",
			arch:     "x86_64",
			expected: "https://mirror.example.org/extra",
		},
		{
			name:     "no placeholders",
			server:   "https://mirror.example.org/arch",
			repo:     "core",
			arch:     "x86_64",
			expected: "https://mirror.example.org/arch",
		},
		{
			name:     "trailing slash",
			server:   "https://mirror.example.org/$repo/os/$arch/",
			repo:     "core",
			arch:     "x86_64",
			expected: "https://mirror.example.org/core/os/x86_64",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := substituteVars(tt.server, tt.repo, tt.arch)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestDetectArch(t *testing.T) {
	// This test just verifies the function returns a non-empty string
	// The actual value depends on runtime.GOARCH
	arch := detectArch()
	if arch == "" {
		t.Error("detectArch returned empty string")
	}
}
