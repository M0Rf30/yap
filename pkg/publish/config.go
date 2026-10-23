// SPDX-FileCopyrightText: 2021-2026 Gianluca Boiano
// SPDX-License-Identifier: GPL-3.0-only

// Package publish uploads built packages to an OpenArtifact repository
// (https://github.com/openartifact/openartifact).
//
// DEB and RPM packages go through OpenArtifact's native upload APIs, which
// regenerate the apt/dnf indexes server-side. APK and Pacman packages (for
// which OpenArtifact has no dedicated format yet) are stored in a hosted raw
// repository under a configurable path prefix. Every upload is verified
// against the SHA-256 reported by the server, retried with exponential
// backoff on transient failures and idempotent: re-publishing identical bytes
// is a no-op. Hosted deb/rpm packages are immutable, so re-publishing the same
// name and version with other bytes fails with [ErrConflict]; raw files are
// replaceable unless the repository enforces immutable releases.
package publish

import (
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/M0Rf30/yap/v2/pkg/errors"
)

// Environment variable names read by [Config.ApplyEnv].
const (
	EnvURL          = "YAP_OA_URL"
	EnvToken        = "YAP_OA_TOKEN"
	EnvTokenFile    = "YAP_OA_TOKEN_FILE"
	EnvDebRepo      = "YAP_OA_DEB_REPO"
	EnvRPMRepo      = "YAP_OA_RPM_REPO"
	EnvAPKRepo      = "YAP_OA_APK_REPO"
	EnvPacmanRepo   = "YAP_OA_PACMAN_REPO"
	EnvDistribution = "YAP_OA_DISTRIBUTION"
	EnvComponent    = "YAP_OA_COMPONENT"
	EnvAPKPath      = "YAP_OA_APK_PATH"
	EnvPacmanPath   = "YAP_OA_PACMAN_PATH"
)

// Defaults applied when a value is not configured.
const (
	DefaultDistribution = "stable"
	DefaultComponent    = "main"
	DefaultTimeout      = 30 * time.Minute
)

// Secret holds a credential. It never renders its value through fmt, JSON or
// slog, so it cannot leak into logs or error messages by accident; use
// [Secret.Reveal] where the raw value is required.
type Secret string

// Reveal returns the raw secret value.
func (s Secret) Reveal() string { return string(s) }

// String implements fmt.Stringer.
func (s Secret) String() string { return redacted(s) }

// GoString implements fmt.GoStringer (the %#v verb).
func (s Secret) GoString() string { return redacted(s) }

// MarshalText implements encoding.TextMarshaler (JSON, YAML, …).
func (s Secret) MarshalText() ([]byte, error) { return []byte(redacted(s)), nil }

// LogValue implements slog.LogValuer.
func (s Secret) LogValue() slog.Value { return slog.StringValue(redacted(s)) }

func redacted(s Secret) string {
	if s == "" {
		return ""
	}

	return "[REDACTED]"
}

// Config describes the target OpenArtifact server and repositories.
//
// The JSON form is the "publish" object of yap.json. The token is deliberately
// not part of it: supply it with YAP_OA_TOKEN, YAP_OA_TOKEN_FILE or
// --token-file.
type Config struct {
	// URL is the OpenArtifact base URL, for example https://oa.example.com.
	URL string `json:"url,omitempty"`
	// DebRepo is the hosted deb repository receiving .deb files.
	DebRepo string `json:"debRepo,omitempty"`
	// RPMRepo is the hosted rpm repository receiving .rpm files.
	RPMRepo string `json:"rpmRepo,omitempty"`
	// APKRepo is the hosted raw repository receiving .apk files.
	APKRepo string `json:"apkRepo,omitempty"`
	// PacmanRepo is the hosted raw repository receiving .pkg.tar.* files.
	PacmanRepo string `json:"pacmanRepo,omitempty"`
	// Distribution is the apt distribution (suite) for .deb uploads.
	Distribution string `json:"distribution,omitempty"`
	// Component is the apt component for .deb uploads.
	Component string `json:"component,omitempty"`
	// APKPath is the path prefix inside APKRepo (for example "v3.20/main/x86_64").
	APKPath string `json:"apkPath,omitempty"`
	// PacmanPath is the path prefix inside PacmanRepo (for example "x86_64").
	PacmanPath string `json:"pacmanPath,omitempty"`

	// Token is the OpenArtifact API token, sent as "Authorization: Bearer".
	Token Secret `json:"-"`
	// AllowInsecureHTTP permits sending the token over plain http to a
	// non-loopback host.
	AllowInsecureHTTP bool `json:"-"`
	// Timeout bounds one HTTP request, including the body transfer.
	Timeout time.Duration `json:"-"`
}

// ApplyEnv overrides fields from the YAP_OA_* environment variables. Only
// variables that are set and non-empty change anything. A token file named by
// YAP_OA_TOKEN_FILE is used when YAP_OA_TOKEN is unset.
func (c *Config) ApplyEnv(getenv func(string) string) error {
	set := func(dst *string, key string) {
		if v := strings.TrimSpace(getenv(key)); v != "" {
			*dst = v
		}
	}

	set(&c.URL, EnvURL)
	set(&c.DebRepo, EnvDebRepo)
	set(&c.RPMRepo, EnvRPMRepo)
	set(&c.APKRepo, EnvAPKRepo)
	set(&c.PacmanRepo, EnvPacmanRepo)
	set(&c.Distribution, EnvDistribution)
	set(&c.Component, EnvComponent)
	set(&c.APKPath, EnvAPKPath)
	set(&c.PacmanPath, EnvPacmanPath)

	if v := strings.TrimSpace(getenv(EnvToken)); v != "" {
		c.Token = Secret(v)

		return nil
	}

	if file := strings.TrimSpace(getenv(EnvTokenFile)); file != "" {
		return c.LoadTokenFile(file)
	}

	return nil
}

// LoadTokenFile reads the API token from path (surrounding whitespace is
// trimmed).
func (c *Config) LoadTokenFile(path string) error {
	data, err := os.ReadFile(path) //nolint:gosec // user-selected token file
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem, "cannot read token file").
			WithOperation("LoadTokenFile").
			WithContext("path", path)
	}

	token := strings.TrimSpace(string(data))
	if token == "" {
		return errors.New(errors.ErrTypeConfiguration, "token file is empty").
			WithOperation("LoadTokenFile").
			WithContext("path", path)
	}

	c.Token = Secret(token)

	return nil
}

// RepoFor returns the repository configured for a package format, or "".
func (c *Config) RepoFor(format string) string {
	switch format {
	case FormatDeb:
		return c.DebRepo
	case FormatRPM:
		return c.RPMRepo
	case FormatAPK:
		return c.APKRepo
	case FormatPacman:
		return c.PacmanRepo
	default:
		return ""
	}
}

func (c *Config) pathFor(format string) string {
	switch format {
	case FormatAPK:
		return c.APKPath
	case FormatPacman:
		return c.PacmanPath
	default:
		return ""
	}
}

// validate normalises the configuration and checks the target URL and token.
func (c *Config) validate() (*url.URL, error) {
	if strings.TrimSpace(c.URL) == "" {
		return nil, errors.New(errors.ErrTypeConfiguration,
			"OpenArtifact URL is not set (--url, "+EnvURL+" or yap.json publish.url)").
			WithOperation("validate")
	}

	base, err := url.Parse(strings.TrimSpace(c.URL))
	if err != nil || base.Host == "" || (base.Scheme != "https" && base.Scheme != "http") {
		return nil, errors.New(errors.ErrTypeConfiguration,
			"OpenArtifact URL must be an absolute http(s) URL").
			WithOperation("validate")
	}

	if base.User != nil {
		return nil, errors.New(errors.ErrTypeConfiguration,
			"OpenArtifact URL must not embed credentials; use "+EnvToken+" or a token file").
			WithOperation("validate")
	}

	if c.Token == "" {
		return nil, errors.New(errors.ErrTypeConfiguration,
			"OpenArtifact API token is not set ("+EnvToken+", "+EnvTokenFile+" or --token-file)").
			WithOperation("validate")
	}

	if base.Scheme == "http" && !c.AllowInsecureHTTP && !isLoopback(base.Hostname()) {
		return nil, errors.New(errors.ErrTypeConfiguration,
			"refusing to send the API token over plain http; use https or --insecure-http").
			WithOperation("validate").
			WithContext("host", base.Host)
	}

	base.RawQuery, base.Fragment = "", ""
	base.Path = strings.TrimRight(base.Path, "/")

	return base, nil
}

func isLoopback(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}

	ip := net.ParseIP(host)

	return ip != nil && ip.IsLoopback()
}

// String renders the configuration for diagnostics; the token is redacted.
func (c Config) String() string {
	return fmt.Sprintf("publish.Config{URL:%q DebRepo:%q RPMRepo:%q APKRepo:%q PacmanRepo:%q Token:%s}",
		c.URL, c.DebRepo, c.RPMRepo, c.APKRepo, c.PacmanRepo, c.Token)
}
