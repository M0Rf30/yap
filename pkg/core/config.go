// Package core provides package manager configurations and metadata.
package core

import "github.com/M0Rf30/yap/v2/pkg/constants"

// Config holds common configuration for all package managers.
type Config struct {
}

// PackageManagerConfigs holds all package manager configurations.
var PackageManagerConfigs = map[string]*Config{
	constants.FormatAPK:    {},
	constants.PMApt:        {},
	constants.FormatPacman: {},
	constants.PMYum:        {},
	constants.PMZypper:     {},
}

// GetConfig returns the configuration for a given package manager.
func GetConfig(packageManager string) *Config {
	return PackageManagerConfigs[packageManager]
}
