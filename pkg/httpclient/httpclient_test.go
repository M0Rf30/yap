package httpclient_test

import (
	stderrors "errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/M0Rf30/yap/v2/pkg/httpclient"
)

func TestHTTPStatusError_ErrorString(t *testing.T) {
	e := &httpclient.HTTPStatusError{Code: 404, URL: "https://example.test/repo"}
	got := e.Error()

	if !strings.Contains(got, "404") || !strings.Contains(got, "https://example.test/repo") {
		t.Fatalf("Error() = %q, want code + URL", got)
	}
}

func TestHTTPStatusError_IsClientError(t *testing.T) {
	cases := []struct {
		code int
		want bool
	}{
		{399, false},
		{400, true},
		{404, true},
		{499, true},
		{500, false},
		{503, false},
	}

	for _, tc := range cases {
		e := &httpclient.HTTPStatusError{Code: tc.code}
		if got := e.IsClientError(); got != tc.want {
			t.Errorf("IsClientError(%d) = %v, want %v", tc.code, got, tc.want)
		}
	}
}

func TestCheckStatus_2xxReturnsNil(t *testing.T) {
	for _, code := range []int{200, 201, 204, 299} {
		resp := &http.Response{StatusCode: code}
		if err := httpclient.CheckStatus(resp, "https://example.test"); err != nil {
			t.Errorf("httpclient.CheckStatus(%d) returned %v, want nil", code, err)
		}
	}
}

func TestCheckStatus_NonSuccessReturnsTypedError(t *testing.T) {
	resp := &http.Response{StatusCode: 503}

	err := httpclient.CheckStatus(resp, "https://example.test/key")
	if err == nil {
		t.Fatal("httpclient.CheckStatus(503) returned nil, want error")
	}

	var statusErr *httpclient.HTTPStatusError
	if !stderrors.As(err, &statusErr) {
		t.Fatalf("CheckStatus error is not *httpclient.HTTPStatusError: %T", err)
	}

	if statusErr.Code != 503 {
		t.Errorf("Code = %d, want 503", statusErr.Code)
	}

	if statusErr.URL != "https://example.test/key" {
		t.Errorf("URL = %q, want %q", statusErr.URL, "https://example.test/key")
	}
}

func TestLimitedBody_CapsAtDefault(t *testing.T) {
	resp := &http.Response{Body: io.NopCloser(strings.NewReader("hello"))}

	r := httpclient.LimitedBody(resp)

	got, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	if string(got) != "hello" {
		t.Errorf("body = %q, want %q", got, "hello")
	}
}

func TestLimitedBodyN_RespectsTighterCap(t *testing.T) {
	resp := &http.Response{Body: io.NopCloser(strings.NewReader("abcdefghij"))}

	got, err := io.ReadAll(httpclient.LimitedBodyN(resp, 4))
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	if string(got) != "abcd" {
		t.Errorf("body = %q, want %q", got, "abcd")
	}
}

func TestLimitedBodyN_NonPositiveFallsBackToDefault(t *testing.T) {
	resp := &http.Response{Body: io.NopCloser(strings.NewReader("xyz"))}

	got, err := io.ReadAll(httpclient.LimitedBodyN(resp, 0))
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	if string(got) != "xyz" {
		t.Errorf("body = %q, want %q", got, "xyz")
	}
}

func TestClient_ReturnsSameInstance(t *testing.T) {
	a := httpclient.Client()
	b := httpclient.Client()

	if a != b {
		t.Error("httpclient.Client() should return the same shared instance across calls")
	}

	if a.Timeout != httpclient.DefaultTimeout {
		t.Errorf("Client timeout = %v, want %v", a.Timeout, httpclient.DefaultTimeout)
	}
}
