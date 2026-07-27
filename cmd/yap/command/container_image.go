package command

import (
	yapErrors "github.com/M0Rf30/yap/v2/pkg/errors"
	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
	"github.com/M0Rf30/yap/v2/pkg/platform"
)

// ResolveContainerImage maps a requested distro family and optional release
// onto the yap builder image tag to dispatch into. The returned tag selects
// only the BUILD ENVIRONMENT: it never changes the package identity (the
// distro/release that reaches the PKGBUILD and the release suffix).
func ResolveContainerImage(distro, release string) (string, error) {
	host, _ := platform.ParseOSRelease()

	return resolveContainerImage(distro, release, host)
}

// resolveContainerImage is the pure, host-injected core of
// ResolveContainerImage so resolution can be unit tested without touching
// the real /etc/os-release. Rules are evaluated in order:
//
//  1. An explicit release always wins: "<distro>-<release>".
//  2. A few distro families publish a single, unqualified builder image
//     (Alpine and Arch track a rolling release) — the bare family name is
//     itself a valid tag.
//  3. A bare family matching the host's own distro is back-filled with the
//     host's VERSION_CODENAME, so dispatch picks the same environment a
//     native (non-container) build on that host would use.
//  4. Anything else has no image to dispatch into: fail with an actionable
//     validation error instead of guessing a tag that does not exist.
func resolveContainerImage(distro, release string, host platform.OSRelease) (string, error) {
	if release != "" {
		return distro + "-" + release, nil
	}

	if distro == alpineDistro || distro == archDistro {
		return distro, nil
	}

	if distro == host.ID && host.Codename != "" {
		image := distro + "-" + host.Codename

		logger.Info(i18n.T("logger.build.auto_detected_codename"),
			"distro", distro, "codename", host.Codename, "image", image)

		return image, nil
	}

	return "", yapErrors.New(yapErrors.ErrTypeValidation,
		i18n.T("errors.validation.unresolvable_container_image")).
		WithOperation("ResolveContainerImage").
		WithContext("distro", distro)
}
