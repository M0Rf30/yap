// SPDX-FileCopyrightText: 2021-2026 Gianluca Boiano
// SPDX-License-Identifier: GPL-3.0-only

package platform

import "testing"

func TestGoChecksumURL(t *testing.T) {
	got := goChecksumURL("https://go.dev/dl/go1.27.0.linux-amd64.tar.gz")
	want := "https://dl.google.com/go/go1.27.0.linux-amd64.tar.gz.sha256"

	if got != want {
		t.Fatalf("goChecksumURL() = %q, want %q", got, want)
	}
}
