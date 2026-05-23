// Package project provides multi-package project management and build orchestration.
package project

import (
	yerrors "github.com/M0Rf30/yap/v2/pkg/errors"
	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
)

// getProjectsInRange returns the subset of projects based on --from and --to flags.
// Returns all projects if no flags are set, or only projects in the specified range.
func (mpc *MultipleProject) getProjectsInRange() []*Project {
	// If no filtering is specified, return all projects
	if mpc.Opts.FromPkgName == "" && mpc.Opts.ToPkgName == "" {
		return mpc.Projects
	}

	var filtered []*Project

	startProcessing := (mpc.Opts.FromPkgName == "")

	for _, proj := range mpc.Projects {
		pkgName := proj.Builder.PKGBUILD.PkgName

		// Check if this is the FromPkgName - start processing from here
		if mpc.Opts.FromPkgName != "" && !startProcessing && pkgName == mpc.Opts.FromPkgName {
			startProcessing = true

			logger.Debug(i18n.T("logger.project.found_from_package"),
				"package", mpc.Opts.FromPkgName)
		}

		// If we should be processing, include this package
		if startProcessing {
			filtered = append(filtered, proj)

			// Check if this is the ToPkgName - stop after this package
			if mpc.Opts.ToPkgName != "" && pkgName == mpc.Opts.ToPkgName {
				logger.Debug(i18n.T("logger.project.found_to_package"),
					"package", mpc.Opts.ToPkgName)

				break
			}
		}
	}

	if len(filtered) > 0 {
		var pkgNames []string
		for _, proj := range filtered {
			pkgNames = append(pkgNames, proj.Builder.PKGBUILD.PkgName)
		}

		logger.Info(i18n.T("logger.project.filtering_projects"),
			"total", len(mpc.Projects),
			"filtered", len(filtered),
			"packages", pkgNames)
	}

	return filtered
}

// checkPkgsRange checks the range of packages from `fromPkgName` to `toPkgName`.
//
// It takes two parameters:
// - fromPkgName: string representing the name of the starting package.
// - toPkgName: string representing the name of the ending package.
func (mpc *MultipleProject) checkPkgsRange(fromPkgName, toPkgName string) error {
	var firstIndex, lastIndex int

	if fromPkgName != "" {
		idx, err := mpc.findPackageInProjects(fromPkgName)
		if err != nil {
			return err
		}

		firstIndex = idx
	}

	if toPkgName != "" {
		idx, err := mpc.findPackageInProjects(toPkgName)
		if err != nil {
			return err
		}

		lastIndex = idx
	}

	if fromPkgName != "" && toPkgName != "" && firstIndex > lastIndex {
		return yerrors.New(yerrors.ErrTypeInternal, i18n.T("logger.invalid_package_order")).
			WithOperation("checkPkgsRange").
			WithContext("required_first", fromPkgName).
			WithContext("required_second", toPkgName)
	}

	return nil
}

// findPackageInProjects finds a package in the MultipleProject struct.
//
// pkgName: the name of the package to find.
// Returns the index of the package if found, and an error if not found.
func (mpc *MultipleProject) findPackageInProjects(pkgName string) (int, error) {
	for i, proj := range mpc.Projects {
		if pkgName == proj.Builder.PKGBUILD.PkgName {
			return i, nil
		}
	}

	return -1, yerrors.New(yerrors.ErrTypeInternal, "package not found in projects").
		WithOperation("findPackageInProjects").
		WithContext("package", pkgName)
}
