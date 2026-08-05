package nfpm

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"mvdan.cc/sh/v3/syntax"

	yaperrors "github.com/M0Rf30/yap/v2/pkg/errors"
	"github.com/M0Rf30/yap/v2/pkg/pkgbuild"
)

const testdataConvertTo = "testdata/convert_to"

// baseFullSpecConfig returns a *Config exercising every mapping-table row
// that does not require a specific packager, rooted at testdataConvertTo.
func baseFullSpecConfig() *Config {
	cfg := &Config{
		Info: Info{
			Overridables: Overridables{
				Replaces:   []string{"oldapp"},
				Provides:   []string{"myapp-bin"},
				Depends:    []string{"libc"},
				Recommends: []string{"curl"},
				Suggests:   []string{"bash-completion"},
				Conflicts:  []string{"otherapp"},
				Contents: Contents{
					{Type: TypeDir, Destination: "/etc/myapp", FileInfo: &ContentFileInfo{Mode: 0o755}},
					{
						Type: TypeFile, Source: "content/bin/myapp", Destination: "/usr/bin/myapp",
						FileInfo: &ContentFileInfo{Mode: 0o755},
					},
					{
						Type: TypeConfig, Source: "content/etc/myapp.conf", Destination: "/etc/myapp/myapp.conf",
						FileInfo: &ContentFileInfo{Mode: 0o644},
					},
				},
				Scripts: Scripts{
					PreInstall:  "scripts/preinstall.sh",
					PostInstall: "scripts/postinstall.sh",
					PreRemove:   "scripts/preremove.sh",
					PostRemove:  "scripts/postremove.sh",
				},
				RPM: RPM{
					Group: "Applications/Internet",
					Scripts: RPMScripts{
						PreTrans:  "scripts/pretrans.sh",
						PostTrans: "scripts/posttrans.sh",
					},
				},
				Deb: Deb{
					Breaks:     []string{"veryoldapp"},
					Predepends: []string{"dpkg"},
					Fields: map[string]string{
						"Bugs":        "https://bugs.example.com",
						"Multi-Arch":  "same",
						"Source":      "myapp-src",
						"Built-Using": "libbar (= 1.0), libbaz (= 2.0)",
					},
				},
			},
			Name:            "myapp",
			Arch:            "amd64",
			Version:         "1.2.3",
			Prerelease:      "beta.1",
			VersionMetadata: "deadbeef",
			Release:         "2",
			Epoch:           "1",
			Section:         "utils",
			Priority:        "optional",
			Maintainer:      "Jane Doe <jane@example.com>",
			Description:     "My App",
			Homepage:        "https://example.com/myapp",
			License:         "MIT",
			Changelog:       "changelog.yaml",
		},
	}

	cfg.SetBaseDir(testdataConvertTo)

	return cfg
}

func TestToPKGBUILD_FullSpec_Deb(t *testing.T) {
	cfg := baseFullSpecConfig()

	pkg, msgs, err := cfg.ToPKGBUILD(&ConvertOptions{
		Packager: PackagerDeb, Distro: "ubuntu", Codename: "jammy",
		StartDir: "/build/myapp", Home: "/home/build", TargetArch: "x86_64",
		ExpandGlobs: true,
	})
	require.NoError(t, err)
	require.NotNil(t, pkg)

	// 1. name -> PkgName, PkgBase
	assert.Equal(t, "myapp", pkg.PkgName)
	assert.Equal(t, "myapp", pkg.PkgBase)

	// 3. version folding (deb bucket)
	assert.Equal(t, "1.2.3~beta.1+deadbeef", pkg.PkgVer)

	// 4/5. release, epoch
	assert.Equal(t, "2", pkg.PkgRel)
	assert.Equal(t, "1", pkg.Epoch)

	// 6. description (rpm.summary row does not apply for deb)
	assert.Equal(t, "My App", pkg.PkgDesc)

	// 8/9/10/11/12
	assert.Equal(t, "utils", pkg.Section)
	assert.Equal(t, "optional", pkg.Priority)
	assert.Equal(t, "Jane Doe <jane@example.com>", pkg.Maintainer)
	assert.Equal(t, "https://example.com/myapp", pkg.URL)
	assert.Equal(t, []string{"MIT"}, pkg.License)

	// 13. arch normalization: amd64 -> x86_64
	assert.Equal(t, []string{"x86_64"}, pkg.Arch)

	// 14-19
	assert.Equal(t, []string{"libc"}, pkg.Depends)
	assert.Equal(t, []string{"curl"}, pkg.OptDepends)
	assert.Equal(t, []string{"bash-completion"}, pkg.Suggests)
	assert.Equal(t, []string{"otherapp"}, pkg.Conflicts)
	assert.Equal(t, []string{"oldapp"}, pkg.Replaces)
	assert.Equal(t, []string{"myapp-bin"}, pkg.Provides)

	// 20/21
	assert.Equal(t, []string{"veryoldapp"}, pkg.Breaks)
	assert.Equal(t, []string{"dpkg"}, pkg.PreDepends)

	// 22-25 deb.fields
	assert.Equal(t, "https://bugs.example.com", pkg.Bugs)
	assert.Equal(t, "same", pkg.MultiArch)
	assert.Equal(t, "myapp-src", pkg.SourcePkg)
	assert.Equal(t, []string{"libbar (= 1.0)", "libbaz (= 2.0)"}, pkg.BuiltUsing)

	// 26. rpm.group -> Group (unconditional on packager per the mapping table)
	assert.Equal(t, "Applications/Internet", pkg.Group)

	// 27. scriptlets: shebang stripped, indented by two spaces
	assert.Equal(t, "  echo \"before install\"\n  systemctl stop myapp || true\n", pkg.PreInst)
	assert.Equal(t, "  systemctl daemon-reload\n  systemctl enable myapp\n", pkg.PostInst)
	assert.Equal(t, "  systemctl stop myapp\n", pkg.PreRm)
	assert.Equal(t, "  systemctl daemon-reload\n", pkg.PostRm)

	// 28. rpm.scripts.pretrans/posttrans -> PreTrans/PostTrans (also unconditional)
	assert.Equal(t, "  echo \"pretrans\"\n", pkg.PreTrans)
	assert.Equal(t, "  echo \"posttrans\"\n", pkg.PostTrans)

	// 29. apk/archlinux upgrade scripts do not apply for deb
	assert.Empty(t, pkg.PreUpgrade)
	assert.Empty(t, pkg.PostUpgrade)

	// 31. contents type config* -> Backup, leading slash stripped
	assert.Equal(t, []string{"etc/myapp/myapp.conf"}, pkg.Backup)

	// 32. changelog -> ChangelogData (deb dialect, codename supplied)
	require.NotEmpty(t, pkg.ChangelogData)
	assert.Contains(t, string(pkg.ChangelogData), "myapp (1.0.1) jammy; urgency=medium")

	// nfpm content is prebuilt: post-processing must be disabled. EmptyDirs is
	// the exception — makepkg's polarity means ENABLED keeps empty directories,
	// and options.Apply only prunes them when it is false, so an nfpm
	// `type: dir` entry survives only while this stays true.
	assert.False(t, pkg.StripEnabled)
	assert.False(t, pkg.ZipManEnabled)
	assert.False(t, pkg.DebugEnabled)
	assert.True(t, pkg.EmptyDirsEnabled)
	assert.Empty(t, pkg.Options)

	// ConvertOptions passthrough
	assert.Equal(t, "ubuntu", pkg.Distro)
	assert.Equal(t, "jammy", pkg.Codename)
	assert.Equal(t, "/build/myapp", pkg.StartDir)
	assert.Equal(t, "/home/build", pkg.Home)
	assert.Equal(t, "x86_64", pkg.TargetArch)

	// no unexpected dropped-feature noise from a spec with none of those
	// fields set.
	assert.Empty(t, msgs)

	require.NotEmpty(t, pkg.Package)
	_, parseErr := syntax.NewParser(syntax.Variant(syntax.LangBash)).
		Parse(strings.NewReader(pkg.Package), "package")
	require.NoError(t, parseErr)
}

// TestToPKGBUILD_PostProcessingDisabledSurvivesInit is a regression test:
// PKGBUILD.Init() calls processOptions(), which unconditionally resets
// StripEnabled/ZipManEnabled/DebugEnabled/EmptyDirsEnabled to makepkg's
// hardcoded defaults (strip=true, emptydirs=true, ...) whenever Options is
// empty. ToPKGBUILD MUST apply its overrides AFTER calling Init(), never
// before — reordering this regresses nfpm-sourced builds back into
// stripping/repacking prebuilt artifacts that must ship byte-identical.
//
// EmptyDirsEnabled is deliberately left TRUE: makepkg's `emptydirs` option
// means "leave empty directories in the package", and pkg/options.Apply only
// calls RemoveEmptyDirs when it is false. A `type: dir` content entry is an
// explicit request for an empty directory, so flipping this to false silently
// deletes those entries after the synthesized package() created them.
func TestToPKGBUILD_PostProcessingDisabledSurvivesInit(t *testing.T) {
	for _, packager := range Packagers {
		if FormatForPackager(packager) == "" {
			continue
		}

		t.Run(packager, func(t *testing.T) {
			pkg, _, err := baseFullSpecConfig().ToPKGBUILD(&ConvertOptions{Packager: packager, ExpandGlobs: true})
			require.NoError(t, err)

			assert.False(t, pkg.StripEnabled)
			assert.False(t, pkg.ZipManEnabled)
			assert.False(t, pkg.DebugEnabled)
			assert.True(t, pkg.EmptyDirsEnabled)
			assert.Empty(t, pkg.Options)
		})
	}
}

func TestToPKGBUILD_RPMSummaryAndGroup(t *testing.T) {
	cfg := baseFullSpecConfig()
	cfg.RPM.Summary = "One-line RPM summary"

	pkg, _, err := cfg.ToPKGBUILD(&ConvertOptions{Packager: PackagerRPM, ExpandGlobs: true})
	require.NoError(t, err)

	// 7. rpm.summary -> PkgDesc when packager==rpm and set
	assert.Equal(t, "One-line RPM summary", pkg.PkgDesc)
	assert.Equal(t, "Applications/Internet", pkg.Group)
}

func TestToPKGBUILD_RPMSummaryIgnoredForOtherPackagers(t *testing.T) {
	cfg := baseFullSpecConfig()
	cfg.RPM.Summary = "One-line RPM summary"

	pkg, _, err := cfg.ToPKGBUILD(&ConvertOptions{Packager: PackagerDeb, ExpandGlobs: true})
	require.NoError(t, err)
	assert.Equal(t, "My App", pkg.PkgDesc)
}

func TestToPKGBUILD_ArchLinuxPkgbase(t *testing.T) {
	cfg := baseFullSpecConfig()
	cfg.ArchLinux.Pkgbase = "myapp-base"

	pkg, _, err := cfg.ToPKGBUILD(&ConvertOptions{Packager: PackagerArchLinux, ExpandGlobs: true})
	require.NoError(t, err)

	// 2. archlinux.pkgbase -> PkgBase (archlinux packager only)
	assert.Equal(t, "myapp-base", pkg.PkgBase)
	assert.Equal(t, "myapp", pkg.PkgName)
}

func TestToPKGBUILD_PkgBaseFallsBackToNameForNonArchLinux(t *testing.T) {
	cfg := baseFullSpecConfig()
	cfg.ArchLinux.Pkgbase = "myapp-base"

	pkg, _, err := cfg.ToPKGBUILD(&ConvertOptions{Packager: PackagerDeb, ExpandGlobs: true})
	require.NoError(t, err)
	assert.Equal(t, "myapp", pkg.PkgBase)
}

func TestToPKGBUILD_UpgradeScripts(t *testing.T) {
	tests := []struct {
		name     string
		packager string
		mutate   func(*Config)
	}{
		{
			name:     "apk",
			packager: PackagerAPK,
			mutate: func(c *Config) {
				c.APK.Scripts.PreUpgrade = "scripts/preupgrade.sh"
				c.APK.Scripts.PostUpgrade = "scripts/postupgrade.sh"
			},
		},
		{
			name:     "archlinux",
			packager: PackagerArchLinux,
			mutate: func(c *Config) {
				c.ArchLinux.Scripts.PreUpgrade = "scripts/preupgrade.sh"
				c.ArchLinux.Scripts.PostUpgrade = "scripts/postupgrade.sh"
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := baseFullSpecConfig()
			tt.mutate(cfg)

			pkg, _, err := cfg.ToPKGBUILD(&ConvertOptions{Packager: tt.packager, ExpandGlobs: true})
			require.NoError(t, err)
			assert.Equal(t, "  echo \"preupgrade\"\n", pkg.PreUpgrade)
			assert.Equal(t, "  echo \"postupgrade\"\n", pkg.PostUpgrade)
		})
	}
}

func TestToPKGBUILD_UpgradeScriptsNotAppliedForOtherPackagers(t *testing.T) {
	cfg := baseFullSpecConfig()
	cfg.APK.Scripts.PreUpgrade = "scripts/preupgrade.sh"
	cfg.ArchLinux.Scripts.PostUpgrade = "scripts/postupgrade.sh"

	pkg, _, err := cfg.ToPKGBUILD(&ConvertOptions{Packager: PackagerDeb, ExpandGlobs: true})
	require.NoError(t, err)
	assert.Empty(t, pkg.PreUpgrade)
	assert.Empty(t, pkg.PostUpgrade)
}

func TestToPKGBUILD_DebScriptsTemplatesAndConfigAreCopiedNotRead(t *testing.T) {
	cfg := baseFullSpecConfig()
	cfg.Deb.Scripts.Templates = "does/not/exist/templates"
	cfg.Deb.Scripts.Config = "does/not/exist/config"

	pkg, _, err := cfg.ToPKGBUILD(&ConvertOptions{Packager: PackagerDeb, ExpandGlobs: true})
	require.NoError(t, err)
	assert.Equal(t, "does/not/exist/templates", pkg.DebTemplate)
	assert.Equal(t, "does/not/exist/config", pkg.DebConfig)
}

func TestToPKGBUILD_Changelog(t *testing.T) {
	t.Run("deb renders entries with per-entry distribution/urgency overrides", func(t *testing.T) {
		cfg := baseFullSpecConfig()

		pkg, _, err := cfg.ToPKGBUILD(&ConvertOptions{Packager: PackagerDeb, ExpandGlobs: true})
		require.NoError(t, err)
		assert.Contains(t, string(pkg.ChangelogData), "myapp (1.0.1) jammy; urgency=medium")
		assert.Contains(t, string(pkg.ChangelogData), "myapp (1.0.0)")
	})

	t.Run("rpm renders the RPM dialect regardless of codename", func(t *testing.T) {
		cfg := baseFullSpecConfig()

		pkg, _, err := cfg.ToPKGBUILD(&ConvertOptions{Packager: PackagerRPM, Codename: "jammy", ExpandGlobs: true})
		require.NoError(t, err)
		assert.Contains(t, string(pkg.ChangelogData), "* Sat Feb 01 2025")
		assert.Contains(t, string(pkg.ChangelogData), "- Fix crash on startup")
	})

	t.Run("no changelog set is a no-op", func(t *testing.T) {
		cfg := baseFullSpecConfig()
		cfg.Changelog = ""

		pkg, _, err := cfg.ToPKGBUILD(&ConvertOptions{Packager: PackagerDeb, ExpandGlobs: true})
		require.NoError(t, err)
		assert.Empty(t, pkg.ChangelogData)
	})

	t.Run("missing changelog file is a hard error", func(t *testing.T) {
		cfg := baseFullSpecConfig()
		cfg.Changelog = "does-not-exist.yaml"

		_, _, err := cfg.ToPKGBUILD(&ConvertOptions{Packager: PackagerDeb, ExpandGlobs: true})
		require.Error(t, err)

		var yerr *yaperrors.YapError
		require.True(t, errors.As(err, &yerr))
		assert.Equal(t, yaperrors.ErrTypeFileSystem, yerr.Type)
	})
}

func TestToPKGBUILD_DebCodenameMissingUsesUnstable(t *testing.T) {
	withCodename, _, err := baseFullSpecConfig().ToPKGBUILD(
		&ConvertOptions{Packager: PackagerDeb, Codename: "focal", ExpandGlobs: true})
	require.NoError(t, err)
	assert.Contains(t, string(withCodename.ChangelogData), "myapp (1.0.0) focal")

	withoutCodename, _, err := baseFullSpecConfig().ToPKGBUILD(
		&ConvertOptions{Packager: PackagerDeb, ExpandGlobs: true})
	require.NoError(t, err)
	assert.Contains(t, string(withoutCodename.ChangelogData), "myapp (1.0.0) unstable")
}

func TestToPKGBUILD_ScriptletMissingFileIsHardError(t *testing.T) {
	cfg := baseFullSpecConfig()
	cfg.Scripts.PreInstall = "scripts/does-not-exist.sh"

	_, _, err := cfg.ToPKGBUILD(&ConvertOptions{Packager: PackagerDeb, ExpandGlobs: true})
	require.Error(t, err)

	var yerr *yaperrors.YapError
	require.True(t, errors.As(err, &yerr))
	assert.Equal(t, yaperrors.ErrTypeFileSystem, yerr.Type)
}

func TestToPKGBUILD_UnsupportedPackager(t *testing.T) {
	for _, packager := range []string{PackagerIPK, "bogus", ""} {
		t.Run(packager, func(t *testing.T) {
			cfg := baseFullSpecConfig()

			_, _, err := cfg.ToPKGBUILD(&ConvertOptions{Packager: packager, ExpandGlobs: true})
			require.Error(t, err)

			var yerr *yaperrors.YapError
			require.True(t, errors.As(err, &yerr))
			assert.Equal(t, yaperrors.ErrTypeValidation, yerr.Type)
		})
	}
}

func TestToPKGBUILD_OverrideResolution(t *testing.T) {
	cfg := &Config{
		Info: Info{
			Name:    "myapp",
			Version: "1.0.0",
			Overridables: Overridables{
				Depends: []string{"base-dep"},
			},
		},
		Overrides: map[string]*Overridables{
			PackagerRPM: {Depends: []string{"rpm-dep"}},
		},
	}

	rpmPkg, _, err := cfg.ToPKGBUILD(&ConvertOptions{Packager: PackagerRPM, ExpandGlobs: true})
	require.NoError(t, err)
	assert.Equal(t, []string{"rpm-dep"}, rpmPkg.Depends)

	debPkg, _, err := cfg.ToPKGBUILD(&ConvertOptions{Packager: PackagerDeb, ExpandGlobs: true})
	require.NoError(t, err)
	assert.Equal(t, []string{"base-dep"}, debPkg.Depends)
}

func TestFoldVersion(t *testing.T) {
	tests := []struct {
		name       string
		packager   string
		version    string
		prerelease string
		metadata   string
		wantVer    string
		wantMsg    bool
	}{
		{
			name: "deb full", packager: PackagerDeb, version: "1.2.3",
			prerelease: "beta.1", metadata: "deadbeef", wantVer: "1.2.3~beta.1+deadbeef",
		},
		{
			name: "deb version only", packager: PackagerDeb, version: "1.2.3",
			wantVer: "1.2.3",
		},
		{
			name: "rpm full strips illegal hyphen", packager: PackagerRPM, version: "1.2.3",
			prerelease: "beta-1", metadata: "deadbeef", wantVer: "1.2.3~beta1+deadbeef",
		},
		{
			name: "apk drops metadata with message", packager: PackagerAPK, version: "1.2.3",
			prerelease: "beta-1", metadata: "deadbeef", wantVer: "1.2.3beta_1", wantMsg: true,
		},
		{
			name: "apk no prerelease no metadata", packager: PackagerAPK, version: "1.2.3",
			wantVer: "1.2.3",
		},
		{
			name: "archlinux drops tilde and plus from a dirty version", packager: PackagerArchLinux,
			version: "1.2.3", prerelease: "rc-1", wantVer: "1.2.3rc_1",
		},
		{
			name: "archlinux with metadata drops it", packager: PackagerArchLinux,
			version: "1.2.3", metadata: "abc", wantVer: "1.2.3", wantMsg: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, msg := FoldVersion(tt.packager, tt.version, tt.prerelease, tt.metadata)
			assert.Equal(t, tt.wantVer, got)

			if tt.wantMsg {
				assert.NotEmpty(t, msg)
			} else {
				assert.Empty(t, msg)
			}
		})
	}
}

func TestSanitizeVersion(t *testing.T) {
	assert.Equal(t, "123abc", sanitizeVersion("1:2:3abc", legalVersionChars(PackagerRPM)))
	assert.Equal(t, "1.2.3-beta", sanitizeVersion("1.2.3-beta", legalVersionChars(PackagerDeb)))
	assert.Equal(t, "1.2.3beta", sanitizeVersion("1.2.3-beta", legalVersionChars(PackagerRPM)))
	assert.Equal(t, "1.2.3betax", sanitizeVersion("1.2.3~beta+x", legalVersionChars(PackagerAPK)))
}

func TestResolveArch(t *testing.T) {
	tests := []struct {
		name     string
		packager string
		info     *Info
		want     string
	}{
		{
			name: "all folds to any", packager: PackagerDeb,
			info: &Info{Arch: "all"}, want: "any",
		},
		{
			name: "packager-specific arch wins over Info.Arch", packager: PackagerRPM,
			info: &Info{Arch: "amd64", Overridables: Overridables{RPM: RPM{Arch: "arm64"}}},
			want: "aarch64",
		},
		{
			name: "falls back to Info.Arch when packager arch unset", packager: PackagerDeb,
			info: &Info{Arch: "arm64"}, want: "aarch64",
		},
		{
			name: "already-canonical arch passes through", packager: PackagerDeb,
			info: &Info{Arch: "x86_64"}, want: "x86_64",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, resolveArch(tt.info, tt.packager))
		})
	}
}

func TestIndentScriptBody(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "shebang and blank lines stripped",
			in:   "#!/bin/sh\n\necho hi\n",
			want: "  echo hi\n",
		},
		{
			name: "no shebang, leading blank lines stripped",
			in:   "\n\necho hi\nls\n",
			want: "  echo hi\n  ls\n",
		},
		{
			name: "no trailing newline still indents every line",
			in:   "#!/bin/sh\necho hi",
			want: "  echo hi\n",
		},
		{
			name: "empty input",
			in:   "",
			want: "",
		},
		{
			name: "shebang only",
			in:   "#!/bin/sh\n",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IndentScriptBody([]byte(tt.in)))
		})
	}
}

func TestReadScriptlet_MissingFileIsHardError(t *testing.T) {
	_, err := readScriptlet(testdataConvertTo, "scripts/does-not-exist.sh")
	require.Error(t, err)

	var yerr *yaperrors.YapError
	require.True(t, errors.As(err, &yerr))
	assert.Equal(t, yaperrors.ErrTypeFileSystem, yerr.Type)
}

func TestReadScriptlet_RelativeAndAbsolute(t *testing.T) {
	body, err := readScriptlet(testdataConvertTo, "scripts/preremove.sh")
	require.NoError(t, err)
	assert.Equal(t, "  systemctl stop myapp\n", body)

	abs, err := filepath.Abs(filepath.Join(testdataConvertTo, "scripts", "preremove.sh"))
	require.NoError(t, err)

	body, err = readScriptlet("/nonexistent-base-dir", abs)
	require.NoError(t, err)
	assert.Equal(t, "  systemctl stop myapp\n", body)
}

func TestMapDebFields(t *testing.T) {
	pkg := &pkgbuild.PKGBUILD{}
	mapDebFields(pkg, map[string]string{
		"Bugs":        "https://bugs.example.com",
		"Multi-Arch":  "same",
		"Source":      "src",
		"Built-Using": "a (= 1), b (= 2)",
		"Unknown":     "ignored-by-mapDebFields",
	})
	assert.Equal(t, "https://bugs.example.com", pkg.Bugs)
	assert.Equal(t, "same", pkg.MultiArch)
	assert.Equal(t, "src", pkg.SourcePkg)
	assert.Equal(t, []string{"a (= 1)", "b (= 2)"}, pkg.BuiltUsing)
}

func TestDroppedFeatureMessages(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Info)
		feature string
	}{
		{featVendor, func(i *Info) { i.Vendor = "Acme" }, featVendor},
		{featRPMBuildHost, func(i *Info) { i.RPM.BuildHost = "builder1" }, featRPMBuildHost},
		{featRPMPackager, func(i *Info) { i.RPM.Packager = "Jane" }, featRPMPackager},
		{featArchLinuxPackager, func(i *Info) { i.ArchLinux.Packager = "Jane" }, featArchLinuxPackager},
		{featRPMPrefixes, func(i *Info) { i.RPM.Prefixes = []string{"/opt"} }, featRPMPrefixes},
		{featRPMRequiresPost, func(i *Info) { i.RPM.Requires.Post = []string{"libc"} }, featRPMRequiresPost},
		{featRPMGhostFiles, func(i *Info) { i.RPM.Ghosts = []string{"/var/run/app.pid"} }, featRPMGhostFiles},
		{featRPMScriptsVerify, func(i *Info) { i.RPM.Scripts.Verify = "scripts/verify.sh" }, featRPMScriptsVerify},
		{featRPMCompression, func(i *Info) { i.RPM.Compression = "zstd" }, featRPMCompression},
		{featRPMSignature, func(i *Info) { i.RPM.Signature.KeyFile = "k.gpg" }, featRPMSignature},
		{featDebCompression, func(i *Info) { i.Deb.Compression = "xz" }, featDebCompression},
		{featDebArchVariant, func(i *Info) { i.Deb.ArchVariant = "v7" }, featDebArchVariant},
		{featDebTriggers, func(i *Info) { i.Deb.Triggers.Interest = []string{"/usr/share/x"} }, featDebTriggers},
		{featDebScriptsRules, func(i *Info) { i.Deb.Scripts.Rules = "debian/rules" }, featDebScriptsRules},
		{featDebSignature, func(i *Info) { i.Deb.Signature.Method = "dpkg-sig" }, featDebSignature},
		{featAPKSignature, func(i *Info) { i.APK.Signature.KeyName = "mykey" }, featAPKSignature},
		{PackagerIPK, func(i *Info) { i.IPK.Essential = true }, PackagerIPK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var info Info

			msgs := droppedFeatureMessages(&info)
			assert.Empty(t, msgs, "baseline (feature absent) must produce no message")

			tt.mutate(&info)
			msgs = droppedFeatureMessages(&info)
			require.Len(t, msgs, 1)
			assert.Contains(t, msgs[0], tt.feature)
		})
	}
}

func TestDroppedFeatureMessages_UnrecognizedDebFieldsAreSortedAndUnique(t *testing.T) {
	info := Info{
		Overridables: Overridables{
			Deb: Deb{
				Fields: map[string]string{
					"Bugs":     "known, not dropped",
					"Z-Custom": "dropped",
					"A-Custom": "dropped",
				},
			},
		},
	}

	msgs := droppedFeatureMessages(&info)
	require.Len(t, msgs, 2)
	assert.Contains(t, msgs[0], "deb.fields[A-Custom]")
	assert.Contains(t, msgs[1], "deb.fields[Z-Custom]")
}

func TestCollectBackup(t *testing.T) {
	contents := Contents{
		{Type: TypeFile, Destination: "/usr/bin/app"},
		{Type: TypeConfig, Destination: "/etc/app.conf"},
		{Type: TypeConfigNoReplace, Destination: "/etc/app2.conf"},
		{Type: TypeConfigTree, Destination: "/etc/app.d"},
	}

	assert.Equal(t, []string{"etc/app.conf", "etc/app2.conf", "etc/app.d"}, collectBackup(contents))
}

func TestSplitCommaTrim(t *testing.T) {
	assert.Equal(t, []string{"a", "b (= 1)"}, splitCommaTrim(" a , b (= 1) , "))
	assert.Empty(t, splitCommaTrim(""))
}
