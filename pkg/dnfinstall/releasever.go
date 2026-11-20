// SPDX-FileCopyrightText: 2021-2026 Gianluca Boiano
// SPDX-License-Identifier: GPL-3.0-only

package dnfinstall

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// releaseverProvide is the capability the distribution's release package
// provides with the release as its version (rocky-release provides
// "system-release(releasever) = 9"). dnf takes $releasever from it.
const releaseverProvide = "system-release(releasever)"

// releaseverQueryTimeout bounds the rpm query behind readReleasever.
const releaseverQueryTimeout = 30 * time.Second

// hostVarsDirs are the directories of dnf variable files on the host.
var hostVarsDirs = []string{"/etc/dnf/vars", "/etc/yum/vars"}

// commandRunner runs a command and returns its standard output.
type commandRunner func(ctx context.Context, name string, args ...string) ([]byte, error)

func execRunner(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).Output()
}

// readReleasever returns the host's $releasever, cached for the process
// lifetime (it is static host configuration). See resolveReleasever.
var readReleasever = sync.OnceValue(func() string {
	ctx, cancel := context.WithTimeout(context.Background(), releaseverQueryTimeout)
	defer cancel()

	return resolveReleasever(ctx, hostVarsDirs, osReleaseFn, execRunner)
})

// resolveReleasever determines $releasever the way dnf does, in this order:
//
//  1. the "releasever" variable file of the dnf variable directories
//     (/etc/dnf/vars/releasever);
//  2. the version of the "system-release(releasever)" capability provided by
//     an installed package, read with "rpm" (rocky-release-9.7 provides 9, so
//     mirror list URLs name the major release the mirrors serve, not 9.7);
//  3. VERSION_ID of os-release.
//
// The result is empty when none of them yields a value.
func resolveReleasever(
	ctx context.Context, varsDirs []string, osReleasePath string, run commandRunner,
) string {
	if v := releaseverFromVars(varsDirs); v != "" {
		return v
	}

	if v := releaseverFromProvide(ctx, run); v != "" {
		return v
	}

	return releaseverFromOSRelease(osReleasePath)
}

// releaseverFromVars reads the first line of the "releasever" file of the first
// directory that has a non-empty one.
func releaseverFromVars(varsDirs []string) string {
	for _, dir := range varsDirs {
		data, err := os.ReadFile(filepath.Join(dir, "releasever")) //nolint:gosec // fixed host directories
		if err != nil {
			continue
		}

		first, _, _ := strings.Cut(string(data), "\n")
		if v := strings.TrimSpace(first); v != "" {
			return v
		}
	}

	return ""
}

// releaseverFromProvide returns the version of the first installed
// "system-release(releasever)" provide. rpm lists the provides of every package
// that provides the capability; only the matching one carries the release.
func releaseverFromProvide(ctx context.Context, run commandRunner) string {
	out, err := run(ctx, "rpm", "-q", "--whatprovides", releaseverProvide,
		"--queryformat", "[%{PROVIDENAME}\t%{PROVIDEVERSION}\n]")
	if err != nil {
		return ""
	}

	for line := range strings.SplitSeq(string(out), "\n") {
		name, version, ok := strings.Cut(line, "\t")
		if !ok || strings.TrimSpace(name) != releaseverProvide {
			continue
		}

		if v := strings.TrimSpace(version); v != "" {
			return v
		}
	}

	return ""
}

// releaseverFromOSRelease returns VERSION_ID of the os-release file at path, or
// "" when the file cannot be read or has no such field.
func releaseverFromOSRelease(path string) string {
	data, err := os.ReadFile(path) //nolint:gosec // the host os-release file
	if err != nil {
		return ""
	}

	for line := range strings.SplitSeq(string(data), "\n") {
		key, val, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(key) != "VERSION_ID" {
			continue
		}

		return strings.Trim(strings.TrimSpace(val), `"'`)
	}

	return ""
}
