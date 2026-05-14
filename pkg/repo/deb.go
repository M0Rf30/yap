package repo

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/M0Rf30/yap/v2/pkg/logger"
)

const (
	debKeyringsDir = "/etc/apt/keyrings"
	debSourcesDir  = "/etc/apt/sources.list.d"
)

// setupDeb writes a deb822 .sources file under /etc/apt/sources.list.d/ and
// places the signing key under /etc/apt/keyrings/. The active apt is expected
// to support the deb822 Signed-By directive (Ubuntu Focal+, Debian Bullseye+).
func setupDeb(r *Repo) error {
	if r.Suite == "" {
		return fmt.Errorf("repo %q: suite is required for deb format", r.Name)
	}

	components := r.Components
	if len(components) == 0 {
		components = []string{"main"}
	}

	signedBy := ""

	if r.KeyURL != "" {
		keyPath := filepath.Join(debKeyringsDir, "yap-"+r.Name+".asc")
		if err := fetchKey(r.KeyURL, keyPath); err != nil {
			return err
		}

		signedBy = keyPath
	}

	// /etc/apt/sources.list.d is a documented Debian system directory and
	// must remain traversable for unprivileged apt operations.
	if err := os.MkdirAll(debSourcesDir, 0o755); err != nil { // #nosec G301
		return err
	}

	var b strings.Builder

	fmt.Fprintf(&b, "Types: deb\n")
	fmt.Fprintf(&b, "URIs: %s\n", r.URL)
	fmt.Fprintf(&b, "Suites: %s\n", r.Suite)
	fmt.Fprintf(&b, "Components: %s\n", strings.Join(components, " "))

	if signedBy != "" {
		fmt.Fprintf(&b, "Signed-By: %s\n", signedBy)
	} else {
		fmt.Fprintf(&b, "Trusted: yes\n")
	}

	dst := filepath.Join(debSourcesDir, "yap-"+r.Name+".sources")
	// apt must read this file as the unprivileged _apt user, so it has to be
	// world-readable; gosec's stricter 0o600 default does not apply here.
	if err := os.WriteFile(dst, []byte(b.String()), 0o644); err != nil { // #nosec G306
		return fmt.Errorf("repo %q: write %s: %w", r.Name, dst, err)
	}

	logger.Info("repo: installed apt source",
		"name", r.Name,
		"path", dst,
		"signed", signedBy != "")

	return nil
}
