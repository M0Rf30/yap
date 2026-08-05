package mcp_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

const testPKGBUILD = `pkgname=hello
pkgver=1.0.0
pkgrel=1
pkgdesc="Hello world test package"
maintainer="Test <test@example.com>"
arch=('x86_64')
license=('MIT')
url="https://example.com/hello"
depends=('glibc')

package() {
  install -Dm644 /dev/null "${pkgdir}/usr/share/doc/hello/README"
}
`

const testNfpmYAML = `name: hello
version: 1.0.0
description: Hello world test package
maintainer: Test <test@example.com>
license: MIT
depends:
  - glibc
`

const testNfpmYAMLWithChangelog = testNfpmYAML + "changelog: changelog.yaml\n"

const testChangelogYAML = `name: hello
entries:
  - semver: "1.0.0-1"
    date: 2025-01-01T00:00:00Z
    changes:
      - note: "Initial release"
`

// callConvertSpec drives convert_spec through the registered MCP tool
// (not by calling the internal helpers directly) and returns the
// structured result alongside the raw CallToolResult.
func callConvertSpec(t *testing.T, args map[string]any) (map[string]any, *mcpsdk.CallToolResult) {
	t.Helper()

	cs, cleanup := connectClient(t)
	defer cleanup()

	res, err := cs.CallTool(context.Background(), &mcpsdk.CallToolParams{
		Name:      "convert_spec",
		Arguments: args,
	})
	if err != nil {
		t.Fatalf("CallTool convert_spec: %v", err)
	}

	if res.IsError {
		return nil, res
	}

	out, ok := res.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("structured content is %T, want map[string]any: %+v", res.StructuredContent, res.StructuredContent)
	}

	return out, res
}

func TestConvertSpec_NfpmToPKGBUILD_TextReturn(t *testing.T) {
	dir := t.TempDir()

	specPath := filepath.Join(dir, "nfpm.yaml")
	if err := os.WriteFile(specPath, []byte(testNfpmYAML), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	out, res := callConvertSpec(t, map[string]any{"specPath": specPath})
	if res.IsError {
		t.Fatalf("convert_spec returned error: %+v", res.Content)
	}

	if got := out["detected"]; got != "nfpm" {
		t.Errorf("detected = %v, want nfpm", got)
	}

	if got := out["to"]; got != "pkgbuild" {
		t.Errorf("to = %v, want pkgbuild", got)
	}

	if out["outputPath"] != nil {
		t.Errorf("outputPath = %v, want unset for text-return path", out["outputPath"])
	}

	rendered, _ := out["rendered"].(string)
	if rendered == "" {
		t.Fatal("rendered is empty; expected PKGBUILD text")
	}

	if len(res.Content) == 0 {
		t.Fatal("expected non-empty text content")
	}

	text, ok := res.Content[0].(*mcpsdk.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", res.Content[0])
	}

	if text.Text == "" {
		t.Error("text content is empty")
	}
}

func TestConvertSpec_PKGBUILDToNfpm_FileWrite(t *testing.T) {
	dir := t.TempDir()

	pkgbuildPath := filepath.Join(dir, "PKGBUILD")
	if err := os.WriteFile(pkgbuildPath, []byte(testPKGBUILD), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	outputPath := filepath.Join(dir, "out", "nfpm.yaml")

	out, res := callConvertSpec(t, map[string]any{
		"specPath":   dir,
		"outputPath": outputPath,
	})
	if res.IsError {
		t.Fatalf("convert_spec returned error: %+v", res.Content)
	}

	if got := out["detected"]; got != "pkgbuild" {
		t.Errorf("detected = %v, want pkgbuild", got)
	}

	if got := out["to"]; got != "nfpm" {
		t.Errorf("to = %v, want nfpm", got)
	}

	if got := out["outputPath"]; got != outputPath {
		t.Errorf("outputPath = %v, want %v", got, outputPath)
	}

	if out["rendered"] != nil && out["rendered"] != "" {
		t.Errorf("rendered = %v, want unset for file-write path", out["rendered"])
	}

	written, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read written output: %v", err)
	}

	if len(written) == 0 {
		t.Fatal("written nfpm.yaml is empty")
	}

	bytesFloat, ok := out["bytes"].(float64)
	if !ok || int(bytesFloat) != len(written) {
		t.Errorf("bytes = %v, want %d", out["bytes"], len(written))
	}
}

func TestConvertSpec_InvalidTo(t *testing.T) {
	dir := t.TempDir()

	specPath := filepath.Join(dir, "nfpm.yaml")
	if err := os.WriteFile(specPath, []byte(testNfpmYAML), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	_, res := callConvertSpec(t, map[string]any{
		"specPath": specPath,
		"to":       "rpm",
	})
	if !res.IsError {
		t.Fatal("expected an error for invalid to value")
	}
}

func TestConvertSpec_MissingSpecFile(t *testing.T) {
	dir := t.TempDir()

	_, res := callConvertSpec(t, map[string]any{
		"specPath": filepath.Join(dir, "does-not-exist.yaml"),
	})
	if !res.IsError {
		t.Fatal("expected an error for a missing spec file")
	}
}

// TestConvertSpec_NfpmChangelogMessageSurfaced covers the changelog: field
// via the text-return path (no outputPath, so no sidecar directory to write
// into): the changelog= directive is still emitted and the "not written"
// message reaches both the structured dropped list and the "## Dropped
// features" heading in the tool's text content.
func TestConvertSpec_NfpmChangelogMessageSurfaced(t *testing.T) {
	dir := t.TempDir()

	specPath := filepath.Join(dir, "nfpm.yaml")
	if err := os.WriteFile(specPath, []byte(testNfpmYAMLWithChangelog), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "changelog.yaml"), []byte(testChangelogYAML), 0o644); err != nil {
		t.Fatalf("write changelog fixture: %v", err)
	}

	out, res := callConvertSpec(t, map[string]any{"specPath": specPath})
	if res.IsError {
		t.Fatalf("convert_spec returned error: %+v", res.Content)
	}

	dropped, ok := out["dropped"].([]any)
	if !ok || len(dropped) != 1 {
		t.Fatalf("dropped = %v, want exactly one message", out["dropped"])
	}

	msg, _ := dropped[0].(string)
	if !strings.Contains(msg, "hello.changelog") {
		t.Errorf("dropped message = %q, want it to mention hello.changelog", msg)
	}

	rendered, _ := out["rendered"].(string)
	if !strings.Contains(rendered, `changelog="hello.changelog"`) {
		t.Errorf("rendered = %q, want a changelog= directive", rendered)
	}

	if len(res.Content) == 0 {
		t.Fatal("expected non-empty text content")
	}

	text, ok := res.Content[0].(*mcpsdk.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", res.Content[0])
	}

	if !strings.Contains(text.Text, "## Dropped features") {
		t.Errorf("text content missing Dropped features heading: %q", text.Text)
	}

	if !strings.Contains(text.Text, "hello.changelog") {
		t.Errorf("text content missing changelog message: %q", text.Text)
	}
}
