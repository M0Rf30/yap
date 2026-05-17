package download

import (
	"bytes"
	"testing"
	"time"
)

func TestNewProgressBar(t *testing.T) {
	var buf bytes.Buffer

	packageName := "test-package"
	title := "downloading"
	total := int64(1000)

	pb := NewProgressBar(&buf, packageName, title, total)

	if pb == nil {
		t.Fatal("NewProgressBar returned nil")
	}

	if pb.writer != &buf {
		t.Error("Writer not set correctly")
	}

	if pb.packageName != packageName {
		t.Errorf("PackageName = %q, want %q", pb.packageName, packageName)
	}

	if pb.title != title {
		t.Errorf("Title = %q, want %q", pb.title, title)
	}

	if pb.total != total {
		t.Errorf("Total = %d, want %d", pb.total, total)
	}

	if pb.current != 0 {
		t.Errorf("Current = %d, want 0", pb.current)
	}

	if pb.lastPercent != 0 {
		t.Errorf("LastPercent = %d, want 0 (after initial render)", pb.lastPercent)
	}

	if pb.startTime.IsZero() {
		t.Error("StartTime should be set")
	}

	if pb.lastUpdate.IsZero() {
		t.Error("LastUpdate should be set")
	}
}

func TestProgressBarUpdate(t *testing.T) {
	var buf bytes.Buffer

	pb := NewProgressBar(&buf, "test-pkg", "downloading", 1000)

	pb.Update(250)

	if pb.current != 250 {
		t.Errorf("Current = %d, want 250", pb.current)
	}
}

func TestProgressBarUpdateThrottling(t *testing.T) {
	var buf bytes.Buffer

	pb := NewProgressBar(&buf, "test-pkg", "downloading", 1000)

	pb.Update(100)
	pb.Update(200)

	if pb.current != 200 {
		t.Errorf("Current = %d, want 200", pb.current)
	}
}

func TestProgressBarFinish(t *testing.T) {
	var buf bytes.Buffer

	pb := NewProgressBar(&buf, "test-pkg", "downloading", 1000)

	pb.Update(500)
	pb.Finish()

	if pb.current != pb.total {
		t.Errorf("After Finish, current = %d, want %d", pb.current, pb.total)
	}
}

func TestProgressBarMultipleUpdates(t *testing.T) {
	var buf bytes.Buffer

	pb := NewProgressBar(&buf, "test-pkg", "testing", 1000)

	for _, update := range []int64{100, 200, 300, 400, 500} {
		pb.Update(update)
	}

	if pb.current != 500 {
		t.Errorf("Final current = %d, want 500", pb.current)
	}
}

func TestHumanBytes(t *testing.T) {
	cases := []struct {
		input float64
		want  string
	}{
		{500, "500 B"},
		{1500, "1.5 KB"},
		{1_500_000, "1.4 MB"},
		{1_500_000_000, "1.4 GB"},
	}

	for _, tc := range cases {
		got := humanBytes(tc.input)
		if got != tc.want {
			t.Errorf("humanBytes(%v) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestFmtDuration(t *testing.T) {
	cases := []struct {
		input time.Duration
		want  string
	}{
		{5 * time.Second, "5s"},
		{90 * time.Second, "1m30s"},
		{3700 * time.Second, "1h01m40s"},
	}

	for _, tc := range cases {
		got := fmtDuration(tc.input)
		if got != tc.want {
			t.Errorf("fmtDuration(%v) = %q, want %q", tc.input, got, tc.want)
		}
	}
}
