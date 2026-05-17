// Package command implements the YAP CLI commands including build, install, graph, and utility operations.
package command

import (
	"errors"
	"strings"

	"github.com/spf13/cobra"

	"github.com/M0Rf30/yap/v2/pkg/builders/common"
	yapErrors "github.com/M0Rf30/yap/v2/pkg/errors"
	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
	"github.com/M0Rf30/yap/v2/pkg/parser"
	"github.com/M0Rf30/yap/v2/pkg/project"
	"github.com/M0Rf30/yap/v2/pkg/shell"
	"github.com/M0Rf30/yap/v2/pkg/signing"
	"github.com/M0Rf30/yap/v2/pkg/source"
)

// sshPassword is the local holder for the --ssh-password flag value.
var sshPassword string

// compressionDeb is the local holder for the --compression-deb flag value.
var compressionDeb string

// compressionRpm is the local holder for the --compression-rpm flag value.
var compressionRpm string

// signKey is the local holder for the --sign-key flag value.
var signKey string

// signPassphrase is the local holder for the --sign-passphrase flag value.
var signPassphrase string

// signKeyName is the local holder for the --sign-key-name flag value.
var signKeyName string

// sign is the local holder for the --sign flag value.
var sign bool

// buildOpts holds all build configuration options from CLI flags.
var buildOpts project.BuildOptions

// buildCmd represents the command to build the entire project.
var buildCmd = &cobra.Command{
	Use:     buildCommand + " [distro] <path>",
	GroupID: buildGroup,
	Aliases: []string{"b"},
	Short:   "Build packages from yap.json project definition", // Will be set in init()
	Long:    "",                                                // Will be set in init()
	Example: "",                                                // Will be set in init()
	Args:    cobra.RangeArgs(1, 2),                             // Allow 1-2 arguments
	PreRun:  PreRunValidation,
	RunE: func(_ *cobra.Command, args []string) error {
		// Propagate ssh-password flag value to the source package.
		source.SetSSHPassword(sshPassword)

		// Set the skip toolchain validation flag
		common.SkipToolchainValidation = buildOpts.SkipToolchainValidation

		// Set verbose flag from global flag
		shell.SetVerbose(verbose)

		// Enhanced user feedback with progress
		if verbose {
			logger.Debug(i18n.T("logger.build.starting_verbose"))
		}

		// Parse flexible arguments using shared function
		distro, release, fullJSONPath, err := ParseFlexibleArgs(args)
		if err != nil {
			return err
		}

		// Auto-detect distro and codename from /etc/os-release when missing.
		userProvidedDistro := distro != ""
		distro, release = ResolveDistroRelease(distro, release,
			"logger.build.no_distribution_specified")

		if userProvidedDistro {
			logArgs := []any{"distro", distro}
			if release != "" {
				logArgs = append(logArgs, "release", release)
			}

			logArgs = append(logArgs, "path", fullJSONPath)
			logger.Info(i18n.T("logger.build.building_for_distribution"), logArgs...)
		}

		// Initialize MultipleProject and attach build options
		buildOpts.Verbose = verbose
		mpc := project.MultipleProject{Opts: buildOpts}

		// Apply CLI compression flags BEFORE MultiProject() so populateProjects() picks them up
		if compressionDeb != "" {
			if err := validateCompression(compressionDeb); err != nil {
				return err
			}

			mpc.CompressionDeb = compressionDeb
		}

		if compressionRpm != "" {
			if err := validateCompression(compressionRpm); err != nil {
				return err
			}

			mpc.CompressionRpm = compressionRpm
		}

		err = mpc.MultiProject(distro, release, fullJSONPath)
		if err != nil {
			var yapErr *yapErrors.YapError
			if errors.As(err, &yapErr) {
				logStructuredError(yapErr)
			} else {
				logger.Fatal(i18n.T("logger.build.project_init_failed"), "error", err)
			}

			return err
		}

		// Apply CLI signing flags if provided
		if sign {
			// Resolve signing configuration from CLI flags, env vars, and project config
			signingCfg, err := resolveSigning(&mpc)
			if err != nil {
				return err
			}

			// Propagate signing config to all projects
			for _, proj := range mpc.Projects {
				proj.Signing = signingCfg
			}
		}

		logger.Info(i18n.T("logger.build.project_init_success"))

		// Build packages with timestamp logging
		logger.Info(i18n.T("logger.build.building_packages"))

		err = mpc.BuildAll()
		if err != nil {
			var yapErr *yapErrors.YapError
			if errors.As(err, &yapErr) {
				logStructuredError(yapErr)
			} else {
				logger.Fatal(i18n.T("logger.build.build_failed"), "error", err)
			}

			return err
		}

		logger.Info(i18n.T("logger.build.build_completed"))

		return nil
	},
}

// validateCompression validates that the compression algorithm is supported.
func validateCompression(compression string) error {
	switch compression {
	case "zstd", "gzip", "xz":
		return nil
	default:
		return yapErrors.New(
			yapErrors.ErrTypeConfiguration,
			"unsupported compression algorithm",
		).WithContext("compression", compression).
			WithOperation("validateCompression")
	}
}

// resolveSigning resolves the signing configuration from CLI flags,
// environment variables, and project config using the full priority chain.
func resolveSigning(mpc *project.MultipleProject) (*signing.Config, error) {
	// Get config from project if available
	configKey := ""
	configPass := ""

	if mpc.Signing != nil {
		configKey = mpc.Signing.KeyPath
		configPass = mpc.Signing.Passphrase
	}

	// Use signing.ResolveGeneric() for project-level resolution.
	// This skips format-specific env vars (YAP_DEB_KEY, YAP_APK_KEY, etc.)
	// since the actual artifact format is not yet known.
	// Format-specific resolution happens in signArtifact() for each artifact.
	resolved, err := signing.ResolveGeneric(signKey,
		signPassphrase, configKey, configPass)
	if err != nil {
		return nil, err
	}

	// Apply KeyName from CLI flag (not handled by Resolve)
	keyName := signKeyName
	if keyName == "" && mpc.Signing != nil {
		keyName = mpc.Signing.KeyName
	}

	return &signing.Config{
		Enabled:    sign,
		KeyPath:    resolved.KeyPath,
		Passphrase: resolved.Passphrase,
		KeyName:    keyName,
	}, nil
}

// logStructuredError logs a concise fatal build error.
func logStructuredError(yapErr *yapErrors.YapError) {
	pkg, _ := yapErr.Context["package"].(string)
	ver, _ := yapErr.Context["version"].(string)
	rel, _ := yapErr.Context["release"].(string)
	stage, _ := yapErr.Context["stage"].(string)

	parts := []string{i18n.T("logger.build.build_failed")}

	if pkg != "" {
		coord := pkg
		if ver != "" {
			coord += " " + ver
			if rel != "" {
				coord += "-" + rel
			}
		}

		parts = append(parts, coord)
	}

	if stage != "" {
		parts = append(parts, "(stage: "+stage+")")
	} else if yapErr.Operation != "" {
		parts = append(parts, "("+yapErr.Operation+")")
	}

	msg := strings.Join(parts, ": ")

	if yapErr.Cause != nil {
		msg += " — " + yapErr.Cause.Error()
	}

	logger.Fatal(msg)
}

// InitializeBuildDescriptions sets the localized descriptions for the build command.
// This must be called after i18n is initialized.
func InitializeBuildDescriptions() {
	// #nosec G101 -- map keys are CLI flag names, not credentials
	initCommandDescriptions(buildCmd, "build", map[string]string{
		"cleanbuild":                "flags.build.cleanbuild",
		"nobuild":                   "flags.build.nobuild",
		commandZap:                  "flags.build.zap",
		"nomakedeps":                "flags.build.nomakedeps",
		flagSkipSync:                "flags.build.skip_sync",
		"skip-toolchain-validation": "flags.build.skip_toolchain_validation",
		"parallel":                  "flags.build.parallel",
		"pkgver":                    "flags.build.pkgver",
		"pkgrel":                    "flags.build.pkgrel",
		"ssh-password":              "flags.build.ssh_password",
		flagFrom:                    "flags.build.from",
		"to":                        "flags.build.to",
		"only":                      "flags.build.only",
		"target-arch":               "flags.build.target_arch",
		"sbom":                      "flags.build.sbom",
		"sbom-format":               "flags.build.sbom_format",
		"compression-deb":           "flags.build.compression_deb",
		"compression-rpm":           "flags.build.compression_rpm",
		"repo":                      "flags.build.repo",
		"debug-dir":                 "flags.build.debug_dir",
		"sign":                      "flags.build.sign",
		"sign-key":                  "flags.build.sign_key",
		"sign-passphrase":           "flags.build.sign_passphrase",
		"sign-key-name":             "flags.build.sign_key_name",
	})
}

//nolint:gochecknoinits // Required for cobra command registration
func init() {
	// Command descriptions will be set later via InitializeLocalizedDescriptions()
	rootCmd.AddCommand(buildCmd)

	// Add completion for command arguments
	buildCmd.ValidArgsFunction = func(
		cmd *cobra.Command,
		args []string,
		toComplete string,
	) ([]string, cobra.ShellCompDirective) {
		switch len(args) {
		case 0:
			// First arg: distribution or path
			if strings.Contains(toComplete, "/") || toComplete == "." {
				return nil, cobra.ShellCompDirectiveFilterDirs
			}

			return ValidDistrosCompletion(cmd, args, toComplete)
		case 1:
			// Second arg: path (if first was distro)
			return nil, cobra.ShellCompDirectiveFilterDirs
		default:
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
	}

	// BUILD BEHAVIOR FLAGS
	buildCmd.Flags().BoolVarP(&buildOpts.CleanBuild,
		"cleanbuild", "c", false, "")
	buildCmd.Flags().BoolVarP(&buildOpts.NoBuild,
		"nobuild", "o", false, "")
	buildCmd.Flags().BoolVarP(&buildOpts.Zap,
		"zap", "z", false, "")

	// DEPENDENCY MANAGEMENT FLAGS
	buildCmd.Flags().BoolVarP(&buildOpts.NoMakeDeps,
		"nomakedeps", "d", false, "")
	buildCmd.Flags().BoolVarP(&buildOpts.SkipSyncDeps,
		flagSkipSync, "s", false, "")
	buildCmd.Flags().BoolVarP(&buildOpts.SkipToolchainValidation,
		"skip-toolchain-validation", "", false, "")
	buildCmd.Flags().BoolVarP(&buildOpts.Parallel,
		"parallel", "P", false, "")

	// VERSION CONTROL FLAGS
	buildCmd.PersistentFlags().StringVarP(&parser.OverridePkgVer,
		"pkgver", "w", "", "")
	buildCmd.PersistentFlags().StringVarP(&parser.OverridePkgRel,
		"pkgrel", "r", "", "")

	// SOURCE ACCESS FLAGS
	buildCmd.Flags().StringVarP(&sshPassword,
		"ssh-password", "p", "", "")

	// BUILD RANGE CONTROL FLAGS
	buildCmd.Flags().StringVarP(&buildOpts.FromPkgName,
		flagFrom, "", "", "")
	buildCmd.Flags().StringVarP(&buildOpts.ToPkgName,
		"to", "", "", "")

	// PROJECT FILTER FLAGS
	buildCmd.Flags().StringVarP(&buildOpts.OnlyPkgNames,
		"only", "", "", "Comma-separated list of project names to build (filters yap.json)")

	// CROSS-COMPILATION FLAGS
	buildCmd.Flags().StringVarP(&buildOpts.TargetArch,
		"target-arch", "t", "", "Target architecture for cross-compilation (e.g., arm64, armv7, x86_64)")

	// DEBUG SYMBOL FLAGS
	buildCmd.Flags().StringVarP(&buildOpts.DebugDir,
		"debug-dir", "", "", "")

	// SBOM GENERATION FLAGS
	buildCmd.Flags().BoolVarP(&buildOpts.SBOM,
		"sbom", "", false, "")
	buildCmd.Flags().StringVarP(&buildOpts.SBOMFormat,
		"sbom-format", "", "both", "")

	// COMPRESSION FLAGS
	buildCmd.Flags().StringVarP(&compressionDeb,
		"compression-deb", "", "zstd", "")
	buildCmd.Flags().StringVarP(&compressionRpm,
		"compression-rpm", "", "zstd", "")

	// SIGNING FLAGS
	buildCmd.Flags().BoolVarP(&sign,
		"sign", "", false, "")
	buildCmd.Flags().StringVarP(&signKey,
		"sign-key", "", "", "")
	buildCmd.Flags().StringVarP(&signPassphrase,
		"sign-passphrase", "", "", "")
	buildCmd.Flags().StringVarP(&signKeyName,
		"sign-key-name", "", "", "")

	// EXTRA REPOSITORY FLAGS
	// StringArrayVar (not StringSliceVar) so commas inside the spec are not
	// treated as multi-value separators — every --repo invocation maps to one
	// repository definition.
	buildCmd.Flags().StringArrayVar(&buildOpts.ExtraRepos,
		"repo", nil, "")
}
