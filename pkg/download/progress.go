package download

import (
	"fmt"
	"io"
	"time"

	"github.com/pterm/pterm"

	"github.com/M0Rf30/yap/v2/pkg/logger"
)

// ProgressBar provides a pterm native progress bar implementation.
type ProgressBar struct {
	writer      io.Writer
	packageName string
	title       string
	total       int64
	current     int64
	lastPercent int
	startTime   time.Time
	lastUpdate  time.Time
	ptermBar    *pterm.ProgressbarPrinter
}

// NewProgressBar creates a new pterm native progress bar with log-style format.
func NewProgressBar(writer io.Writer, packageName, title string,
	total int64) *ProgressBar {
	// Create pterm progress bar with log-style format including timestamp and INFO level
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	logTitle := fmt.Sprintf("%s INFO [%s] %s", timestamp, packageName, title)

	ptermBar := pterm.DefaultProgressbar.
		WithTitle(logTitle).
		WithTotal(int(total)).
		WithShowElapsedTime(true).
		WithShowCount(false).
		WithShowPercentage(true).
		WithBarStyle(&pterm.Style{pterm.FgLightBlue}).
		WithTitleStyle(&pterm.Style{pterm.FgLightBlue}).
		WithBarCharacter("█").
		WithLastCharacter("█").
		WithElapsedTimeRoundingFactor(time.Millisecond).
		WithBarFiller("░")

	// Start the progress bar
	ptermBar, _ = ptermBar.Start()

	return &ProgressBar{
		writer:      writer,
		packageName: packageName,
		title:       title,
		total:       total,
		startTime:   time.Now(),
		lastUpdate:  time.Now(),
		lastPercent: -1,
		ptermBar:    ptermBar,
	}
}

// Update updates the progress bar with new current value.
func (epb *ProgressBar) Update(current int64) {
	epb.current = current
	percent := int((current * 100) / epb.total)

	// Only update if progress changed by at least 1% or if it's been more than 500ms
	now := time.Now()
	if percent != epb.lastPercent || now.Sub(epb.lastUpdate) > 500*time.Millisecond {
		if epb.ptermBar != nil {
			// Calculate analytical information
			duration := time.Since(epb.startTime)

			var speed string

			if duration.Seconds() > 0 {
				bytesPerSec := float64(epb.current) / duration.Seconds()
				speed = formatBytes(int64(bytesPerSec)) + "/s"
			}

			// Update title with log-style format including timestamp and INFO level
			timestamp := time.Now().Format("2006-01-02 15:04:05")
			currentSize := formatBytes(epb.current)
			totalSize := formatBytes(epb.total)

			logTitle := fmt.Sprintf("%s INFO [%s] %s • %s/%s • %s • ETA: %s",
				timestamp,
				epb.packageName,
				epb.title,
				currentSize,
				totalSize,
				speed,
				epb.calculateETA(duration, current),
			)

			// Update the progress bar title and current value
			epb.ptermBar.UpdateTitle(logTitle)
			epb.ptermBar.Current = int(current)
		}

		epb.lastPercent = percent
		epb.lastUpdate = now
	}
}

// calculateETA calculates estimated time to arrival
func (epb *ProgressBar) calculateETA(elapsed time.Duration, current int64) string {
	if current == 0 || elapsed.Seconds() == 0 {
		return "calculating..."
	}

	bytesPerSec := float64(current) / elapsed.Seconds()
	remaining := epb.total - current
	etaSeconds := float64(remaining) / bytesPerSec

	eta := time.Duration(etaSeconds) * time.Second
	if eta > time.Hour {
		return fmt.Sprintf("%.1fh", eta.Hours())
	} else if eta > time.Minute {
		return fmt.Sprintf("%.1fm", eta.Minutes())
	}

	return fmt.Sprintf("%.1fs", eta.Seconds())
}

// Finish completes the progress bar.
func (epb *ProgressBar) Finish() {
	epb.current = epb.total

	if epb.ptermBar != nil {
		// Final update with completion info in log-style format
		timestamp := time.Now().Format("2006-01-02 15:04:05")
		duration := time.Since(epb.startTime)
		finalSize := formatBytes(epb.total)
		avgSpeed := formatBytes(int64(float64(epb.total)/duration.Seconds())) + "/s"

		completionTitle := fmt.Sprintf("%s INFO [%s] %s • %s • %s • Completed in %v",
			timestamp,
			epb.packageName,
			epb.title,
			finalSize,
			avgSpeed,
			duration.Round(time.Millisecond),
		)

		// Update to final state and stop
		epb.ptermBar.UpdateTitle(completionTitle)
		epb.ptermBar.Current = int(epb.total)
		_, _ = epb.ptermBar.Stop()
	}

	// Log final completion message using logger.Info for consistency
	duration := time.Since(epb.startTime)
	logger.Logger.Info(fmt.Sprintf(
		"%s completed in %v",
		epb.title,
		duration))
}
