package nfpm_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/M0Rf30/yap/v2/pkg/constants"
	"github.com/M0Rf30/yap/v2/pkg/nfpm"
)

// minimalSpec is the smallest document that passes Validate: name, version,
// and (via top-level arch) the arch requirement.
const minimalSpec = "name: pkg\nversion: \"1.0.0\"\narch: amd64\n"

func TestLoadPackagerForFormat_And_FormatForPackager(t *testing.T) {
	tests := []struct {
		format   string
		packager string
	}{
		{constants.FormatDEB, nfpm.PackagerDeb},
		{constants.FormatRPM, nfpm.PackagerRPM},
		{constants.FormatAPK, nfpm.PackagerAPK},
		{constants.FormatPacman, nfpm.PackagerArchLinux},
	}

	for _, tt := range tests {
		t.Run(tt.format, func(t *testing.T) {
			assert.Equal(t, tt.packager, nfpm.PackagerForFormat(tt.format))
			assert.Equal(t, tt.format, nfpm.FormatForPackager(tt.packager))
		})
	}

	assert.Empty(t, nfpm.PackagerForFormat("unknown-format"))
	assert.Empty(t, nfpm.FormatForPackager("unknown-packager"))
	assert.Empty(t, nfpm.FormatForPackager(nfpm.PackagerIPK), "yap has no IPK build format")
}

func TestParse_StrictUnknownTopLevelField(t *testing.T) {
	_, err := nfpm.Parse(strings.NewReader(minimalSpec + "bogus_top_level_key: 1\n"))
	require.Error(t, err)
}

func TestParse_StrictUnknownNestedField(t *testing.T) {
	_, err := nfpm.Parse(strings.NewReader(minimalSpec + "rpm:\n  bogus_nested_key: 1\n"))
	require.Error(t, err)
}

func TestParse_StrictUnknownOverrideField(t *testing.T) {
	spec := minimalSpec + "overrides:\n  deb:\n    bogus_override_key: 1\n"
	_, err := nfpm.Parse(strings.NewReader(spec))
	require.Error(t, err)
}

func TestParse_ValidMinimalSpec(t *testing.T) {
	cfg, err := nfpm.Parse(strings.NewReader(minimalSpec))
	require.NoError(t, err)
	assert.Equal(t, "pkg", cfg.Name)
	assert.Equal(t, "1.0.0", cfg.Version)
}

func TestParse_EnvExpansion(t *testing.T) {
	t.Setenv("DEP_EXTRA", "libextra")
	t.Setenv("PKG_NAME", "envpkg")

	spec := "name: ${PKG_NAME}\nversion: \"1.0.0\"\narch: amd64\n" +
		"depends:\n  - libc\n  - ${DEP_EXTRA}\n" +
		"rpm:\n  packager: \"$PKG_NAME <rpm@example.com>\"\n" +
		"  requires:\n    post:\n      - ${DEP_EXTRA}\n" +
		"  signature:\n    key_file: keys/${PKG_NAME}.key\n    key_id: ${PKG_NAME}ID\n" +
		"deb:\n  predepends:\n    - ${DEP_EXTRA}\n" +
		"  signature:\n    key_file: keys/${PKG_NAME}-deb.key\n    key_id: ${PKG_NAME}DEBID\n" +
		"  fields:\n    Source: \"${PKG_NAME}-source\"\n" +
		"apk:\n  signature:\n    key_file: keys/${PKG_NAME}.rsa\n    key_id: ${PKG_NAME}APKID\n" +
		"ipk:\n  predepends:\n    - ${DEP_EXTRA}\n  fields:\n    Custom: \"${PKG_NAME}-ipk\"\n" +
		"overrides:\n  rpm:\n    depends:\n      - ${DEP_EXTRA}\n"

	cfg, err := nfpm.Parse(strings.NewReader(spec))
	require.NoError(t, err)

	assert.Equal(t, "envpkg", cfg.Name)
	assert.Equal(t, []string{"libc", "libextra"}, cfg.Depends)
	assert.Equal(t, "envpkg <rpm@example.com>", cfg.RPM.Packager)
	assert.Equal(t, []string{"libextra"}, cfg.RPM.Requires.Post)
	assert.Equal(t, "keys/envpkg.key", cfg.RPM.Signature.KeyFile)
	assert.Equal(t, "envpkgID", cfg.RPM.Signature.KeyID)
	assert.Equal(t, []string{"libextra"}, cfg.Deb.Predepends)
	assert.Equal(t, "keys/envpkg-deb.key", cfg.Deb.Signature.KeyFile)
	assert.Equal(t, "envpkgDEBID", cfg.Deb.Signature.KeyID)
	assert.Equal(t, "envpkg-source", cfg.Deb.Fields["Source"])
	assert.Equal(t, "keys/envpkg.rsa", cfg.APK.Signature.KeyFile)
	assert.Equal(t, "envpkgAPKID", cfg.APK.Signature.KeyID)
	assert.Equal(t, []string{"libextra"}, cfg.IPK.Predepends)
	assert.Equal(t, "envpkg-ipk", cfg.IPK.Fields["Custom"])

	// Expansion applies to Overrides entries too, not just the base.
	assert.Equal(t, []string{"libextra"}, cfg.Overrides["rpm"].Depends)
}

func TestParse_ContentExpandGate(t *testing.T) {
	t.Setenv("BIN_NAME", "example")

	spec := minimalSpec +
		"contents:\n" +
		"  - src: bin/x\n    dst: /usr/bin/${BIN_NAME}\n    type: file\n    expand: true\n" +
		"  - src: bin/y\n    dst: /usr/bin/${BIN_NAME}-literal\n    type: file\n"

	cfg, err := nfpm.Parse(strings.NewReader(spec))
	require.NoError(t, err)
	require.Len(t, cfg.Contents, 2)

	assert.Equal(t, "/usr/bin/example", cfg.Contents[0].Destination, "expand:true content is expanded")
	assert.Equal(t, "/usr/bin/${BIN_NAME}-literal", cfg.Contents[1].Destination,
		"expand:false (default) content is left completely untouched")
}

func TestParseWithEnvMapping_CustomResolver(t *testing.T) {
	spec := minimalSpec + "maintainer: \"${WHOEVER}\"\n"

	cfg, err := nfpm.ParseWithEnvMapping(strings.NewReader(spec), func(key string) string {
		if key == "WHOEVER" {
			return "Custom Resolver"
		}

		return ""
	})
	require.NoError(t, err)
	assert.Equal(t, "Custom Resolver", cfg.Maintainer)
}

func TestParse_PassphraseResolution(t *testing.T) {
	t.Setenv("NFPM_PASSPHRASE", "generic-pass")
	t.Setenv("NFPM_RPM_PASSPHRASE", "rpm-pass")

	cfg, err := nfpm.Parse(strings.NewReader(minimalSpec))
	require.NoError(t, err)

	assert.Equal(t, "rpm-pass", cfg.RPM.Signature.KeyPassphrase, "packager-specific var wins")
	assert.Equal(t, "generic-pass", cfg.Deb.Signature.KeyPassphrase, "falls back to the generic var")
	assert.Equal(t, "generic-pass", cfg.APK.Signature.KeyPassphrase)
}

func TestParse_PassphraseNeverReadFromYAML(t *testing.T) {
	// no passphrase key exists in the schema
	spec := minimalSpec + "rpm:\n  signature:\n    key_id: X\n"
	cfg, err := nfpm.Parse(strings.NewReader(spec))
	require.NoError(t, err)
	assert.Empty(t, cfg.RPM.Signature.KeyPassphrase)
}

func TestLoad_SetsBaseDir(t *testing.T) {
	cfg, err := nfpm.Load("testdata/nfpm.yaml")
	require.NoError(t, err)
	assert.Equal(t, "testdata", cfg.BaseDir())
}

func TestLoad_MissingFile(t *testing.T) {
	_, err := nfpm.Load("testdata/does-not-exist.yaml")
	require.Error(t, err)
}

func TestLoadConfig_SetBaseDir(t *testing.T) {
	cfg, err := nfpm.Parse(strings.NewReader(minimalSpec))
	require.NoError(t, err)
	assert.Empty(t, cfg.BaseDir(), "Parse never sets a base dir")

	cfg.SetBaseDir("/tmp/somewhere")
	assert.Equal(t, "/tmp/somewhere", cfg.BaseDir())
}

func TestWithDefaults_EveryDefault(t *testing.T) {
	cfg, err := nfpm.Parse(strings.NewReader("name: pkg\nversion: \"\"\n"))
	require.NoError(t, err)

	assert.Equal(t, "linux", cfg.Platform)
	assert.Equal(t, "amd64", cfg.Arch)
	assert.Equal(t, "no description given", cfg.Description)
	assert.Equal(t, "1", cfg.Release)
	assert.Equal(t, "optional", cfg.Priority)
	assert.False(t, cfg.MTime.IsZero())
	// version defaulted to v0.0.0-rc0 then split: version=0.0.0, prerelease=rc0.
	assert.Equal(t, "0.0.0", cfg.Version)
	assert.Equal(t, "rc0", cfg.Prerelease)
	assert.Empty(t, cfg.VersionMetadata)
}

func TestWithDefaults_UmaskDefault(t *testing.T) {
	cfg := (&nfpm.Config{Info: nfpm.Info{Name: "x", Version: "1.0.0"}}).WithDefaults()
	assert.Equal(t, 0o02, int(cfg.Umask))
}

func TestWithDefaults_MTimeFromSourceDateEpoch(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", "1700000000")

	cfg := (&nfpm.Config{Info: nfpm.Info{Name: "x", Version: "1.0.0"}}).WithDefaults()
	assert.Equal(t, int64(1700000000), cfg.MTime.Unix())
}

func TestWithDefaults_ExplicitValuesWin(t *testing.T) {
	cfg := &nfpm.Config{Info: nfpm.Info{
		Name: "x", Version: "1.2.3-beta.9", Platform: "linux", Arch: "arm64",
		Description: "custom description", Release: "7", Priority: "required",
		Prerelease: "already-set", VersionMetadata: "already-set-meta",
	}}
	cfg.WithDefaults()

	assert.Equal(t, "arm64", cfg.Arch)
	assert.Equal(t, "custom description", cfg.Description)
	assert.Equal(t, "7", cfg.Release)
	assert.Equal(t, "required", cfg.Priority)
	// Version is always rewritten to the split numeric part...
	assert.Equal(t, "1.2.3", cfg.Version)
	// ...but explicit Prerelease/VersionMetadata are never overwritten.
	assert.Equal(t, "already-set", cfg.Prerelease)
	assert.Equal(t, "already-set-meta", cfg.VersionMetadata)
}

func TestWithDefaults_VersionSchemaNoneSkipsSplit(t *testing.T) {
	cfg := &nfpm.Config{Info: nfpm.Info{
		Name: "x", Version: "1.2.3-beta.1", VersionSchema: "none",
	}}
	cfg.WithDefaults()

	assert.Equal(t, "1.2.3-beta.1", cfg.Version, "version_schema: none leaves Version untouched")
	assert.Empty(t, cfg.Prerelease)
}

func TestValidate_Failures(t *testing.T) {
	// Name/Version/Arch are unreachable as Validate failures through
	// Parse/Load, since WithDefaults always fills them first — exercise
	// Validate directly, bypassing WithDefaults, for those three.
	directCases := []struct {
		name string
		cfg  *nfpm.Config
	}{
		{
			"missing name",
			&nfpm.Config{Info: nfpm.Info{Version: "1.0.0", Arch: "amd64", Platform: "linux"}},
		},
		{"missing version", &nfpm.Config{Info: nfpm.Info{Name: "pkg", Arch: "amd64", Platform: "linux"}}},
		{
			"missing arch with no per-packager fallback",
			&nfpm.Config{Info: nfpm.Info{Name: "pkg", Version: "1.0.0", Platform: "linux"}},
		},
	}

	for _, tt := range directCases {
		t.Run(tt.name, func(t *testing.T) {
			require.Error(t, tt.cfg.Validate())
		})
	}

	// The rest are never defaulted away, so they're reachable end to end
	// through Parse.
	specCases := []struct {
		name string
		spec string
	}{
		{"platform not linux", "name: pkg\nversion: \"1.0.0\"\narch: amd64\nplatform: darwin\n"},
		{
			"unknown override key",
			minimalSpec + "overrides:\n  bogus:\n    depends:\n      - x\n",
		},
		{
			"unknown content type on base",
			minimalSpec + "contents:\n  - dst: /x\n    type: bogus-type\n",
		},
		{
			"unknown content type on override",
			minimalSpec + "overrides:\n  deb:\n    contents:\n      - dst: /x\n        type: bogus-type\n",
		},
		{
			"duplicate content destination same packager",
			minimalSpec + "contents:\n  - src: a\n    dst: /x\n  - src: b\n    dst: /x\n",
		},
	}

	for _, tt := range specCases {
		t.Run(tt.name, func(t *testing.T) {
			_, err := nfpm.Parse(strings.NewReader(tt.spec))
			require.Error(t, err)
		})
	}
}

func TestValidate_PerPackagerArchSatisfiesRequirement(t *testing.T) {
	tests := []string{
		"name: pkg\nversion: \"1.0.0\"\ndeb:\n  arch: amd64\n",
		"name: pkg\nversion: \"1.0.0\"\nrpm:\n  arch: x86_64\n",
		"name: pkg\nversion: \"1.0.0\"\napk:\n  arch: x86_64\n",
		"name: pkg\nversion: \"1.0.0\"\narchlinux:\n  arch: x86_64\n",
		"name: pkg\nversion: \"1.0.0\"\nipk:\n  arch: x86_64\n",
	}

	for _, spec := range tests {
		_, err := nfpm.Parse(strings.NewReader(spec))
		assert.NoError(t, err, spec)
	}
}

func TestValidate_DuplicateDestinationIsPerPackager(t *testing.T) {
	// Same destination, but restricted to two different packagers: no
	// conflict, since each packager's resolved set only ever sees one of
	// them.
	spec := minimalSpec +
		"contents:\n" +
		"  - src: a\n    dst: /x\n    packager: deb\n" +
		"  - src: b\n    dst: /x\n    packager: rpm\n"

	_, err := nfpm.Parse(strings.NewReader(spec))
	assert.NoError(t, err)
}

func TestValidate_DuplicateDestinationErrorNamesBothSources(t *testing.T) {
	spec := minimalSpec + "contents:\n  - src: a\n    dst: /x\n  - src: b\n    dst: /x\n"

	_, err := nfpm.Parse(strings.NewReader(spec))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "\"a\"")
	assert.Contains(t, err.Error(), "\"b\"")
}

func TestLoadForPackager_UnknownPackager(t *testing.T) {
	cfg, err := nfpm.Parse(strings.NewReader(minimalSpec))
	require.NoError(t, err)

	_, err = cfg.ForPackager("bogus")
	require.Error(t, err)
}

func TestLoadForPackager_OverrideMergeIsGranularReplaceNotAppend(t *testing.T) {
	cfg, err := nfpm.Load("testdata/nfpm.yaml")
	require.NoError(t, err)

	deb, err := cfg.ForPackager(nfpm.PackagerDeb)
	require.NoError(t, err)

	// Overridden fields replace wholesale.
	assert.Equal(t, []string{"libdeb-only"}, deb.Depends,
		"override replaces, never appends, to base depends")
	assert.Equal(t, "echo deb postinstall override", deb.Scripts.PostInstall)
	assert.Equal(t, "gzip", deb.Deb.Compression)

	// Non-overridden sibling fields on the SAME struct survive untouched —
	// proves the merge is leaf-field granular, not whole-block replace.
	assert.Equal(t, "v7", deb.Deb.ArchVariant)
	assert.Equal(t, "debian/rules", deb.Deb.Scripts.Rules)
	assert.Equal(t, []string{"oldexample"}, deb.Deb.Breaks)
	assert.Equal(t, "echo preinstall", deb.Scripts.PreInstall,
		"unoverridden Scripts field keeps the base value")

	rpm, err := cfg.ForPackager(nfpm.PackagerRPM)
	require.NoError(t, err)

	assert.Equal(t, []string{"librpm-only"}, rpm.Depends)
	assert.Equal(t, "System/Libraries", rpm.RPM.Group, "override replaces rpm.group")
	assert.Equal(t, "build.example.com", rpm.RPM.BuildHost,
		"unoverridden rpm.buildhost keeps the base value")
	assert.Equal(t, "Example RPM summary", rpm.RPM.Summary)
}

func TestForPackager_ContentsOverrideReplacesBaseWholesale(t *testing.T) {
	cfg, err := nfpm.Load("testdata/nfpm.yaml")
	require.NoError(t, err)

	rpm, err := cfg.ForPackager(nfpm.PackagerRPM)
	require.NoError(t, err)

	require.Len(t, rpm.Contents, 1,
		"rpm override declares its own Contents, replacing the base list entirely")
	assert.Equal(t, "/usr/bin/example-rpm-override", rpm.Contents[0].Destination)
}

func TestForPackager_ContentsFallBackToBaseWhenOverrideOmitsThem(t *testing.T) {
	cfg, err := nfpm.Load("testdata/nfpm.yaml")
	require.NoError(t, err)

	deb, err := cfg.ForPackager(nfpm.PackagerDeb)
	require.NoError(t, err)

	dests := make(map[string]bool, len(deb.Contents))
	for _, c := range deb.Contents {
		dests[c.Destination] = true
	}

	assert.True(t, dests["/usr/bin/example"], "deb override has no contents: falls back to base")
	assert.False(t, dests["/usr/bin/example-rpm-only"],
		"packager-restricted rpm-only entry must not leak into deb")

	apk, err := cfg.ForPackager(nfpm.PackagerAPK)
	require.NoError(t, err)

	apkDests := make(map[string]bool, len(apk.Contents))
	for _, c := range apk.Contents {
		apkDests[c.Destination] = true
	}

	assert.True(t, apkDests["/usr/bin/example"], "apk has no override at all: uses base contents")
	assert.False(t, apkDests["/usr/bin/example-rpm-only"])
}

func TestLoadForPackager_EveryPackagerResolvesTheFixtureWithoutError(t *testing.T) {
	cfg, err := nfpm.Load("testdata/nfpm.yaml")
	require.NoError(t, err)

	for _, packager := range nfpm.Packagers {
		t.Run(packager, func(t *testing.T) {
			info, err := cfg.ForPackager(packager)
			require.NoError(t, err)
			assert.NotEmpty(t, info.Contents)

			for _, c := range info.Contents {
				assert.True(t, strings.HasPrefix(c.Destination, "/"))
				require.NotNil(t, c.FileInfo)
			}
		})
	}
}

func TestLoadForPackager_TreeAndConfigContentTreeExpandedForEveryPackager(t *testing.T) {
	cfg, err := nfpm.Load("testdata/nfpm.yaml")
	require.NoError(t, err)

	apk, err := cfg.ForPackager(nfpm.PackagerAPK)
	require.NoError(t, err)

	var sawTreeFile, sawConfigTreeFile bool

	for _, c := range apk.Contents {
		switch c.Destination {
		case "/usr/share/example/bin/tool":
			sawTreeFile = true

			assert.Equal(t, nfpm.TypeFile, c.Type)
		case "/etc/example/configtree/app.conf":
			sawConfigTreeFile = true

			assert.Equal(t, nfpm.TypeConfig, c.Type)
		}
	}

	assert.True(t, sawTreeFile, "plain tree must expand into leaf files")
	assert.True(t, sawConfigTreeFile, "config|tree leaves must become type=config")
}

func TestWithDefaults_ReturnsSelfForChaining(t *testing.T) {
	cfg := &nfpm.Config{Info: nfpm.Info{Name: "x", Version: "1.0.0"}}
	require.Same(t, cfg, cfg.WithDefaults())
}

func TestParse_MTimeParsedAsRFC3339(t *testing.T) {
	cfg, err := nfpm.Parse(strings.NewReader(minimalSpec + "mtime: 2025-01-15T10:00:00Z\n"))
	require.NoError(t, err)
	assert.True(t, cfg.MTime.Equal(time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)))
}

// erroringReader always fails, exercising Parse's io.ReadAll error path.
type erroringReader struct{}

func (erroringReader) Read([]byte) (int, error) { return 0, errors.New("boom") }

func TestParse_ReadError(t *testing.T) {
	_, err := nfpm.Parse(erroringReader{})
	require.Error(t, err)
}

func TestLoad_PropagatesParseError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nfpm.yaml")
	require.NoError(t, os.WriteFile(path, []byte("name: pkg\nbogus_top_level_key: 1\n"), 0o644))

	_, err := nfpm.Load(path)
	require.Error(t, err)
}

func TestLoadForPackager_PropagatesPrepareForPackagerError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nfpm.yaml")
	spec := minimalSpec + "contents:\n  - src: does-not-exist\n    dst: /x\n    type: file\n"
	require.NoError(t, os.WriteFile(path, []byte(spec), 0o644))

	cfg, err := nfpm.Load(path)
	require.NoError(t, err)

	_, err = cfg.ForPackager(nfpm.PackagerDeb)
	require.Error(t, err)
}

func TestLoadForPackager_IPKMergeExercisesBothBranches(t *testing.T) {
	spec := minimalSpec +
		"ipk:\n" +
		"  abi_version: \"1.0\"\n" +
		"  auto_installed: true\n" +
		"  essential: false\n" +
		"  fields:\n    Base: baseval\n" +
		"  alternatives:\n    - priority: 1\n      target: /a\n      link_name: /b\n" +
		"overrides:\n" +
		"  ipk:\n" +
		"    ipk:\n" +
		"      abi_version: \"2.0\"\n" +
		"      essential: true\n" +
		"      fields:\n        Override: overrideval\n" +
		"      alternatives:\n        - priority: 2\n          target: /c\n          link_name: /d\n"

	cfg, err := nfpm.Parse(strings.NewReader(spec))
	require.NoError(t, err)

	info, err := cfg.ForPackager(nfpm.PackagerIPK)
	require.NoError(t, err)

	assert.Equal(t, "2.0", info.IPK.ABIVersion, "overridden string field replaces")
	assert.True(t, info.IPK.AutoInstalled, "unoverridden bool field keeps the base's true")
	assert.True(t, info.IPK.Essential, "overridden bool field (true) wins over the base's false")
	assert.Equal(t, map[string]string{"Override": "overrideval"}, info.IPK.Fields,
		"overridden map field replaces the base map wholesale, never merges keys")
	require.Len(t, info.IPK.Alternatives, 1,
		"overridden slice field replaces the base slice wholesale")
	assert.Equal(t, "/c", info.IPK.Alternatives[0].Target)
}
