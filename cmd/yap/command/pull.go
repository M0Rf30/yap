// SPDX-FileCopyrightText: 2021-2026 Gianluca Boiano
// SPDX-License-Identifier: GPL-3.0-only

package command

import (
	"errors"

	"github.com/spf13/cobra"

	"github.com/M0Rf30/yap/v2/pkg/constants"
	"github.com/M0Rf30/yap/v2/pkg/container"
	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
)

const (
	alpineDistro = "alpine"
	archDistro   = "arch"
)

// isRollingDistro reports whether a distro family publishes a single,
// unqualified builder image (rolling release) so no codename is required.
func isRollingDistro(distro string) bool {
	return distro == alpineDistro || distro == archDistro ||
		distro == constants.DistroOpenSUSETumbleweed
}

// pullCmd represents the pull command.
var pullCmd = &cobra.Command{
	Use:     commandPull + " <distro>",
	GroupID: commandEnvironment,
	Aliases: []string{"download"},
	Short:   "", // Set by InitializeLocalizedDescriptions
	Long:    "", // Set by InitializeLocalizedDescriptions
	Example: "", // Set by InitializeLocalizedDescriptions
	Args:    createValidateDistroArgs(1),
	PreRun:  PreRunValidation,
	RunE: func(_ *cobra.Command, args []string) error {
		distro, release := parseDistroAndRelease(args[0])

		if release == "" && !isRollingDistro(distro) {
			return errors.New(i18n.T("logger.pull.specify_codename"))
		}

		rt, err := container.Detect(ContainerRuntimeOverride())
		if err != nil {
			return err
		}

		logger.Info(i18n.T("logger.command.info.using_container_runtime"), "type", string(rt.Type()))

		if err := rt.Pull(args[0]); err != nil {
			return err
		}

		return nil
	},
}

//nolint:gochecknoinits // Required for cobra command registration
func init() {
	rootCmd.AddCommand(pullCmd)

	// Add completion for distribution argument
	pullCmd.ValidArgsFunction = ValidDistrosCompletion
}
