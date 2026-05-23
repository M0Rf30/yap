package dnfinstall

import (
	rpmutils "github.com/sassoftware/go-rpmutils"

	"github.com/M0Rf30/yap/v2/pkg/yapdb"
)

// extractCapabilities extracts Provides, Requires, Conflicts, and Obsoletes
// from an RPM header and returns them as a slice of yapdb.Capability.
func extractCapabilities(rpm *rpmutils.Rpm) []yapdb.Capability {
	var caps []yapdb.Capability

	caps = append(caps, readCapList(rpm, "provide",
		rpmutils.PROVIDENAME, rpmutils.PROVIDEFLAGS, rpmutils.PROVIDEVERSION)...)
	caps = append(caps, readCapList(rpm, "require",
		rpmutils.REQUIRENAME, rpmutils.REQUIREFLAGS, rpmutils.REQUIREVERSION)...)
	caps = append(caps, readCapList(rpm, "conflict",
		rpmutils.CONFLICTNAME, rpmutils.CONFLICTFLAGS, rpmutils.CONFLICTVERSION)...)
	caps = append(caps, readCapList(rpm, "obsolete",
		rpmutils.OBSOLETENAME, rpmutils.OBSOLETEFLAGS, rpmutils.OBSOLETEVERSION)...)

	return caps
}

// readCapList reads a capability list (names, flags, versions) from the RPM header
// and returns them as a slice of yapdb.Capability with the given kind.
// The three arrays are expected to be parallel (same length or shorter versions/flags).
func readCapList(rpm *rpmutils.Rpm, kind string, nameTag, flagsTag, versionTag int) []yapdb.Capability {
	names, _ := rpm.Header.GetStrings(nameTag)
	if len(names) == 0 {
		return nil
	}

	flags, _ := rpm.Header.GetUint32s(flagsTag)
	versions, _ := rpm.Header.GetStrings(versionTag)

	out := make([]yapdb.Capability, len(names))
	for i := range names {
		var (
			fl  int
			ver string
		)

		if i < len(flags) {
			fl = int(flags[i])
		}

		if i < len(versions) {
			ver = versions[i]
		}

		out[i] = yapdb.Capability{
			Kind:    kind,
			Name:    names[i],
			Flags:   fl,
			Version: ver,
		}
	}

	return out
}
