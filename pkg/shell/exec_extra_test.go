package shell

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/M0Rf30/yap/v2/pkg/i18n"
)

//nolint:gochecknoinits // Required to initialise i18n before test functions run
func init() {
	_ = i18n.Init("en")
}

// ---------------------------------------------------------------------------
// extractErrorLines
// ---------------------------------------------------------------------------

func TestExtractErrorLines_AllProgress(t *testing.T) {
	raw := strings.Join([]string{
		"go: downloading foo v1.0.0",
		"Downloading bar",
		"fetching baz",
		"resolving deps",
		"compiling pkg",
		"installing stuff",
		"updating index",
		"warning: something",
		"note: see docs",
		"info: verbose",
		"hint: try this",
		"+ set -e",
	}, "\n")

	got := extractErrorLines(raw, "fallback")
	assert.Equal(t, "fallback", got, "all-progress input should return fallback")
}

func TestExtractErrorLines_MixedContent(t *testing.T) {
	raw := "go: downloading foo\nERROR: build failed\nsome other error"
	got := extractErrorLines(raw, "fallback")
	assert.Contains(t, got, "ERROR: build failed")
	assert.Contains(t, got, "some other error")
	assert.NotContains(t, got, "go: downloading foo")
}

func TestExtractErrorLines_EmptyInput(t *testing.T) {
	got := extractErrorLines("", "my fallback")
	assert.Equal(t, "my fallback", got)
}

func TestExtractErrorLines_OnlyBlanks(t *testing.T) {
	got := extractErrorLines("   \n\n\t\n", "fb")
	assert.Equal(t, "fb", got)
}

func TestExtractErrorLines_PureErrors(t *testing.T) {
	raw := "error: something went wrong\nfailed to build"
	got := extractErrorLines(raw, "fallback")
	assert.Equal(t, "error: something went wrong\nfailed to build", got)
}

func TestExtractErrorLines_CaseInsensitiveSkip(t *testing.T) {
	// "Warning: " (capital W) should still be skipped
	raw := "Warning: deprecated API\nreal error here"
	got := extractErrorLines(raw, "fb")
	assert.NotContains(t, got, "Warning")
	assert.Contains(t, got, "real error here")
}

// ---------------------------------------------------------------------------
// normalizeScriptContent
// ---------------------------------------------------------------------------

func TestNormalizeScriptContent_LineContinuation(t *testing.T) {
	script := "echo hello \\\n  world"
	got := normalizeScriptContent(script)
	assert.Equal(t, "echo hello world", got)
}

func TestNormalizeScriptContent_EmptyLines(t *testing.T) {
	script := "echo a\n\necho b"
	got := normalizeScriptContent(script)
	assert.Equal(t, "echo a\necho b", got)
}

func TestNormalizeScriptContent_MultiContinuation(t *testing.T) {
	script := "cmake \\\n  -DFOO=bar \\\n  -DBAZ=qux \\\n  .."
	got := normalizeScriptContent(script)
	assert.Equal(t, "cmake -DFOO=bar -DBAZ=qux ..", got)
}

func TestNormalizeScriptContent_NoChange(t *testing.T) {
	script := "echo hello\necho world"
	got := normalizeScriptContent(script)
	assert.Equal(t, script, got)
}

func TestNormalizeScriptContent_Empty(t *testing.T) {
	got := normalizeScriptContent("")
	assert.Equal(t, "", got)
}

func TestNormalizeScriptContent_TrailingContinuation(t *testing.T) {
	// Trailing backslash with no following line — should still flush
	script := "echo foo \\"
	got := normalizeScriptContent(script)
	assert.Equal(t, "echo foo", got)
}

// ---------------------------------------------------------------------------
// ExecWithContext — success and failure paths
// ---------------------------------------------------------------------------

func TestExecWithContext_Success(t *testing.T) {
	ctx := context.Background()
	err := ExecWithContext(ctx, true, "", "true")
	require.NoError(t, err)
}

func TestExecWithContext_Failure(t *testing.T) {
	ctx := context.Background()
	err := ExecWithContext(ctx, true, "", "false")
	require.Error(t, err)
}

func TestExecWithContext_NonExistentCommand(t *testing.T) {
	ctx := context.Background()
	err := ExecWithContext(ctx, true, "", "this-command-does-not-exist-yap-test")
	require.Error(t, err)
}

func TestExecWithContext_WithDir(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	err := ExecWithContext(ctx, true, dir, "true")
	require.NoError(t, err)
}

func TestExecWithContext_CancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	err := ExecWithContext(ctx, true, "", "sleep", "10")
	require.Error(t, err)
}

// ---------------------------------------------------------------------------
// logScriptResult — success and error branches, with/without packageName
// ---------------------------------------------------------------------------

func TestLogScriptResult_SuccessWithPackage(t *testing.T) {
	buf := &bytes.Buffer{}
	err := logScriptResult(nil, "mypkg", 100*time.Millisecond, buf, "TestOp")
	assert.NoError(t, err)
}

func TestLogScriptResult_SuccessNoPackage(t *testing.T) {
	buf := &bytes.Buffer{}
	err := logScriptResult(nil, "", 50*time.Millisecond, buf, "TestOp")
	assert.NoError(t, err)
}

func TestLogScriptResult_ErrorWithPackage(t *testing.T) {
	buf := bytes.NewBufferString("go: downloading foo\nERROR: build failed")
	inputErr := assert.AnError
	err := logScriptResult(inputErr, "mypkg", 200*time.Millisecond, buf, "TestOp")
	require.Error(t, err)
}

func TestLogScriptResult_ErrorNoPackage(t *testing.T) {
	buf := bytes.NewBufferString("some error output")
	inputErr := assert.AnError
	err := logScriptResult(inputErr, "", 10*time.Millisecond, buf, "TestOp")
	require.Error(t, err)
}

// ---------------------------------------------------------------------------
// GitProgressWriter.Close — the 0% branch
// ---------------------------------------------------------------------------

func TestGitProgressWriterClose(t *testing.T) {
	var buf bytes.Buffer

	gpw := NewGitProgressWriter(&buf, "test-pkg")
	// Close without writing anything — should not panic
	err := gpw.Close()
	assert.NoError(t, err)
}

// ---------------------------------------------------------------------------
// ExecWithSudoContext — allowed commands validation (pure logic, no execution)
// ---------------------------------------------------------------------------

func TestExecWithSudoContext_AllowedCommandsPassValidation(t *testing.T) {
	// These commands are in the allowlist. We verify they pass the validation
	// gate (no "not allowed" error).
	//
	// Set SUDO_USER so needsSudo=false — the command runs directly (not via
	// sudo) and fails immediately because the binary doesn't exist on PATH.
	// This avoids any interactive sudo password prompt.
	t.Setenv("SUDO_USER", "testuser")

	allowed := []string{"pacman", "dnf", "yum", "apt-get", "apt", "apk", "dpkg", "rpm", "makepkg", "zypper"}

	for _, cmd := range allowed {
		t.Run(cmd, func(t *testing.T) {
			ctx := context.Background()
			err := ExecWithSudoContext(ctx, true, "", cmd, "--yap-test-nonexistent-flag")
			// The command may or may not be installed; either way the error
			// must NOT be the "not allowed for sudo execution" validation error.
			if err != nil {
				assert.NotContains(t, err.Error(), "not allowed for sudo execution",
					"allowed command %q should not be rejected by validation", cmd)
			}
		})
	}
}

func TestExecWithSudoContext_DisallowedCommandsRejected(t *testing.T) {
	disallowed := []string{"bash", "sh", "curl", "wget", "rm", "cat", "python3"}

	for _, cmd := range disallowed {
		t.Run(cmd, func(t *testing.T) {
			ctx := context.Background()
			err := ExecWithSudoContext(ctx, true, "", cmd, "--version")
			require.Error(t, err)
			assert.Contains(t, err.Error(), "not allowed for sudo execution")
		})
	}
}
