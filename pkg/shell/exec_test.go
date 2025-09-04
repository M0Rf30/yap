package shell

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/M0Rf30/yap/v2/pkg/i18n"
)

func TestNewPackageDecoratedWriter(t *testing.T) {
	var buf bytes.Buffer

	writer := NewPackageDecoratedWriter(&buf, "test-package")

	if writer == nil {
		t.Fatal("NewPackageDecoratedWriter should not return nil")
	}

	if writer.packageName != "test-package" {
		t.Fatalf("Expected package name 'test-package', got '%s'", writer.packageName)
	}
}

func TestNewGitProgressWriter(t *testing.T) {
	var buf bytes.Buffer

	writer := NewGitProgressWriter(&buf, "test-package")

	if writer == nil {
		t.Fatal("NewGitProgressWriter should not return nil")
	}

	if writer.packageName != "test-package" {
		t.Fatalf("Expected package name 'test-package', got '%s'", writer.packageName)
	}
}

func TestPackageDecoratedWriterWrite(t *testing.T) {
	var buf bytes.Buffer

	writer := NewPackageDecoratedWriter(&buf, "test-package")

	testLine := "This is a test line\n"

	n, err := writer.Write([]byte(testLine))
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	if n != len(testLine) {
		t.Fatalf("Expected to write %d bytes, wrote %d", len(testLine), n)
	}

	output := buf.String()
	if !strings.Contains(output, "test-package") {
		t.Fatal("Output should contain package name")
	}

	if !strings.Contains(output, "This is a test line") {
		t.Fatal("Output should contain the original line content")
	}
}

func TestPackageDecoratedWriterWriteEmptyLine(t *testing.T) {
	var buf bytes.Buffer

	writer := NewPackageDecoratedWriter(&buf, "test-package")

	emptyLine := "\n"

	n, err := writer.Write([]byte(emptyLine))
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	if n != len(emptyLine) {
		t.Fatalf("Expected to write %d bytes, wrote %d", len(emptyLine), n)
	}

	output := buf.String()
	if output != emptyLine {
		t.Fatalf("Empty line should be written as-is, got: %q", output)
	}
}

func TestPackageDecoratedWriterWritePartialLine(t *testing.T) {
	var buf bytes.Buffer

	writer := NewPackageDecoratedWriter(&buf, "test-package")

	// Write partial line first
	partialLine := "This is a partial"

	n, err := writer.Write([]byte(partialLine))
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	if n != len(partialLine) {
		t.Fatalf("Expected to write %d bytes, wrote %d", len(partialLine), n)
	}

	// Buffer should be empty since no newline was found
	output := buf.String()
	if output != "" {
		t.Fatalf("Expected no output for partial line, got: %q", output)
	}

	// Complete the line
	completion := " line\n"

	_, err = writer.Write([]byte(completion))
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// Now we should have output
	output = buf.String()
	if !strings.Contains(output, "This is a partial line") {
		t.Fatal("Output should contain the complete line")
	}
}

func TestGitProgressWriterWrite(t *testing.T) {
	var buf bytes.Buffer

	writer := NewGitProgressWriter(&buf, "git-package")

	testLine := "Cloning repository\n"

	n, err := writer.Write([]byte(testLine))
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	if n != len(testLine) {
		t.Fatalf("Expected to write %d bytes, wrote %d", len(testLine), n)
	}

	output := buf.String()
	if !strings.Contains(output, "git-package") {
		t.Fatal("Output should contain package name")
	}

	if !strings.Contains(output, "Cloning repository") {
		t.Fatal("Output should contain the original line content")
	}
}

func TestGitProgressWriterCarriageReturn(t *testing.T) {
	var buf bytes.Buffer

	writer := NewGitProgressWriter(&buf, "git-package")

	// Write a progress line with carriage return
	progressLine := "Progress: 50%\r"

	n, err := writer.Write([]byte(progressLine))
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	if n != len(progressLine) {
		t.Fatalf("Expected to write %d bytes, wrote %d", len(progressLine), n)
	}

	// Should have no output yet for carriage return
	output := buf.String()
	if output != "" {
		t.Fatalf("Expected no output for carriage return line, got: %q", output)
	}
}

func TestExec(t *testing.T) {
	// Test with a simple command that should exist on most systems
	err := Exec(true, "", "echo", "test")
	if err != nil {
		t.Fatalf("Exec failed: %v", err)
	}
}

func TestExecWithContext(t *testing.T) {
	ctx := context.Background()

	// Test with a simple command
	err := ExecWithContext(ctx, true, "", "echo", "test")
	if err != nil {
		t.Fatalf("ExecWithContext failed: %v", err)
	}
}

func TestExecWithContextTimeout(t *testing.T) {
	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Test with a command that should timeout
	err := ExecWithContext(ctx, true, "", "sleep", "1")
	if err == nil {
		t.Fatal("Expected timeout error, got nil")
	}
}

func TestExecInvalidCommand(t *testing.T) {
	// Test with non-existent command
	err := Exec(true, "", "non-existent-command-xyz")
	if err == nil {
		t.Fatal("Expected error for non-existent command, got nil")
	}
}

func TestNormalizeScriptContent(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple command",
			input:    "echo hello",
			expected: "echo hello",
		},
		{
			name:     "command with line continuation",
			input:    "echo \\\nhello",
			expected: "echo hello",
		},
		{
			name:     "multiple commands",
			input:    "echo hello\necho world",
			expected: "echo hello\necho world",
		},
		{
			name:     "command with empty lines",
			input:    "echo hello\n\necho world",
			expected: "echo hello\necho world",
		},
		{
			name:     "complex line continuation",
			input:    "echo hello \\\n  world \\\n  test",
			expected: "echo hello world test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeScriptContent(tt.input)
			if result != tt.expected {
				t.Fatalf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestRunScript(t *testing.T) {
	// Test with a simple echo command
	err := RunScript("echo 'test script'")
	if err != nil {
		t.Fatalf("RunScript failed: %v", err)
	}
}

func TestRunScriptWithPackage(t *testing.T) {
	// Test with a simple echo command and package name
	err := RunScriptWithPackage("echo 'test script'", "test-package")
	if err != nil {
		t.Fatalf("RunScriptWithPackage failed: %v", err)
	}
}

func TestRunScriptEmpty(t *testing.T) {
	// Test with empty script
	err := RunScript("")
	if err != nil {
		t.Fatalf("RunScript with empty script failed: %v", err)
	}
}

func TestRunScriptInvalid(t *testing.T) {
	// Test with invalid script syntax
	err := RunScript("echo 'unclosed quote")
	if err == nil {
		t.Fatal("Expected error for invalid script syntax, got nil")
	}
}

func TestLogScriptContent(t *testing.T) {
	// Test logScriptContent function - this mainly tests that it doesn't panic
	logScriptContent("echo hello\necho world")

	// Test with empty script
	logScriptContent("")

	// Test with script containing line continuations
	logScriptContent("echo hello \\\n  world")
}

// TestExecWithSudoValidation tests only the validation logic to avoid CI issues with sudo
func TestExecWithSudoValidation(t *testing.T) {
	// Initialize i18n for test
	_ = i18n.Init("en")

	// Test individual invalid commands (should be rejected by validation before execution)
	err := ExecWithSudo(true, "", "rm", "test")
	if err == nil {
		t.Fatal("Expected error for rm command, got nil")
	}

	if !strings.Contains(err.Error(), "not allowed for sudo execution") {
		t.Fatalf("Expected validation error for rm, got: %v", err)
	}

	err = ExecWithSudo(true, "", "curl", "test")
	if err == nil {
		t.Fatal("Expected error for curl command, got nil")
	}

	if !strings.Contains(err.Error(), "not allowed for sudo execution") {
		t.Fatalf("Expected validation error for curl, got: %v", err)
	}

	err = ExecWithSudo(true, "", "bash", "test")
	if err == nil {
		t.Fatal("Expected error for bash command, got nil")
	}

	if !strings.Contains(err.Error(), "not allowed for sudo execution") {
		t.Fatalf("Expected validation error for bash, got: %v", err)
	}
}

func TestExecWithSudoContextValidation(t *testing.T) {
	// Initialize i18n for test
	_ = i18n.Init("en")

	ctx := context.Background()

	// Test with invalid command (should be rejected by validation)
	err := ExecWithSudoContext(ctx, true, "", "bash", "test")
	if err == nil {
		t.Fatal("Expected error for bash command, got nil")
	}

	if !strings.Contains(err.Error(), "not allowed for sudo execution") {
		t.Fatalf("Expected validation error for bash, got: %v", err)
	}
}
