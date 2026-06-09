// export_test.go exposes unexported helpers for white-box unit tests.
// This file is only compiled when running tests.
package common

import "github.com/M0Rf30/yap/v2/pkg/aptcache"

// IsPerlModule exposes isPerlModule for unit tests.
var IsPerlModule = isPerlModule

// PartitionArchAllDeps exposes partitionArchAllDeps (install mode) for unit tests.
func PartitionArchAllDeps(deps []string) (archSpecific, archAll []string) {
	return partitionArchAllDeps(deps, partitionForInstall)
}

// PartitionArchAllDepsForExtract exposes partitionArchAllDeps (extract mode) for unit tests.
func PartitionArchAllDepsForExtract(deps []string) (archSpecific, archAll []string) {
	return partitionArchAllDeps(deps, partitionForExtract)
}

// CountDirect exposes countDirect for unit tests.
var CountDirect = countDirect

// MakeTestCache builds a minimal aptcache.Cache from a slice of PackageInfo
// entries and installs it as the global singleton so that partitionArchAllDeps
// and partitionArchAllDepsForExtract see it during tests.
func MakeTestCache(entries []aptcache.PackageInfo) {
	c := aptcache.NewEmptyCache()
	for i := range entries {
		c.AddEntry(&entries[i])
	}

	aptcache.StoreGlobal(c)
}
