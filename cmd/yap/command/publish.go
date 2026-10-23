// SPDX-FileCopyrightText: 2021-2026 Gianluca Boiano
// SPDX-License-Identifier: GPL-3.0-only

package command

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/M0Rf30/yap/v2/pkg/errors"
	"github.com/M0Rf30/yap/v2/pkg/httpclient"
	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
	"github.com/M0Rf30/yap/v2/pkg/publish"
)

const (
	commandPublish = "publish"

	publishBackoffBase = 500 * time.Millisecond
	publishDefaultTry  = 3
)

// publishFlags holds the values of the publish command flags.
type publishFlags struct {
	config       string
	url          string
	tokenFile    string
	debRepo      string
	rpmRepo      string
	apkRepo      string
	pacmanRepo   string
	distribution string
	component    string
	apkPath      string
	pacmanPath   string
	insecureHTTP bool
	dryRun       bool
	retries      int
	timeout      time.Duration
}

var pubFlags publishFlags

// publishCmd uploads built packages to an OpenArtifact repository.
var publishCmd = &cobra.Command{
	Use:     commandPublish + " [artifact-or-dir]...",
	GroupID: commandUtility,
	Short:   "", // Set by InitializeLocalizedDescriptions
	Long:    "", // Set by InitializeLocalizedDescriptions
	Example: "", // Set by InitializeLocalizedDescriptions
	Args:    cobra.ArbitraryArgs,
	RunE:    runPublish,
}

// resolvePublishConfig merges yap.json < YAP_OA_* environment < flags.
func resolvePublishConfig(cmd *cobra.Command) (publish.Config, string, error) {
	var (
		cfg    publish.Config
		output string
	)

	projectFile := pubFlags.config
	if projectFile == "" {
		if _, err := os.Stat(publish.ProjectFileName); err == nil {
			projectFile = publish.ProjectFileName
		}
	}

	if projectFile != "" {
		var err error

		cfg, output, err = publish.LoadProjectFile(projectFile)
		if err != nil {
			return cfg, "", err
		}
	}

	if err := cfg.ApplyEnv(os.Getenv); err != nil {
		return cfg, "", err
	}

	applyPublishFlags(cmd, &cfg)

	if cmd.Flags().Changed("token-file") {
		if err := cfg.LoadTokenFile(pubFlags.tokenFile); err != nil {
			return cfg, "", err
		}
	}

	return cfg, output, nil
}

// applyPublishFlags overrides cfg with every flag the user passed explicitly.
func applyPublishFlags(cmd *cobra.Command, cfg *publish.Config) {
	str := map[string]struct {
		flag string
		dst  *string
	}{
		"url":          {pubFlags.url, &cfg.URL},
		"deb-repo":     {pubFlags.debRepo, &cfg.DebRepo},
		"rpm-repo":     {pubFlags.rpmRepo, &cfg.RPMRepo},
		"apk-repo":     {pubFlags.apkRepo, &cfg.APKRepo},
		"pacman-repo":  {pubFlags.pacmanRepo, &cfg.PacmanRepo},
		"distribution": {pubFlags.distribution, &cfg.Distribution},
		"component":    {pubFlags.component, &cfg.Component},
		"apk-path":     {pubFlags.apkPath, &cfg.APKPath},
		"pacman-path":  {pubFlags.pacmanPath, &cfg.PacmanPath},
	}

	for name, f := range str {
		if cmd.Flags().Changed(name) {
			*f.dst = f.flag
		}
	}

	cfg.AllowInsecureHTTP = pubFlags.insecureHTTP
	cfg.Timeout = pubFlags.timeout
}

func runPublish(cmd *cobra.Command, args []string) error {
	cfg, output, err := resolvePublishConfig(cmd)
	if err != nil {
		return err
	}

	if len(args) == 0 {
		if output == "" {
			return errors.New(errors.ErrTypeValidation, i18n.T("errors.publish.no_artifacts")).
				WithOperation("runPublish")
		}

		args = []string{output}
	}

	files, fromDir, err := publish.CollectArtifacts(args)
	if err != nil {
		return err
	}

	if len(files) == 0 {
		return errors.New(errors.ErrTypeValidation, i18n.T("errors.publish.no_packages_found")).
			WithOperation("runPublish").
			WithContext("paths", fmt.Sprint(args))
	}

	if pubFlags.dryRun {
		return dryRunPublish(&cfg, files, fromDir)
	}

	httpclient.SetRetryPolicy(pubFlags.retries, publishBackoffBase)

	pub, err := publish.New(&cfg)
	if err != nil {
		return err
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	return publishAll(ctx, pub, &cfg, files, fromDir)
}

// dryRunPublish lists what would be uploaded without contacting the server.
func dryRunPublish(cfg *publish.Config, files []string, fromDir map[string]bool) error {
	for _, f := range files {
		format, _ := publish.DetectFormat(f)
		repo := cfg.RepoFor(format)

		if repo == "" {
			if !fromDir[f] {
				return errors.Wrap(publish.ErrNoRepository, errors.ErrTypeConfiguration, format).
					WithOperation("dryRunPublish").
					WithContext("file", filepath.Base(f))
			}

			logger.Warn(i18n.T("logger.publish.warn.skipped_no_repo"), "file", filepath.Base(f), "format", format)

			continue
		}

		logger.Info(i18n.T("logger.publish.info.dry_run"), "file", filepath.Base(f), "format", format, "repo", repo)
	}

	return nil
}

func publishAll(
	ctx context.Context, pub *publish.Publisher, cfg *publish.Config, files []string, fromDir map[string]bool,
) error {
	var failed, uploaded, unchanged int

	for _, f := range files {
		format, _ := publish.DetectFormat(f)

		if cfg.RepoFor(format) == "" && fromDir[f] {
			logger.Warn(i18n.T("logger.publish.warn.skipped_no_repo"), "file", filepath.Base(f), "format", format)

			continue
		}

		results, err := pub.Publish(ctx, f)
		for _, r := range results {
			if r.Status == publish.StatusUploaded {
				uploaded++
			} else {
				unchanged++
			}
		}

		if err != nil {
			failed++

			logger.Error(i18n.T("logger.publish.error.failed"), "file", filepath.Base(f), "error", err)
		}
	}

	logger.Info(i18n.T("logger.publish.info.summary"),
		"uploaded", uploaded, "unchanged", unchanged, "failed", failed)

	if failed > 0 {
		return errors.New(errors.ErrTypeNetwork, i18n.T("errors.publish.failed")).
			WithOperation("publishAll").
			WithContext("failed", failed)
	}

	return nil
}

// InitializePublishDescriptions sets the localized descriptions for publish.
func InitializePublishDescriptions() {
	initCommandDescriptions(publishCmd, commandPublish, map[string]string{
		"config":        "flags.publish.config",
		"url":           "flags.publish.url",
		"token-file":    "flags.publish.token_file",
		"deb-repo":      "flags.publish.deb_repo",
		"rpm-repo":      "flags.publish.rpm_repo",
		"apk-repo":      "flags.publish.apk_repo",
		"pacman-repo":   "flags.publish.pacman_repo",
		"distribution":  "flags.publish.distribution",
		"component":     "flags.publish.component",
		"apk-path":      "flags.publish.apk_path",
		"pacman-path":   "flags.publish.pacman_path",
		"insecure-http": "flags.publish.insecure_http",
		"dry-run":       "flags.publish.dry_run",
		"retries":       "flags.publish.retries",
		"timeout":       "flags.publish.timeout",
	})
}

//nolint:gochecknoinits // Required for cobra command registration
func init() {
	f := publishCmd.Flags()
	f.StringVar(&pubFlags.config, "config", "", "")
	f.StringVar(&pubFlags.url, "url", "", "")
	f.StringVar(&pubFlags.tokenFile, "token-file", "", "")
	f.StringVar(&pubFlags.debRepo, "deb-repo", "", "")
	f.StringVar(&pubFlags.rpmRepo, "rpm-repo", "", "")
	f.StringVar(&pubFlags.apkRepo, "apk-repo", "", "")
	f.StringVar(&pubFlags.pacmanRepo, "pacman-repo", "", "")
	f.StringVar(&pubFlags.distribution, "distribution", "", "")
	f.StringVar(&pubFlags.component, "component", "", "")
	f.StringVar(&pubFlags.apkPath, "apk-path", "", "")
	f.StringVar(&pubFlags.pacmanPath, "pacman-path", "", "")
	f.BoolVar(&pubFlags.insecureHTTP, "insecure-http", false, "")
	f.BoolVar(&pubFlags.dryRun, "dry-run", false, "")
	f.IntVar(&pubFlags.retries, "retries", publishDefaultTry, "")
	f.DurationVar(&pubFlags.timeout, "timeout", publish.DefaultTimeout, "")

	rootCmd.AddCommand(publishCmd)
}
