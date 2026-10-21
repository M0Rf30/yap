// SPDX-FileCopyrightText: 2021-2026 Gianluca Boiano
// SPDX-License-Identifier: GPL-3.0-only

package shell

import (
	"bytes"
	"context"
	"runtime"
	"testing"
)

func TestExecCaptureEnvPassesEnv(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires sh")
	}

	var buf bytes.Buffer

	err := ExecCaptureEnv(context.Background(), []string{"YAP_TEST_FOO=bar"}, &buf, "",
		"sh", "-c", `printf %s "$YAP_TEST_FOO"`)
	if err != nil {
		t.Fatalf("ExecCaptureEnv: %v", err)
	}

	if buf.String() != "bar" {
		t.Errorf("got %q, want %q", buf.String(), "bar")
	}
}
