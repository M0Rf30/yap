package builder

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/M0Rf30/yap/v2/pkg/builders/common"
	"github.com/M0Rf30/yap/v2/pkg/constants"
	"github.com/M0Rf30/yap/v2/pkg/pkgbuild"
)

func TestCrossEnvSliceFailurePolicy(t *testing.T) {
	for _, format := range []string{constants.FormatDEB, constants.FormatAPK} {
		p := &pkgbuild.PKGBUILD{PkgName: "x", ArchComputed: "x86_64"}
		bb := &common.BaseBuilder{PKGBUILD: p, Format: format}
		b := &Builder{PKGBUILD: p}

		// Probe whether this combination errors at the toolchain layer.
		p.TargetArch = "bogus-arch"

		_, probeErr := bb.BuildCrossEnvSlice(p.TargetArch)
		if probeErr == nil {
			t.Skipf("%s: bogus arch did not error", format)
		}

		_, err := b.crossEnvSlice(bb)
		if format == constants.FormatAPK {
			require.NoError(t, err)
		} else {
			require.Error(t, err)
		}
	}
}
