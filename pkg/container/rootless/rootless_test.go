//go:build linux

//nolint:testpackage // exercises unexported helpers
package rootless

import (
	"os"
	"reflect"
	"testing"
)

func TestEncodeDecodeArgsKeepsEmptyAndSpecial(t *testing.T) {
	cases := [][]string{
		{"/bin/sh", "-c", ""},
		{"cmd", "", "--flag", "", "x"},
		{"a\x1fb", "line1\nline2", " "},
		{},
	}

	for _, want := range cases {
		got, err := decodeArgs(encodeArgs(want))
		if err != nil {
			t.Fatalf("decodeArgs(%q): %v", want, err)
		}

		if len(want) == 0 && len(got) == 0 {
			continue
		}

		if !reflect.DeepEqual(got, want) {
			t.Errorf("round trip mismatch: got %q, want %q", got, want)
		}
	}
}

func TestDecodeArgsInvalid(t *testing.T) {
	if _, err := decodeArgs("not json"); err == nil {
		t.Error("expected error for invalid payload")
	}

	got, err := decodeArgs("")
	if err != nil || got != nil {
		t.Errorf("empty payload: got %v, %v; want nil, nil", got, err)
	}
}

func TestSetEnvOnceRestores(t *testing.T) {
	t.Setenv("YAP_TEST_PRESET", "orig")

	restore := setEnvOnce(map[string]string{
		"YAP_TEST_PRESET": "changed",
		"YAP_TEST_UNSET":  "v",
	})

	if os.Getenv("YAP_TEST_PRESET") != "changed" || os.Getenv("YAP_TEST_UNSET") != "v" {
		t.Fatal("setEnvOnce did not apply values")
	}

	restore()

	if got := os.Getenv("YAP_TEST_PRESET"); got != "orig" {
		t.Errorf("preset not restored: %q", got)
	}

	if _, ok := os.LookupEnv("YAP_TEST_UNSET"); ok {
		t.Error("previously unset variable was not removed")
	}
}
