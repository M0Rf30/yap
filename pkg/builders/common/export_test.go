// export_test.go exposes unexported helpers for white-box unit tests.
// This file is only compiled when running tests.
package common

import "github.com/M0Rf30/yap/v2/pkg/aptcache"

// IsPerlModule exposes isPerlModule for unit tests.
var IsPerlModule = isPerlModule

// PartitionArchAllDeps exposes partitionArchAllDeps for unit tests.
var PartitionArchAllDeps = partitionArchAllDeps

// PartitionArchAllDepsForExtract exposes partitionArchAllDepsForExtract for unit tests.
var PartitionArchAllDepsForExtract = partitionArchAllDepsForExtract

// CountDirect exposes countDirect for unit tests.
var CountDirect = countDirect

// MakeTestCache builds a minimal aptcache.Cache from a slice of PackageInfo
// entries and installs it as the global singleton so that partitionArchAllDeps
// and partitionArchAllDepsForExtract see it during tests.
func MakeTestCache(entries []aptcache.PackageInfo) {
	c := aptcache.NewEmptyCache()
	for _, e := range entries {
		c.AddEntry(e)
	}

	aptcache.StoreGlobal(c)
}
