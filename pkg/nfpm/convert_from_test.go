package nfpm

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	yaperrors "github.com/M0Rf30/yap/v2/pkg/errors"
	"github.com/M0Rf30/yap/v2/pkg/pkgbuild"
)

// fullFixturePKGBUILD returns a single-package PKGBUILD exercising every row
// of the §4 mapping table FromPKGBUILD inverts.
func fullFixturePKGBUILD() *pkgbuild.PKGBUILD {
	return &pkgbuild.PKGBUILD{
		PkgName:     "myapp",
		PkgBase:     "mybase",
		PkgVer:      "1.2.3~beta.1+deadbeef",
		PkgRel:      "2",
		Epoch:       "1",
		PkgDesc:     "A test application",
		Maintainer:  "Test Maintainer <test@example.com>",
		URL:         "https://example.com/myapp",
		Section:     "utils",
		Priority:    "optional",
		License:     []string{"MIT"},
		Arch:        []string{"x86_64"},
		Depends:     []string{"glibc"},
		OptDepends:  []string{"curl"},
		Suggests:    []string{"bash-completion"},
		Conflicts:   []string{"oldapp"},
		Replaces:    []string{"legacyapp"},
		Provides:    []string{"myapp-bin"},
		Breaks:      []string{"myapp (<< 1.0)"},
		PreDepends:  []string{"dpkg (>= 1.14.0)"},
		Bugs:        "https://example.com/bugs",
		DebTemplate: "debian/templates",
		DebConfig:   "debian/config",
		Group:       "Applications/System",
		Backup:      []string{"etc/myapp/config.yml"},
		BuiltUsing:  []string{"libfoo (= 1.0)", "libbar (= 2.0)"},
		MultiArch:   "same",
		SourcePkg:   "myapp-src",
		PreInst:     "echo pre-install",
		PostInst:    "echo post-install",
		PreRm:       "echo pre-remove",
		PostRm:      "echo post-remove",
		PreTrans:    "echo pre-trans",
		PostTrans:   "echo post-trans",
		PreUpgrade:  "echo pre-upgrade",
		PostUpgrade: "echo post-upgrade",
		BuildDate:   1700000000,
	}
}

func TestFromPKGBUILD_SplitPackageError(t *testing.T) {
	p := &pkgbuild.PKGBUILD{
		PkgBase:  "mybase",
		PkgNames: []string{"mybase", "mybase-dev"},
	}

	cfg, messages, err := FromPKGBUILD(p, FromOptions{})

	require.Error(t, err)
	assert.Nil(t, cfg)
	assert.Nil(t, messages)

	var yapErr *yaperrors.YapError

	require.ErrorAs(t, err, &yapErr)
	assert.Equal(t, yaperrors.ErrTypeValidation, yapErr.Type)
}

func TestFromPKGBUILD_CoreFields(t *testing.T) {
	outDir := t.TempDir()
	p := fullFixturePKGBUILD()

	cfg, messages, err := FromPKGBUILD(p, FromOptions{OutputDir: outDir})
	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.Empty(t, messages)

	assert.Equal(t, "myapp", cfg.Name)
	assert.Equal(t, "linux", cfg.Platform)
	assert.Equal(t, os.FileMode(0o02), cfg.Umask)
	assert.Equal(t, "A test application", cfg.Description)
	assert.Equal(t, "Test Maintainer <test@example.com>", cfg.Maintainer)
	assert.Equal(t, "https://example.com/myapp", cfg.Homepage)
	assert.Equal(t, "utils", cfg.Section)
	assert.Equal(t, "optional", cfg.Priority)
	assert.Equal(t, "2", cfg.Release)
	assert.Equal(t, "1", cfg.Epoch)
	assert.Equal(t, "MIT", cfg.License)
	assert.Equal(t, "amd64", cfg.Arch)
	assert.Equal(t, "1.2.3", cfg.Version)
	assert.Equal(t, "beta.1", cfg.Prerelease)
	assert.Equal(t, "deadbeef", cfg.VersionMetadata)
	assert.Equal(t, time.Unix(1700000000, 0).UTC(), cfg.MTime)

	assert.Equal(t, []string{"glibc"}, cfg.Depends)
	assert.Equal(t, []string{"curl"}, cfg.Recommends)
	assert.Equal(t, []string{"bash-completion"}, cfg.Suggests)
	assert.Equal(t, []string{"oldapp"}, cfg.Conflicts)
	assert.Equal(t, []string{"legacyapp"}, cfg.Replaces)
	assert.Equal(t, []string{"myapp-bin"}, cfg.Provides)

	assert.Equal(t, "Applications/System", cfg.RPM.Group)
	assert.Equal(t, []string{"myapp (<< 1.0)"}, cfg.Deb.Breaks)
	assert.Equal(t, []string{"dpkg (>= 1.14.0)"}, cfg.Deb.Predepends)
	assert.Equal(t, "https://example.com/bugs", cfg.Deb.Fields["Bugs"])
	assert.Equal(t, "libfoo (= 1.0), libbar (= 2.0)", cfg.Deb.Fields["Built-Using"])
	assert.Equal(t, "same", cfg.Deb.Fields["Multi-Arch"])
	assert.Equal(t, "myapp-src", cfg.Deb.Fields["Source"])
	assert.Equal(t, "debian/templates", cfg.Deb.Scripts.Templates)
	assert.Equal(t, "debian/config", cfg.Deb.Scripts.Config)
	assert.Equal(t, "mybase", cfg.ArchLinux.Pkgbase)

	assert.Equal(t, "myapp.preinstall.sh", cfg.Scripts.PreInstall)
	assert.Equal(t, "myapp.postinstall.sh", cfg.Scripts.PostInstall)
	assert.Equal(t, "myapp.preremove.sh", cfg.Scripts.PreRemove)
	assert.Equal(t, "myapp.postremove.sh", cfg.Scripts.PostRemove)
	assert.Equal(t, "myapp.pretrans.sh", cfg.RPM.Scripts.PreTrans)
	assert.Equal(t, "myapp.posttrans.sh", cfg.RPM.Scripts.PostTrans)
	assert.Equal(t, "myapp.preupgrade.sh", cfg.APK.Scripts.PreUpgrade)
	assert.Equal(t, "myapp.postupgrade.sh", cfg.APK.Scripts.PostUpgrade)
	assert.Equal(t, "myapp.preupgrade.sh", cfg.ArchLinux.Scripts.PreUpgrade)
	assert.Equal(t, "myapp.postupgrade.sh", cfg.ArchLinux.Scripts.PostUpgrade)

	for _, name := range []string{
		"myapp.preinstall.sh", "myapp.postinstall.sh", "myapp.preremove.sh", "myapp.postremove.sh",
		"myapp.pretrans.sh", "myapp.posttrans.sh", "myapp.preupgrade.sh", "myapp.postupgrade.sh",
	} {
		data, readErr := os.ReadFile(filepath.Join(outDir, name))
		require.NoError(t, readErr, name)
		assert.True(t, strings.HasPrefix(string(data), "#!/bin/sh\n"), name)
	}

	preInstData, err := os.ReadFile(filepath.Join(outDir, "myapp.preinstall.sh"))
	require.NoError(t, err)
	assert.Equal(t, "#!/bin/sh\necho pre-install\n", string(preInstData))

	require.Len(t, cfg.Contents, 1)
	assert.Equal(t, "/etc/myapp/config.yml", cfg.Contents[0].Destination)
	assert.Equal(t, TypeConfig, cfg.Contents[0].Type)
	assert.Empty(t, cfg.Contents[0].Source)
}

func TestFromPKGBUILD_PackagerScoping(t *testing.T) {
	p := fullFixturePKGBUILD()
	outDir := t.TempDir()

	cfg, _, err := FromPKGBUILD(p, FromOptions{Packager: PackagerRPM, OutputDir: outDir})
	require.NoError(t, err)

	assert.Equal(t, "Applications/System", cfg.RPM.Group)
	assert.NotEmpty(t, cfg.RPM.Scripts.PreTrans)
	assert.NotEmpty(t, cfg.RPM.Scripts.PostTrans)

	assert.Empty(t, cfg.Deb.Breaks)
	assert.Empty(t, cfg.Deb.Predepends)
	assert.Empty(t, cfg.Deb.Fields)
	assert.Empty(t, cfg.Deb.Scripts.Templates)
	assert.Empty(t, cfg.Deb.Scripts.Config)
	assert.Empty(t, cfg.ArchLinux.Pkgbase)
	assert.Empty(t, cfg.APK.Scripts.PreUpgrade)
	assert.Empty(t, cfg.APK.Scripts.PostUpgrade)
	assert.Empty(t, cfg.ArchLinux.Scripts.PreUpgrade)
	assert.Empty(t, cfg.ArchLinux.Scripts.PostUpgrade)

	// Packager-agnostic scripts always populate regardless of scoping.
	assert.NotEmpty(t, cfg.Scripts.PreInstall)
	assert.NotEmpty(t, cfg.Scripts.PostInstall)
}

func TestFromPKGBUILD_ArchMapping(t *testing.T) {
	tests := []struct {
		yapArch  string
		nfpmArch string
	}{
		{"x86_64", "amd64"},
		{"aarch64", "arm64"},
		{"i686", "386"},
		{"armv7", "arm7"},
		{"armv6", "arm6"},
		{"any", "all"},
		{"amd64", "amd64"},
		{"riscv64", "riscv64"},
	}

	for _, tt := range tests {
		p := &pkgbuild.PKGBUILD{
			PkgName: "myapp", PkgVer: "1.0.0", PkgRel: "1", Arch: []string{tt.yapArch},
		}

		cfg, _, err := FromPKGBUILD(p, FromOptions{})
		require.NoError(t, err)
		assert.Equal(t, tt.nfpmArch, cfg.Arch, "yap arch %s", tt.yapArch)
	}
}

func TestFromPKGBUILD_VersionSplitting(t *testing.T) {
	tests := []struct {
		pkgVer         string
		wantVersion    string
		wantPrerelease string
		wantMetadata   string
	}{
		{"1.2.3", "1.2.3", "", ""},
		{"1.2.3~beta.1", "1.2.3", "beta.1", ""},
		{"1.2.3~beta.1+deadbeef", "1.2.3", "beta.1", "deadbeef"},
		{"1.2.3beta.1", "1.2.3beta.1", "", ""},
	}

	for _, tt := range tests {
		p := &pkgbuild.PKGBUILD{PkgName: "myapp", PkgVer: tt.pkgVer, PkgRel: "1"}

		cfg, _, err := FromPKGBUILD(p, FromOptions{})
		require.NoError(t, err)
		assert.Equal(t, tt.wantVersion, cfg.Version, "pkgver %s", tt.pkgVer)
		assert.Equal(t, tt.wantPrerelease, cfg.Prerelease, "pkgver %s", tt.pkgVer)
		assert.Equal(t, tt.wantMetadata, cfg.VersionMetadata, "pkgver %s", tt.pkgVer)
	}
}

func TestFromPKGBUILD_ScriptsNotWritten(t *testing.T) {
	p := &pkgbuild.PKGBUILD{
		PkgName: "myapp",
		PkgVer:  "1.0.0",
		PkgRel:  "1",
		PreInst: "echo hi",
	}

	cfg, messages, err := FromPKGBUILD(p, FromOptions{})
	require.NoError(t, err)
	assert.Equal(t, "myapp.preinstall.sh", cfg.Scripts.PreInstall)

	found := false

	for _, m := range messages {
		if strings.Contains(m, "myapp.preinstall.sh") {
			found = true
		}
	}

	assert.True(t, found, "expected a scripts-not-written message, got %v", messages)
}

func TestFromPKGBUILD_ContentsFromWalk(t *testing.T) {
	root := t.TempDir()

	require.NoError(t, os.MkdirAll(filepath.Join(root, "etc", "myapp"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(root, "etc", "myapp", "config.yml"), []byte("k: v\n"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "usr", "bin"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(root, "usr", "bin", "myapp"), []byte("#!/bin/sh\n"), 0o755))
	require.NoError(t, os.Symlink("myapp", filepath.Join(root, "usr", "bin", "myapp-link")))

	p := &pkgbuild.PKGBUILD{
		PkgName: "myapp",
		PkgVer:  "1.0.0",
		PkgRel:  "1",
		Backup:  []string{"etc/myapp/config.yml"},
	}

	cfg, _, err := FromPKGBUILD(p, FromOptions{ContentsFrom: root})
	require.NoError(t, err)

	byDest := make(map[string]*Content, len(cfg.Contents))
	dests := make([]string, 0, len(cfg.Contents))

	for _, entry := range cfg.Contents {
		dests = append(dests, entry.Destination)
		byDest[entry.Destination] = entry
	}

	assert.True(t, sort.StringsAreSorted(dests), "contents not sorted: %v", dests)

	cfgEntry := byDest["/etc/myapp/config.yml"]
	require.NotNil(t, cfgEntry)
	assert.Equal(t, TypeConfig, cfgEntry.Type)
	assert.Equal(t, filepath.Join(root, "etc", "myapp", "config.yml"), cfgEntry.Source)

	dirEntry := byDest["/etc/myapp"]
	require.NotNil(t, dirEntry)
	assert.Equal(t, TypeDir, dirEntry.Type)
	require.NotNil(t, dirEntry.FileInfo)

	fileEntry := byDest["/usr/bin/myapp"]
	require.NotNil(t, fileEntry)
	assert.Equal(t, TypeFile, fileEntry.Type)
	assert.Equal(t, filepath.Join(root, "usr", "bin", "myapp"), fileEntry.Source)
	require.NotNil(t, fileEntry.FileInfo)
	assert.Equal(t, os.FileMode(0o755), fileEntry.FileInfo.Mode)

	linkEntry := byDest["/usr/bin/myapp-link"]
	require.NotNil(t, linkEntry)
	assert.Equal(t, TypeSymlink, linkEntry.Type)
	assert.Equal(t, "myapp", linkEntry.Source)

	assert.NotNil(t, byDest["/usr/bin"])
	assert.NotNil(t, byDest["/usr"])

	_, hasRoot := byDest["/"]
	assert.False(t, hasRoot)

	for _, entry := range cfg.Contents {
		require.NotNil(t, entry.FileInfo)
		assert.NotEmpty(t, entry.FileInfo.Owner)
		assert.NotEmpty(t, entry.FileInfo.Group)
	}
}

// droppedFeatureCase associates a PKGBUILD mutation with a substring
// expected in FromPKGBUILD's dropped-feature messages when present, absent
// otherwise.
type droppedFeatureCase struct {
	name    string
	mutate  func(*pkgbuild.PKGBUILD)
	wantKey string
}

func droppedFeatureCases() []droppedFeatureCase {
	return []droppedFeatureCase{
		{"prepare", func(p *pkgbuild.PKGBUILD) { p.Prepare = "echo prepare" }, "prepare()"},
		{"build", func(p *pkgbuild.PKGBUILD) { p.Build = "make" }, "build()"},
		{"check", func(p *pkgbuild.PKGBUILD) { p.Check = "make test" }, "check()"},
		{
			"helper_functions",
			func(p *pkgbuild.PKGBUILD) {
				p.HelperFunctions = map[string]string{"_helper": "_helper() {\necho hi\n}"}
			},
			"helper function",
		},
		{
			"sources",
			func(p *pkgbuild.PKGBUILD) { p.SourceURI = []string{"https://example.com/a.tar.gz"} },
			"source",
		},
		{
			"custom_variables",
			func(p *pkgbuild.PKGBUILD) { p.CustomVariables = map[string]string{"FOO": "bar"} },
			"custom variable",
		},
		{
			"custom_arrays",
			func(p *pkgbuild.PKGBUILD) { p.CustomArrays = map[string][]string{"FOOS": {"a", "b"}} },
			"custom array",
		},
		{"makedepends", func(p *pkgbuild.PKGBUILD) { p.MakeDepends = []string{"gcc"} }, "makedepends"},
		{"options", func(p *pkgbuild.PKGBUILD) { p.Options = []string{"!strip"} }, "options"},
		{"install", func(p *pkgbuild.PKGBUILD) { p.Install = "myapp.install" }, "install="},
		{"copyright", func(p *pkgbuild.PKGBUILD) { p.Copyright = []string{"2024, Me"} }, "copyright"},
		{
			"no_extract",
			func(p *pkgbuild.PKGBUILD) { p.NoExtract = []string{"vendor.tar.gz"} },
			"noextract",
		},
		{"enhances", func(p *pkgbuild.PKGBUILD) { p.Enhances = []string{"otherapp"} }, "enhances"},
		{
			"supplements",
			func(p *pkgbuild.PKGBUILD) { p.Supplements = []string{"otherapp"} },
			"supplements",
		},
	}
}

func TestFromPKGBUILD_DroppedFeatures(t *testing.T) {
	base := func() *pkgbuild.PKGBUILD {
		return &pkgbuild.PKGBUILD{PkgName: "myapp", PkgVer: "1.0.0", PkgRel: "1"}
	}

	for _, tt := range droppedFeatureCases() {
		t.Run(tt.name, func(t *testing.T) {
			withFeature := base()
			tt.mutate(withFeature)

			_, messagesWith, err := FromPKGBUILD(withFeature, FromOptions{})
			require.NoError(t, err)

			found := false

			for _, m := range messagesWith {
				if strings.Contains(m, tt.wantKey) {
					found = true
				}
			}

			assert.True(t, found, "expected a message containing %q, got %v", tt.wantKey, messagesWith)

			_, messagesWithout, err := FromPKGBUILD(base(), FromOptions{})
			require.NoError(t, err)
			assert.Empty(t, messagesWithout)
		})
	}
}
