package nfpm_test

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/M0Rf30/yap/v2/pkg/nfpm"
)

// decodeFixture decodes testdata/nfpm.yaml directly with yaml.Unmarshal
// (bypassing Parse/Load's env-expansion/defaulting/validation pipeline) so
// this file can assert the schema's YAML tags transcribe every field
// verbatim, independent of load.go's behaviour (covered in load_test.go).
func decodeFixture(t *testing.T) *nfpm.Config {
	t.Helper()

	data, err := os.ReadFile("testdata/nfpm.yaml")
	require.NoError(t, err)

	var cfg nfpm.Config

	require.NoError(t, yaml.Unmarshal(data, &cfg))

	return &cfg
}

func TestParseConfig_TopLevelFields(t *testing.T) {
	cfg := decodeFixture(t)

	assert.Equal(t, "examplepkg", cfg.Name)
	assert.Equal(t, "amd64", cfg.Arch)
	assert.Equal(t, "linux", cfg.Platform)
	assert.Equal(t, "1.2.3-beta.1+deadbeef", cfg.Version)
	assert.Equal(t, "semver", cfg.VersionSchema)
	assert.Equal(t, "2", cfg.Epoch)
	assert.Equal(t, "3", cfg.Release)
	assert.Equal(t, "rc9", cfg.Prerelease)
	assert.Equal(t, "override123", cfg.VersionMetadata)
	assert.Equal(t, "utils", cfg.Section)
	assert.Equal(t, "extra", cfg.Priority)
	assert.Equal(t, "Test Maintainer <test@example.com>", cfg.Maintainer)
	assert.Equal(t, "An example package exercising every nfpm.yaml field.", cfg.Description)
	assert.Equal(t, "Example Vendor", cfg.Vendor)
	assert.Equal(t, "https://example.com", cfg.Homepage)
	assert.Equal(t, "MIT", cfg.License)
	assert.Equal(t, "changelog.yaml", cfg.Changelog)
	assert.False(t, cfg.DisableGlobbing)
	assert.Equal(t, os.FileMode(0o022), cfg.Umask)
	assert.True(t, cfg.MTime.Equal(time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)))
}

func TestParseConfig_OverridablesTopLevel(t *testing.T) {
	cfg := decodeFixture(t)

	assert.Equal(t, []string{"oldpkg"}, cfg.Replaces)
	assert.Equal(t, []string{"exampleprovides"}, cfg.Provides)
	assert.Equal(t, []string{"libc"}, cfg.Depends)
	assert.Equal(t, []string{"niceext"}, cfg.Recommends)
	assert.Equal(t, []string{"extra-docs"}, cfg.Suggests)
	assert.Equal(t, []string{"badpkg"}, cfg.Conflicts)

	assert.Equal(t, nfpm.Scripts{
		PreInstall:  "echo preinstall",
		PostInstall: "echo postinstall",
		PreRemove:   "echo preremove",
		PostRemove:  "echo postremove",
	}, cfg.Scripts)
}

func TestParseConfig_RPMBlock(t *testing.T) {
	cfg := decodeFixture(t)

	assert.Equal(t, "build.example.com", cfg.RPM.BuildHost)
	assert.Equal(t, "echo pretrans", cfg.RPM.Scripts.PreTrans)
	assert.Equal(t, "echo posttrans", cfg.RPM.Scripts.PostTrans)
	assert.Equal(t, "echo verify", cfg.RPM.Scripts.Verify)
	assert.Equal(t, []string{"postreq"}, cfg.RPM.Requires.Post)
	assert.Equal(t, "Applications/Tools", cfg.RPM.Group)
	assert.Equal(t, "Example RPM summary", cfg.RPM.Summary)
	assert.Equal(t, "zstd", cfg.RPM.Compression)
	assert.Equal(t, "keys/rpm.key", cfg.RPM.Signature.KeyFile)
	assert.Equal(t, "RPMKEYID", cfg.RPM.Signature.KeyID)
	assert.Empty(t, cfg.RPM.Signature.KeyPassphrase,
		"KeyPassphrase is yaml:\"-\"; never read from YAML")
	assert.Equal(t, "RPM Packager <rpm@example.com>", cfg.RPM.Packager)
	assert.Equal(t, []string{"/opt/example"}, cfg.RPM.Prefixes)
	assert.Equal(t, []string{"/var/log/example-extra.log"}, cfg.RPM.Ghosts)
}

func TestParseConfig_DebBlock(t *testing.T) {
	cfg := decodeFixture(t)

	assert.Equal(t, "v7", cfg.Deb.ArchVariant)
	assert.Equal(t, "debian/rules", cfg.Deb.Scripts.Rules)
	assert.Equal(t, "debian/templates", cfg.Deb.Scripts.Templates)
	assert.Equal(t, "debian/config", cfg.Deb.Scripts.Config)
	assert.Equal(t, []string{"/usr/share/example"}, cfg.Deb.Triggers.Interest)
	assert.Equal(t, []string{"/usr/share/example-await"}, cfg.Deb.Triggers.InterestAwait)
	assert.Equal(t, []string{"/usr/share/example-noawait"}, cfg.Deb.Triggers.InterestNoAwait)
	assert.Equal(t, []string{"/usr/share/example-activate"}, cfg.Deb.Triggers.Activate)
	assert.Equal(t, []string{"/usr/share/example-activate-await"}, cfg.Deb.Triggers.ActivateAwait)
	assert.Equal(t, []string{"/usr/share/example-activate-noawait"}, cfg.Deb.Triggers.ActivateNoAwait)
	assert.Equal(t, []string{"oldexample"}, cfg.Deb.Breaks)
	assert.Equal(t, "keys/deb.key", cfg.Deb.Signature.KeyFile)
	assert.Equal(t, "DEBKEYID", cfg.Deb.Signature.KeyID)
	assert.Equal(t, "debsign", cfg.Deb.Signature.Method)
	assert.Equal(t, "origin", cfg.Deb.Signature.Type)
	assert.Equal(t, "Deb Signer <deb@example.com>", cfg.Deb.Signature.Signer)
	assert.Equal(t, "xz", cfg.Deb.Compression)
	assert.Equal(t, map[string]string{
		"Bugs":       "https://example.com/bugs",
		"Multi-Arch": "same",
	}, cfg.Deb.Fields)
	assert.Equal(t, []string{"predepexample"}, cfg.Deb.Predepends)
}

func TestParseConfig_APKBlock(t *testing.T) {
	cfg := decodeFixture(t)

	assert.Equal(t, "keys/apk.rsa", cfg.APK.Signature.KeyFile)
	assert.Equal(t, "APKKEYID", cfg.APK.Signature.KeyID)
	assert.Equal(t, "apk-signing-key", cfg.APK.Signature.KeyName)
	assert.Equal(t, "echo apk-preupgrade", cfg.APK.Scripts.PreUpgrade)
	assert.Equal(t, "echo apk-postupgrade", cfg.APK.Scripts.PostUpgrade)
}

func TestParseConfig_ArchLinuxBlock(t *testing.T) {
	cfg := decodeFixture(t)

	assert.Equal(t, "examplepkg-base", cfg.ArchLinux.Pkgbase)
	assert.Equal(t, "Arch Packager <arch@example.com>", cfg.ArchLinux.Packager)
	assert.Equal(t, "echo arch-preupgrade", cfg.ArchLinux.Scripts.PreUpgrade)
	assert.Equal(t, "echo arch-postupgrade", cfg.ArchLinux.Scripts.PostUpgrade)
}

func TestParseConfig_IPKBlock(t *testing.T) {
	cfg := decodeFixture(t)

	assert.Equal(t, "1.0", cfg.IPK.ABIVersion)
	require.Len(t, cfg.IPK.Alternatives, 1)
	assert.Equal(t, nfpm.IPKAlternative{
		Priority: 100,
		Target:   "/usr/bin/example",
		LinkName: "/usr/bin/example-alt",
	}, cfg.IPK.Alternatives[0])
	assert.True(t, cfg.IPK.AutoInstalled)
	assert.False(t, cfg.IPK.Essential)
	assert.Equal(t, map[string]string{"CustomField": "customvalue"}, cfg.IPK.Fields)
	assert.Equal(t, []string{"ipkpredep"}, cfg.IPK.Predepends)
	assert.Equal(t, []string{"utility"}, cfg.IPK.Tags)
}

func TestParseConfig_Overrides(t *testing.T) {
	cfg := decodeFixture(t)

	require.Contains(t, cfg.Overrides, "deb")
	require.Contains(t, cfg.Overrides, "rpm")

	debOv := cfg.Overrides["deb"]
	assert.Equal(t, []string{"libdeb-only"}, debOv.Depends)
	assert.Equal(t, "echo deb postinstall override", debOv.Scripts.PostInstall)
	assert.Equal(t, "gzip", debOv.Deb.Compression)

	rpmOv := cfg.Overrides["rpm"]
	assert.Equal(t, []string{"librpm-only"}, rpmOv.Depends)
	assert.Equal(t, "System/Libraries", rpmOv.RPM.Group)
	require.Len(t, rpmOv.Contents, 1)
	assert.Equal(t, "/usr/bin/example-rpm-override", rpmOv.Contents[0].Destination)
}

func TestContent_TypeConstants_ExactStrings(t *testing.T) {
	// These are wire values other tools (real nfpm specs, YAP's own
	// rpm/deb builders) depend on verbatim; pin them so an accidental
	// rename is caught immediately.
	assert.Equal(t, "file", nfpm.TypeFile)
	assert.Equal(t, "dir", nfpm.TypeDir)
	assert.Equal(t, "implicit dir", nfpm.TypeImplicitDir)
	assert.Equal(t, "tree", nfpm.TypeTree)
	assert.Equal(t, "symlink", nfpm.TypeSymlink)
	assert.Equal(t, "config", nfpm.TypeConfig)
	assert.Equal(t, "config|noreplace", nfpm.TypeConfigNoReplace)
	assert.Equal(t, "config|missingok", nfpm.TypeConfigMissingOK)
	assert.Equal(t, "config|tree", nfpm.TypeConfigTree)
	assert.Equal(t, "config|noreplace|tree", nfpm.TypeConfigNoReplaceTree)
	assert.Equal(t, "config|missingok|tree", nfpm.TypeConfigMissingOKTree)
	assert.Equal(t, "ghost", nfpm.TypeRPMGhost)
	assert.Equal(t, "doc", nfpm.TypeRPMDoc)
	assert.Equal(t, "licence", nfpm.TypeRPMLicence)
	assert.Equal(t, "license", nfpm.TypeRPMLicense)
	assert.Equal(t, "readme", nfpm.TypeRPMReadme)
	assert.Equal(t, "debian changelog", nfpm.TypeDebChangelog)
}

func TestLoadPackagerConstants_ExactStrings(t *testing.T) {
	assert.Equal(t, "deb", nfpm.PackagerDeb)
	assert.Equal(t, "rpm", nfpm.PackagerRPM)
	assert.Equal(t, "apk", nfpm.PackagerAPK)
	assert.Equal(t, "archlinux", nfpm.PackagerArchLinux)
	assert.Equal(t, "ipk", nfpm.PackagerIPK)
	assert.Equal(t, []string{"deb", "rpm", "apk", "archlinux", "ipk"}, nfpm.Packagers)
}
