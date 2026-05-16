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
	if err := os.MkdirAll(rpmRepoDir, 0o755); err != nil { // #nosec G301
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

		if err := exec.CommandContext(ctx, "rpm", "--import", gpgKey).Run(); err != nil {
			return errors.Wrap(err, errors.ErrTypeBuild,
				fmt.Sprintf("repo %q: rpm --import %s", r.Name, gpgKey)).
				WithOperation("setupRPM").
				WithContext("repo", r.Name).
				WithContext("key", gpgKey)
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
	if err := os.WriteFile(dst, []byte(b.String()), 0o644); err != nil { // #nosec G306
		return errors.Wrap(err, errors.ErrTypeFileSystem,
			fmt.Sprintf("repo %q: write %s", r.Name, dst)).
			WithOperation("setupRPM").
			WithContext("path", dst)
	}

	logger.Info("repo: installed yum repo",
		"name", r.Name,
		"path", dst,
		"gpgcheck", r.GPGCheck)

	return nil
}
