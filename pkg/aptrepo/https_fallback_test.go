package aptrepo_test

import (
	"context"
	"net"
	"net/url"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/M0Rf30/yap/v2/pkg/aptrepo"
	"github.com/M0Rf30/yap/v2/pkg/httpclient"
)

// resetError satisfies net.Error so httpclient.IsRetryable classifies it as
// transient — matching the real "connection reset by peer" seen against
// http://archive.ubuntu.com/ and http://security.ubuntu.com/ on networks
// that block/interfere with plain port-80 traffic while leaving 443 alone.
type resetError struct{}

func (resetError) Error() string   { return "connection reset by peer" }
func (resetError) Timeout() bool   { return false }
func (resetError) Temporary() bool { return true }

// TestHTTPFetch_FallsBackToHTTPSOnResetError reproduces the exact
// user-reported path: an InRelease/Packages.xz fetch against a plain-http
// mirror URL fails with a reset connection, and the fetch must recover by
// escalating to https instead of failing the apt source update.
func TestHTTPFetch_FallsBackToHTTPSOnResetError(t *testing.T) {
	var httpCalls, httpsCalls atomic.Int32

	const payload = "Suite: jammy\n"

	fetcher := func(_ context.Context, fetchURL string) ([]byte, error) {
		u, err := url.Parse(fetchURL)
		require.NoError(t, err)

		if u.Scheme == "http" {
			httpCalls.Add(1)
			return nil, &net.OpError{Op: "read", Net: "tcp", Err: resetError{}}
		}

		httpsCalls.Add(1)

		return []byte(payload), nil
	}

	data, err := aptrepo.HTTPFetchWithFetcherForTesting(
		context.Background(),
		"http://archive.ubuntu.com/ubuntu/dists/jammy/InRelease",
		fetcher,
	)

	require.NoError(t, err, "a reset http connection must be recovered via the https escalation")
	assert.Equal(t, payload, string(data))
	assert.Positive(t, httpCalls.Load(), "the declared http URL must be tried first")
	assert.Equal(t, int32(1), httpsCalls.Load(),
		"exactly one https attempt must follow the http failure")
}

// TestHTTPFetch_NoFallbackOnDefiniteFailure verifies a non-retryable
// failure (a real httpclient.HTTPStatusError 404: no such file on the
// mirror) is surfaced as-is, without wasting a request on an https retry
// that cannot fix a definitive answer.
func TestHTTPFetch_NoFallbackOnDefiniteFailure(t *testing.T) {
	var httpCalls, httpsCalls atomic.Int32

	fetcher := func(_ context.Context, fetchURL string) ([]byte, error) {
		u, err := url.Parse(fetchURL)
		require.NoError(t, err)

		if u.Scheme == "http" {
			httpCalls.Add(1)
			return nil, &httpclient.HTTPStatusError{Code: 404, URL: fetchURL}
		}

		httpsCalls.Add(1)

		return nil, nil
	}

	_, err := aptrepo.HTTPFetchWithFetcherForTesting(
		context.Background(),
		"http://archive.ubuntu.com/ubuntu/dists/jammy/InRelease",
		fetcher,
	)

	require.Error(t, err)
	assert.Positive(t, httpCalls.Load())
	assert.Zero(t, httpsCalls.Load(), "a definitive 404 must not trigger an https retry")
}

// TestHTTPFetch_ReturnsOriginalErrorWhenHTTPSAlsoFails verifies that when
// both the declared http URL and the escalated https URL fail, the caller
// still sees a descriptive error (not a silent swallow of the https
// diagnostic) rather than losing why the recovery attempt didn't help.
func TestHTTPFetch_ReturnsOriginalErrorWhenHTTPSAlsoFails(t *testing.T) {
	var httpCalls, httpsCalls atomic.Int32

	fetcher := func(_ context.Context, fetchURL string) ([]byte, error) {
		u, err := url.Parse(fetchURL)
		require.NoError(t, err)

		if u.Scheme == "http" {
			httpCalls.Add(1)
			return nil, &net.OpError{Op: "read", Net: "tcp", Err: resetError{}}
		}

		httpsCalls.Add(1)

		return nil, &httpclient.HTTPStatusError{Code: 503, URL: fetchURL}
	}

	_, err := aptrepo.HTTPFetchWithFetcherForTesting(
		context.Background(),
		"http://archive.ubuntu.com/ubuntu/dists/jammy/InRelease",
		fetcher,
	)

	require.Error(t, err)
	assert.Positive(t, httpCalls.Load())
	assert.Positive(t, httpsCalls.Load(), "the https escalation must still be attempted")
	assert.Contains(t, err.Error(), "https fallback also failed",
		"the error must record that https was tried and also failed, not swallow the diagnostic")
	assert.Contains(t, err.Error(), "503",
		"the https failure's own status code must be visible for diagnosis")
}
