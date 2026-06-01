package repo

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/M0Rf30/yap/v2/pkg/errors"
	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
)

const (
	rpmGPGKeyDir     = "/etc/pki/rpm-gpg"
	rpmRepoDir       = "/etc/yum.repos.d"
	rpmImportTimeout = 30 * time.Second
)

// setupRPM writes a yum/dnf .repo file under /etc/yum.repos.d/ and imports the
// signing key with rpm --import when KeyURL is set.
func setupRPM(r *Repo) error {
	// /etc/yum.repos.d is a documented system directory and must remain
	// traversable for unprivileged dnf/yum operations.
	if err := os.MkdirAll(rpmRepoDir, 0o755); err != nil {
		return err
	}

	gpgKey := ""

	if r.KeyURL != "" {
		gpgKey = filepath.Join(rpmGPGKeyDir, "RPM-GPG-KEY-yap-"+r.Name)
		if err := fetchKey(r.KeyURL, gpgKey); err != nil {
			return err
		}

		ctx, cancel := context.WithTimeout(context.Background(), rpmImportTimeout)
		defer cancel()

		// rpm --import registers the key in the RPM database so that rpm -V
		// (file verification) works. It is best-effort: dnf/yum still verify
		// package signatures against the gpgkey= path in the .repo file even
		// when the key is not in the RPM DB. Log a warning on failure rather
		// than aborting the build.
		if err := exec.CommandContext(ctx, "rpm", "--import", gpgKey).Run(); err != nil {
			logger.Warn(i18n.T("logger.repo.warn.rpm_import_failed_non"), "repo", r.Name,
				"key", gpgKey,
				"error", err)
		}
	}

	var b strings.Builder

	fmt.Fprintf(&b, "[%s]\n", r.Name)
	fmt.Fprintf(&b, "name=%s\n", r.Name)
	fmt.Fprintf(&b, "baseurl=%s\n", r.URL)
	fmt.Fprintf(&b, "enabled=1\n")

	if r.GPGCheck && gpgKey != "" {
		fmt.Fprintf(&b, "gpgcheck=1\n")
		fmt.Fprintf(&b, "gpgkey=file://%s\n", gpgKey)
	} else {
		fmt.Fprintf(&b, "gpgcheck=0\n")
	}

	dst := filepath.Join(rpmRepoDir, "yap-"+r.Name+".repo")
	// dnf reads /etc/yum.repos.d files as the unprivileged update process, so
	// world-readable 0o644 is the documented mode.
	if err := os.WriteFile(dst, []byte(b.String()), 0o644); err != nil { //nolint:gosec
		return errors.Wrap(err, errors.ErrTypeFileSystem,
			fmt.Sprintf("repo %q: write %s", r.Name, dst)).
			WithOperation("setupRPM").
			WithContext("path", dst)
	}

	logger.Info(i18n.T("logger.repo.info.installed_yum_repo"), "name", r.Name,
		"path", dst,
		"gpgcheck", r.GPGCheck)

	return nil
}
