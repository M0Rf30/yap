// SPDX-FileCopyrightText: 2021-2026 Gianluca Boiano
// SPDX-License-Identifier: GPL-3.0-only

package rpm

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	rpmutils "github.com/M0Rf30/go-rpmutils"
	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/stretchr/testify/require"

	"github.com/M0Rf30/yap/v2/pkg/constants"
)

// buildSignedRPM builds the test package with the given format and an
// embedded signature from a throwaway OpenPGP key, returning the artifact
// path and the key's public keyring.
func buildSignedRPM(t *testing.T, format int) (string, openpgp.EntityList) {
	t.Helper()

	entity, err := openpgp.NewEntity("YAP Test", "", "test@example.com", nil)
	require.NoError(t, err)

	tempDir := t.TempDir()
	packageDir := filepath.Join(tempDir, "package")
	require.NoError(t, os.MkdirAll(filepath.Join(packageDir, "usr", "share", "t"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(packageDir, "usr", "share", "t", "a.txt"), []byte("hi\n"), 0o644))

	pkgBuild := createTestPKGBUILD()
	pkgBuild.PackageDir = packageDir

	r := NewBuilder(pkgBuild, "", format)
	r.SetPGPSigner(func(data []byte) ([]byte, error) {
		var sig bytes.Buffer

		err := openpgp.DetachSign(&sig, entity, bytes.NewReader(data), nil)

		return sig.Bytes(), err
	})

	artifactsDir := filepath.Join(tempDir, "artifacts")
	require.NoError(t, os.MkdirAll(artifactsDir, 0o755))

	path, err := r.BuildPackage(context.Background(), artifactsDir, "")
	require.NoError(t, err)

	return path, openpgp.EntityList{entity}
}

// TestBuildPackageFormatAndEmbeddedSignature checks that the selected RPM
// format reaches the artifact and that the embedded signature verifies with
// the same reader dnfinstall uses (v4: header + header+payload signatures,
// v6: OPENPGP header signature).
func TestBuildPackageFormatAndEmbeddedSignature(t *testing.T) {
	for _, tc := range []struct {
		format   int
		wantSigs int
	}{
		{constants.RPMFormatV4, 2},
		{constants.RPMFormatV6, 1},
	} {
		path, keyring := buildSignedRPM(t, tc.format)

		f, err := os.Open(path)
		require.NoError(t, err)

		hdr, sigs, err := rpmutils.Verify(f, keyring)
		require.NoError(t, err, "format %d", tc.format)
		require.Equal(t, tc.format, hdr.Format())
		require.Len(t, sigs, tc.wantSigs, "format %d", tc.format)
		require.NoError(t, f.Close())

		// Payload must extract with the declared content.
		f, err = os.Open(path)
		require.NoError(t, err)

		rpm, err := rpmutils.ReadRpm(f)
		require.NoError(t, err)

		pr, err := rpm.PayloadReaderExtended()
		require.NoError(t, err)

		var got []byte

		for {
			fi, err := pr.Next()
			if errors.Is(err, io.EOF) {
				break
			}

			require.NoError(t, err)

			if fi.Name() == "/usr/share/t/a.txt" {
				got, err = io.ReadAll(pr)
				require.NoError(t, err)
			}
		}

		require.Equal(t, "hi\n", string(got), "format %d", tc.format)
		require.NoError(t, f.Close())
	}
}

// TestBuildPackageV6WrongKey checks that a signed v6 package fails
// verification against an unrelated key (no silent pass).
func TestBuildPackageV6WrongKey(t *testing.T) {
	path, _ := buildSignedRPM(t, constants.RPMFormatV6)

	other, err := openpgp.NewEntity("Other", "", "other@example.com", nil)
	require.NoError(t, err)

	f, err := os.Open(path)
	require.NoError(t, err)

	defer func() { _ = f.Close() }()

	_, _, err = rpmutils.Verify(f, openpgp.EntityList{other})

	var knf rpmutils.KeyNotFoundError
	require.ErrorAs(t, err, &knf)
}
