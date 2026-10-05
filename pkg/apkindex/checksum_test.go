package apkindex_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha1" //nolint:gosec
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/M0Rf30/yap/v2/pkg/apkindex"
)

func gzTar(t *testing.T, name, body string) []byte {
	t.Helper()

	var buf bytes.Buffer

	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	require.NoError(t, tw.WriteHeader(&tar.Header{Name: name, Size: int64(len(body)), Mode: 0o644}))
	_, err := tw.Write([]byte(body))
	require.NoError(t, err)
	require.NoError(t, tw.Close())
	require.NoError(t, gw.Close())

	return buf.Bytes()
}

func TestControlChecksumVerification(t *testing.T) {
	sig := gzTar(t, ".SIGN.RSA.x.rsa.pub", "sig")
	ctrl := gzTar(t, ".PKGINFO", "pkgname = a\n")
	data := gzTar(t, "usr/bin/a", "x")
	sum := sha1.Sum(ctrl) //nolint:gosec
	good := "Q1" + base64.StdEncoding.EncodeToString(sum[:])

	for name, apk := range map[string][]byte{
		"signed":   bytes.Join([][]byte{sig, ctrl, data}, nil),
		"unsigned": bytes.Join([][]byte{ctrl, data}, nil),
	} {
		t.Run(name, func(t *testing.T) {
			info, err := apkindex.ExportReadControlVerified(
				bytes.NewReader(apk), &apkindex.Package{Name: "a", Checksum: good})
			require.NoError(t, err)
			assert.Contains(t, info, "pkgname = a")

			_, err = apkindex.ExportReadControlVerified(
				bytes.NewReader(apk), &apkindex.Package{Name: "a", Checksum: "Q1AAAA"})
			require.Error(t, err)

			_, err = apkindex.ExportReadControlVerified(
				bytes.NewReader(apk), &apkindex.Package{Name: "a"})
			require.Error(t, err)
		})
	}
}
