package aptcache_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/cavaliergopher/grab/v3"
	"github.com/stretchr/testify/require"

	"github.com/M0Rf30/yap/v2/pkg/aptcache"
)

// resetError satisfies net.Error so httpclient.IsRetryable (and
// download.IsRetryableGrabError, which defers to it) classifies it as
// transient — matching a real "connection reset by peer" on port 80.
type resetError struct{}

func (resetError) Error() string   { return "connection reset by peer" }
func (resetError) Timeout() bool   { return false }
func (resetError) Temporary() bool { return true }

// schemeRoundTripper simulates a network that resets plain-http
// connections outright while serving https normally, without any real
// TCP I/O. It lets the test drive grab's real state machine (HEAD, GET,
// checksum) end to end.
type schemeRoundTripper struct {
	body       []byte
	httpCalls  atomic.Int32
	httpsCalls atomic.Int32
}

func (rt *schemeRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Scheme == "http" {
		rt.httpCalls.Add(1)
		return nil, &net.OpError{Op: "read", Net: "tcp", Err: resetError{}}
	}

	rt.httpsCalls.Add(1)

	body := io.NopCloser(bytes.NewReader(rt.body))
	if req.Method == http.MethodHead {
		body = http.NoBody
	}

	return &http.Response{
		StatusCode:    http.StatusOK,
		ContentLength: int64(len(rt.body)),
		Header:        http.Header{},
		Body:          body,
		Request:       req,
	}, nil
}

// TestDownloadFallsBackToHTTPSOnResetError reproduces the real-world
// complaint: a mirror declared with a plain-http sources.list URL whose
// port-80 connections get reset (corporate egress proxy, sandboxed CI
// network), while https to the exact same host succeeds. Download must
// recover automatically instead of failing the build over the scheme.
func TestDownloadFallsBackToHTTPSOnResetError(t *testing.T) {
	t.Parallel()

	const debBody = "package payload\n"

	sum := sha256.Sum256([]byte(debBody))
	hashHex := hex.EncodeToString(sum[:])

	stanza := fmt.Sprintf(`Package: widget
Architecture: amd64
Version: 1.0
Filename: pool/main/w/widget/widget_1.0_amd64.deb
Size: %d
SHA256: %s
Description: test widget

`, len(debBody), hashHex)

	c := aptcache.NewCacheForTesting()
	require.NoError(t, c.ParseDeb822WithBaseURLForTesting(
		strings.NewReader(stanza), false, "http://archive.example.test/ubuntu/"))

	rt := &schemeRoundTripper{body: []byte(debBody)}
	client := grab.NewClient()
	client.HTTPClient = &http.Client{Transport: rt}

	destDir := t.TempDir()

	err := c.DownloadWithClientForTesting(context.Background(), client, destDir, []string{"widget"})
	require.NoError(t, err, "download must recover via the https fallback")

	require.Positive(t, rt.httpCalls.Load(), "the declared http URL must be tried first")
	require.Positive(t, rt.httpsCalls.Load(), "a reset http connection must trigger an https retry")

	got, readErr := os.ReadFile(filepath.Join(destDir, "widget_1.0_amd64.deb"))
	require.NoError(t, readErr)
	require.Equal(t, debBody, string(got), "the artifact fetched over https must be the real payload")
}

// TestDownloadNoFallbackOnDefiniteFailure verifies a non-retryable failure
// (404: package genuinely missing from the mirror) is surfaced as-is,
// without wasting a request on an https retry that cannot fix a 404.
func TestDownloadNoFallbackOnDefiniteFailure(t *testing.T) {
	t.Parallel()

	const debBody = "irrelevant\n"

	sum := sha256.Sum256([]byte(debBody))
	hashHex := hex.EncodeToString(sum[:])

	stanza := fmt.Sprintf(`Package: widget
Architecture: amd64
Version: 1.0
Filename: pool/main/w/widget/widget_1.0_amd64.deb
Size: %d
SHA256: %s
Description: test widget

`, len(debBody), hashHex)

	c := aptcache.NewCacheForTesting()
	require.NoError(t, c.ParseDeb822WithBaseURLForTesting(
		strings.NewReader(stanza), false, "http://archive.example.test/ubuntu/"))

	rt := &notFoundRoundTripper{}
	client := grab.NewClient()
	client.HTTPClient = &http.Client{Transport: rt}

	destDir := t.TempDir()

	err := c.DownloadWithClientForTesting(context.Background(), client, destDir, []string{"widget"})
	require.Error(t, err)
	require.Zero(t, rt.httpsCalls.Load(), "a definitive 404 must not trigger an https retry")
}

type notFoundRoundTripper struct {
	httpsCalls atomic.Int32
}

func (rt *notFoundRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Scheme == "https" {
		rt.httpsCalls.Add(1)
	}

	return &http.Response{
		StatusCode: http.StatusNotFound,
		Header:     http.Header{},
		Body:       http.NoBody,
		Request:    req,
	}, nil
}
