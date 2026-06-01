package repo

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/M0Rf30/yap/v2/pkg/aptrepo"
	"github.com/M0Rf30/yap/v2/pkg/constants"
	"github.com/M0Rf30/yap/v2/pkg/errors"
	"github.com/M0Rf30/yap/v2/pkg/i18n"
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
	hostDebArch, targetDebArch, ok := resolveCrossArches(opts.TargetArch)
	if !ok {
		return nil
	}

	distro, codename, err := resolveCrossDistroCodename(opts)
	if err != nil {
		return err
	}

	portsURI := portsURIFor(distro)
	if portsURI == "" {
		logger.Info(i18n.T("logger.repo.info.cross_apt_setup_skipped"), "distro", distro)

		return nil
	}

	if err := configureCrossArchAndSources(distro, codename, hostDebArch, targetDebArch, portsURI); err != nil {
		return err
	}

	return refreshCrossAptIndexes()
}

// resolveCrossArches translates the requested target arch into a DEB arch and
// returns it alongside the host DEB arch. The third return is false when no
// cross-setup is needed (either the request is empty or the host already runs
// on the target architecture).
func resolveCrossArches(targetArch string) (hostDebArch, targetDebArch string, needed bool) {
	if targetArch == "" {
		return "", "", false
	}

	mapping := constants.GetArchMapping()
	hostDebArch = mapping.TranslateArch(constants.FormatDEB, runtime.GOARCH)
	targetDebArch = mapping.TranslateArch(constants.FormatDEB, targetArch)

	if targetDebArch == "" || targetDebArch == hostDebArch {
		return "", "", false
	}

	return hostDebArch, targetDebArch, true
}

// resolveCrossDistroCodename returns the (distro, codename) pair to use,
// reading /etc/os-release for any unset value.
func resolveCrossDistroCodename(opts CrossAptOptions) (distro, codename string, err error) {
	distro = opts.Distro
	codename = opts.Codename

	if distro != "" && codename != "" {
		return distro, codename, nil
	}

	d, c, err := readOSRelease()
	if err != nil {
		return "", "", err
	}

	if distro == "" {
		distro = d
	}

	if codename == "" {
		codename = c
	}

	return distro, codename, nil
}

// configureCrossArchAndSources registers the foreign architecture, restricts
// the native sources, and installs the cross-arch repo entry. It owns the
// imperative side-effects on /var/lib/dpkg/arch and /etc/apt/sources.list*.
func configureCrossArchAndSources(
	distro, codename, hostDebArch, targetDebArch, portsURI string,
) error {
	if err := addDpkgArchitecture(targetDebArch); err != nil {
		return errors.Wrap(err, errors.ErrTypeBuild,
			fmt.Sprintf("repo: dpkg --add-architecture %s", targetDebArch)).
			WithOperation("SetupCrossAPT").
			WithContext("arch", targetDebArch)
	}

	if err := restrictNativeSources(hostDebArch); err != nil {
		return err
	}

	r := Repo{
		Name:       "cross-" + targetDebArch,
		URL:        portsURI,
		Suite:      codename,
		Components: []string{componentMain, "restricted", "universe", "multiverse"},
	}

	keyring := archiveKeyringFor(distro)
	if keyring != "" {
		return writeCrossSource(&r, targetDebArch, codename, distro, keyring)
	}

	return setupDeb(&r)
}

// refreshCrossAptIndexes refreshes the apt index via aptrepo. There is
// no subprocess fallback: a failure here means the cross-arch ports repo
// could not be reached and later install steps would fail anyway.
//
// Partial-success policy: if at least one index was fetched (n > 0) and the
// only errors are unknown-signer / no-trust-anchor failures from third-party
// repos, we treat those as non-fatal warnings. The cross-arch setup only
// requires the ports indexes to succeed; a third-party repo whose key is not
// in the container's trust store should not block the build.
func refreshCrossAptIndexes() error {
	ctx, cancel := context.WithTimeout(context.Background(), aptCommandTimeout)
	defer cancel()

	n, err := aptrepo.Update(ctx)
	if err != nil {
		if n > 0 && aptrepo.IsVerificationError(err) {
			// Some indexes succeeded (ports fetched) and the only failures
			// are signature/trust issues on other repos — non-fatal.
			logger.Warn(i18n.T("logger.repo.warn.cross_apt_update_some"), "indexes_ok", n, "error", err)
		} else {
			return errors.Wrap(err, errors.ErrTypeBuild,
				"repo: cross apt update via aptrepo").
				WithOperation("SetupCrossAPT")
		}
	}

	logger.Info(i18n.T("logger.repo.info.cross_apt_indexes_refreshed"), "indexes", n)

	return nil
}

const (
	ubuntuPortsURI = "http://ports.ubuntu.com/ubuntu-ports/"
	debianPortsURI = "http://deb.debian.org/debian-ports/"
)

// portsURIFor returns the per-distro ports archive URI used for non-primary
// architectures, or "" when the distro is not supported.
func portsURIFor(distro string) string {
	switch distro {
	case constants.DistroUbuntu:
		return ubuntuPortsURI
	case constants.DistroDebian:
		return debianPortsURI
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
	if err := os.MkdirAll(debSourcesDir, 0o755); err != nil {
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
	if err := os.WriteFile(dst, []byte(body), 0o644); err != nil { //nolint:gosec
		return errors.Wrap(err, errors.ErrTypeFileSystem,
			fmt.Sprintf("repo %q: write %s", r.Name, dst)).
			WithOperation("writeCrossSource").
			WithContext("path", dst)
	}

	logger.Info(i18n.T("logger.repo.info.installed_cross_arch_apt"), "name", r.Name,
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
	data, err := os.ReadFile(path) //nolint:gosec
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
	data, err := os.ReadFile(debSourcesList)
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

// addDpkgArchitecture registers a foreign architecture with dpkg by appending
// it to /var/lib/dpkg/arch. This is equivalent to
// "dpkg --add-architecture <arch>": dpkg itself only reads and writes that
// plain-text file (one arch per line). The function is idempotent — it is a
// no-op when the arch is already listed.
func addDpkgArchitecture(arch string) error {
	const dpkgArchFile = "/var/lib/dpkg/arch"

	// Read existing content (file may not exist on minimal images).
	data, err := os.ReadFile(dpkgArchFile)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	// Check if the arch is already registered.
	for line := range strings.SplitSeq(string(data), "\n") {
		if strings.TrimSpace(line) == arch {
			logger.Info(i18n.T("logger.repo.info.dpkg_arch_already_registered"), "arch", arch)

			return nil
		}
	}

	// Append the new arch. Ensure the file ends with a newline.
	content := strings.TrimRight(string(data), "\n")
	if content != "" {
		content += "\n"
	}

	content += arch + "\n"

	if err := os.MkdirAll(filepath.Dir(dpkgArchFile), 0o755); err != nil {
		return err
	}

	// /var/lib/dpkg/arch is a dpkg-internal file; 0o644 matches dpkg's own mode.
	if err := os.WriteFile(dpkgArchFile, []byte(content), 0o644); err != nil { //nolint:gosec
		return err
	}

	logger.Info(i18n.T("logger.repo.info.registered_dpkg_architecture"), "arch", arch)

	return nil
}

// writeRoot writes content to a path under /etc. yap is expected to run as
// root (CI invokes it via `sudo yap ...`), so a direct write is enough — there
// is no need for a sudo tee detour.
func writeRoot(path string, data []byte) error {
	// path is composed of package constants plus a sanitized repo name; the
	// caller controls the value and apt requires world-readable mode.
	if err := os.WriteFile(path, data, 0o644); err != nil { //nolint:gosec
		return errors.Wrap(err, errors.ErrTypeFileSystem,
			fmt.Sprintf("repo: write %s", path)).
			WithOperation("writeRoot").
			WithContext("path", path)
	}

	return nil
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
