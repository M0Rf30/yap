package download

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/M0Rf30/yap/v2/pkg/color"
	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
)

const (
	barWidth    = 30
	barFull     = "█"
	barEmpty    = "░"
	updateEvery = 500 * time.Millisecond
)

// ProgressBar renders a single-line in-place progress bar to stderr.
// It uses only stdlib — no external dependencies.
type ProgressBar struct {
	writer      io.Writer
	packageName string
	title       string
	total       int64
	current     int64
	lastPercent int
	startTime   time.Time
	lastUpdate  time.Time
}

// NewProgressBar creates a progress bar whose prefix matches the yap log line
// format so it blends into the surrounding output.
func NewProgressBar(writer io.Writer, packageName, title string, total int64) *ProgressBar {
	pb := &ProgressBar{
		writer:      writer,
		packageName: packageName,
		title:       title,
		total:       total,
		startTime:   time.Now(),
		lastUpdate:  time.Now(),
		lastPercent: -1,
	}

	pb.render(0)

	pb.lastPercent = 0

	return pb
}

// Update sets the current byte count and redraws the bar if enough has changed.
func (pb *ProgressBar) Update(current int64) {
	pb.current = current

	percent := pb.percent(current)
	now := time.Now()

	if percent != pb.lastPercent || now.Sub(pb.lastUpdate) >= updateEvery {
		pb.render(current)

		pb.lastPercent = percent
		pb.lastUpdate = now
	}
}

// Finish marks the bar complete, prints a final newline, and logs completion.
func (pb *ProgressBar) Finish() {
	pb.current = pb.total
	pb.render(pb.total)

	fmt.Fprintln(os.Stderr) //nolint:errcheck // best-effort newline after bar

	duration := time.Since(pb.startTime)

	logger.Info(i18n.T("logger.finish.info.download_completed_1"),
		"title", pb.title,
		"duration", duration)
}

// render writes one bar frame to stderr using \r to overwrite the previous one.
func (pb *ProgressBar) render(current int64) {
	pct := pb.percent(current)
	filled := barWidth * pct / 100
	bar := strings.Repeat(barFull, filled) + strings.Repeat(barEmpty, barWidth-filled)

	elapsed := time.Since(pb.startTime)

	var speed string

	if elapsed.Seconds() > 0 {
		speed = " (" + humanBytes(float64(current)/elapsed.Seconds()) + "/s)"
	}

	prefix := logPrefix(pb.packageName)
	line := fmt.Sprintf("\r%s %s %3d%% [%s]%s [%s]",
		prefix, pb.title, pct, bar, speed, fmtDuration(elapsed))

	fmt.Fprint(os.Stderr, line) //nolint:errcheck // best-effort progress output
}

func (pb *ProgressBar) percent(current int64) int {
	if pb.total <= 0 {
		return 0
	}

	return int(current * 100 / pb.total)
}

// logPrefix returns the colored "timestamp INFO  [packageName]" prefix,
// matching the logger output format so the bar aligns with log lines.
func logPrefix(packageName string) string {
	ts := color.Gray(time.Now().Format("2006-01-02 15:04:05"))

	return ts + " " + color.BoldGreen("INFO ") + " " + color.Bracket(packageName)
}

func fmtDuration(d time.Duration) string {
	d = d.Round(time.Second)

	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60

	if h > 0 {
		return fmt.Sprintf("%dh%02dm%02ds", h, m, s)
	}

	if m > 0 {
		return fmt.Sprintf("%dm%02ds", m, s)
	}

	return fmt.Sprintf("%ds", s)
}

func humanBytes(b float64) string {
	switch {
	case b >= 1<<30:
		return fmt.Sprintf("%.1f GB", b/(1<<30))
	case b >= 1<<20:
		return fmt.Sprintf("%.1f MB", b/(1<<20))
	case b >= 1<<10:
		return fmt.Sprintf("%.1f KB", b/(1<<10))
	default:
		return fmt.Sprintf("%.0f B", b)
	}
}
