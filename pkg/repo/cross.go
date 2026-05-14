package repo

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/M0Rf30/yap/v2/pkg/constants"
	"github.com/M0Rf30/yap/v2/pkg/logger"
)

const (
	debSourcesList    = "/etc/apt/sources.list"
	osReleasePath     = "/etc/os-release"
	aptCommandTimeout = 2 * time.Minute
)

// CrossAptOptions describes the configuration required to make `apt` resolve
// foreign-architecture packages on the build host. Distro and codename can be
// left empty: when so, both are read from /etc/os-release.
type CrossAptOptions struct {
	Distro     string
	Codename   string
	TargetArch string
}

// SetupCrossAPT prepares the apt configuration for cross-architecture builds.
// On Debian-like hosts it:
//
//  1. enables the target architecture via dpkg --add-architecture;
//  2. restricts the pre-installed sources to the build-host architecture so
//     apt does not fetch foreign-arch indexes from archive.ubuntu.com (which
//     only carries amd64/i386);
//  3. registers the matching ports.ubuntu.com / deb.debian.org repository for
//     the target arch through the standard repo.Setup pipeline; and
//  4. refreshes the apt indexes so subsequent dependency installs resolve
//     packages from the new repo.
//
// The function is a no-op when the host already runs on the requested target
// architecture or when the distro is not Debian-based.
func SetupCrossAPT(opts CrossAptOptions) error {
	if opts.TargetArch == "" {
		return nil
	}

	hostDebArch := constants.GetArchMapping().TranslateArch(constants.FormatDEB, runtime.GOARCH)
	targetDebArch := constants.GetArchMapping().TranslateArch(constants.FormatDEB, opts.TargetArch)

	if targetDebArch == "" || targetDebArch == hostDebArch {
		return nil
	}

	distro := opts.Distro
	codename := opts.Codename

	if distro == "" || codename == "" {
		d, c, err := readOSRelease()
		if err != nil {
			return err
		}

		if distro == "" {
			distro = d
		}

		if codename == "" {
			codename = c
		}
	}

	portsURI := portsURIFor(distro)
	if portsURI == "" {
		logger.Info("repo: cross apt setup skipped (not a debian-like distro)",
			"distro", distro)

		return nil
	}

	if err := runTool(aptCommandTimeout, "dpkg", "--add-architecture", targetDebArch); err != nil {
		return fmt.Errorf("repo: dpkg --add-architecture %s: %w", targetDebArch, err)
	}

	if err := restrictNativeSources(hostDebArch); err != nil {
		return err
	}

	r := Repo{
		Name:       "yap-cross-" + targetDebArch,
		URL:        portsURI,
		Suite:      codename,
		Components: []string{componentMain, "restricted", "universe", "multiverse"},
	}

	keyring := archiveKeyringFor(distro)
	if keyring != "" {
		if err := writeCrossSource(&r, targetDebArch, codename, distro, keyring); err != nil {
			return err
		}
	} else if err := setupDeb(&r); err != nil {
		return err
	}

	return runTool(aptCommandTimeout, "apt-get", "update")
}

// portsURIFor returns the per-distro ports archive URI used for non-primary
// architectures, or "" when the distro is not supported.
func portsURIFor(distro string) string {
	switch distro {
	case constants.DistroUbuntu:
		return "http://ports.ubuntu.com/ubuntu-ports/"
	case constants.DistroDebian:
		return "http://deb.debian.org/debian-ports/"
	}

	return ""
}

// archiveKeyringFor returns the absolute path to the distro archive keyring
// shipped by the runtime image, or "" when no canonical keyring is known.
func archiveKeyringFor(distro string) string {
	switch distro {
	case constants.DistroUbuntu:
		return "/usr/share/keyrings/ubuntu-archive-keyring.gpg"
	case constants.DistroDebian:
		return "/usr/share/keyrings/debian-archive-keyring.gpg"
	}

	return ""
}

// writeCrossSource emits a deb822 .sources file restricted to the target arch
// and signed by the local archive keyring. The standard setupDeb path is not
// reused because it cannot constrain Architectures or point at a pre-installed
// keyring without an extra KeyURL fetch.
func writeCrossSource(r *Repo, targetDebArch, codename, distro, keyring string) error {
	// /etc/apt/sources.list.d is a documented Debian system directory.
	if err := os.MkdirAll(debSourcesDir, 0o755); err != nil { // #nosec G301
		return err
	}

	suites := codename

	if distro == constants.DistroUbuntu {
		suites = fmt.Sprintf("%s %s-updates %s-security", codename, codename, codename)
	}

	body := fmt.Sprintf(
		"Types: deb\nURIs: %s\nSuites: %s\nComponents: %s\nArchitectures: %s\nSigned-By: %s\n",
		r.URL,
		suites,
		strings.Join(r.Components, " "),
		targetDebArch,
		keyring,
	)

	dst := filepath.Join(debSourcesDir, "yap-"+r.Name+".sources")
	// apt must read this file as the unprivileged _apt user.
	if err := os.WriteFile(dst, []byte(body), 0o644); err != nil { // #nosec G306
		return fmt.Errorf("repo %q: write %s: %w", r.Name, dst, err)
	}

	logger.Info("repo: installed cross-arch apt source",
		"name", r.Name,
		"arch", targetDebArch,
		"suites", suites,
		"path", dst)

	return nil
}

// restrictNativeSources adds an `Architectures: <host>` constraint (deb822
// files) or an `[arch=<host>]` qualifier (legacy sources.list) to every
// pre-installed apt source so foreign-arch indexes are only fetched from the
// repository we just registered. The operation is idempotent.
func restrictNativeSources(hostDebArch string) error {
	if err := restrictDeb822Sources(hostDebArch); err != nil {
		return err
	}

	return restrictLegacySourcesList(hostDebArch)
}

// deb822TypesLine matches the first `Types:` directive in a deb822 stanza so
// the patch can be inserted immediately after it.
var deb822TypesLine = regexp.MustCompile(`(?m)^Types:.*$`)

// legacyDebLine matches lines that introduce a deb / deb-src entry without an
// existing `[arch=...]` qualifier.
var legacyDebLine = regexp.MustCompile(`(?m)^(deb(?:-src)?)\s+(http|https|ftp)`)

func restrictDeb822Sources(hostDebArch string) error {
	entries, err := os.ReadDir(debSourcesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}

		return err
	}

	for _, e := range entries {
		if e.IsDir() {
			continue
		}

		if !strings.HasSuffix(e.Name(), ".sources") {
			continue
		}

		if strings.HasPrefix(e.Name(), "yap-") {
			continue
		}

		path := filepath.Join(debSourcesDir, e.Name())
		if err := patchDeb822File(path, hostDebArch); err != nil {
			return err
		}
	}

	return nil
}

func patchDeb822File(path, hostDebArch string) error {
	// path is constrained to /etc/apt/sources.list.d; the entry name is sourced
	// from os.ReadDir and is not user-controlled at runtime.
	data, err := os.ReadFile(path) // #nosec G304
	if err != nil {
		return err
	}

	if strings.Contains(string(data), "Architectures:") {
		return nil
	}

	patched := deb822TypesLine.ReplaceAllStringFunc(string(data), func(match string) string {
		return match + "\nArchitectures: " + hostDebArch
	})

	return writeRoot(path, []byte(patched))
}

func restrictLegacySourcesList(hostDebArch string) error {
	data, err := os.ReadFile(debSourcesList) // #nosec G304
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}

		return err
	}

	original := string(data)
	patched := legacyDebLine.ReplaceAllString(original, "$1 [arch="+hostDebArch+"] $2")

	if patched == original {
		return nil
	}

	return writeRoot(debSourcesList, []byte(patched))
}

// writeRoot writes content to a path under /etc. yap is expected to run as
// root (CI invokes it via `sudo yap ...`), so a direct write is enough — there
// is no need for a sudo tee detour.
func writeRoot(path string, data []byte) error {
	// path is composed of package constants plus a sanitized repo name; the
	// caller controls the value and apt requires world-readable mode.
	if err := os.WriteFile(path, data, 0o644); err != nil { // #nosec G306,G703
		return fmt.Errorf("repo: write %s: %w", path, err)
	}

	return nil
}

// runTool invokes the named tool directly inside a bounded context. yap is
// expected to run as root; on non-root invocations the underlying tool will
// return its standard EPERM error and surface it to the caller.
func runTool(timeout time.Duration, name string, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// name and args are limited to constant strings supplied by callers in
	// this package (dpkg, apt-get, ...); there is no flow from user input.
	cmd := exec.CommandContext(ctx, name, args...) // #nosec G204
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// readOSRelease parses /etc/os-release and returns the distro ID and version
// codename. Unknown values surface as empty strings rather than errors.
func readOSRelease() (distro string, codename string, err error) {
	data, err := os.ReadFile(osReleasePath)
	if err != nil {
		return "", "", err
	}

	for raw := range strings.SplitSeq(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}

		val = strings.Trim(val, `"'`)

		switch key {
		case "ID":
			distro = val
		case "VERSION_CODENAME":
			codename = val
		}
	}

	return distro, codename, nil
}
