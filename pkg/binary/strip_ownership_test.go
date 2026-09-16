package binary

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// As root under sudo, every path created by SeparateDebugInfoWithEnv must end
// up owned by the original user, otherwise CI agents cannot archive the tree.
func TestSeparateDebugInfoPreservesOwnership(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("needs root")
	}

	t.Setenv("SUDO_USER", "nobody")
	t.Setenv("SUDO_UID", "1000")
	t.Setenv("SUDO_GID", "1000")

	dir := t.TempDir()
	bin := filepath.Join(dir, "true")

	data, err := os.ReadFile("/bin/true")
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(bin, data, 0o755); err != nil {
		t.Fatal(err)
	}

	debugDir := filepath.Join(dir, "debug-symbols")
	if err := os.MkdirAll(debugDir, 0o750); err != nil {
		t.Fatal(err)
	}

	debugFile, err := SeparateDebugInfo(bin, debugDir)
	if err != nil {
		t.Fatal(err)
	}

	if debugFile == "" {
		t.Fatal("no build-id in /bin/true")
	}

	for _, p := range []string{filepath.Join(debugDir, ".build-id"), filepath.Dir(debugFile), debugFile} {
		info, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}

		st, _ := info.Sys().(*syscall.Stat_t)
		t.Logf("%s uid=%d gid=%d mode=%v", p, st.Uid, st.Gid, info.Mode())

		if st.Uid != 1000 || st.Gid != 1000 {
			t.Errorf("%s owned by %d:%d, want 1000:1000", p, st.Uid, st.Gid)
		}
	}
}
