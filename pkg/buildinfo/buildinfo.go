// Package buildinfo holds build-time metadata injected via -ldflags.
package buildinfo

// These variables are set at build time via:
//
//	-X github.com/M0Rf30/yap/v2/pkg/buildinfo.Version=...
//	-X github.com/M0Rf30/yap/v2/pkg/buildinfo.Commit=...
//	-X github.com/M0Rf30/yap/v2/pkg/buildinfo.BuildTime=...
//
// When not set (e.g. `go run`), they fall back to the values below.
var (
	// Version is the semantic version string (e.g. "v2.1.0").
	Version = "v2.0.0"
	// Commit is the short git commit hash (e.g. "abc1234").
	Commit = "unknown"
	// BuildTime is the UTC build timestamp in RFC3339 format (e.g. "2026-05-17T12:00:00Z").
	BuildTime = "unknown"
)
