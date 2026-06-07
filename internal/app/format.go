package app

import (
	"fmt"
	"path/filepath"
	"strings"

	"corder/internal/audio"
)

func renameSuccessMessage(path string) string {
	return fmt.Sprintf("✓ Renamed to %s", filepath.Base(path))
}

func deleteSuccessMessage(path string) string {
	return fmt.Sprintf("✓ Deleted %s", filepath.Base(path))
}

func openSuccessMessage(path string) string {
	return fmt.Sprintf("Opened %s", filepath.Base(path))
}

func revealSuccessMessage(path string) string {
	return fmt.Sprintf("Revealed %s", filepath.Base(path))
}

func copySuccessMessage(path string, file bool) string {
	if file {
		return fmt.Sprintf("Copied file %s", filepath.Base(path))
	}
	return "Copied path"
}

func captureIssueSummary(stats audio.CaptureStats) string {
	return fmt.Sprintf("Capture stats: port overflows %d, dropped buffers %d, queue peak %s",
		stats.PortOverflow,
		stats.DroppedBuffers,
		stats.QueueSummary(),
	)
}

func formatCaptureStats(stats audio.CaptureStats) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Device: %s\n", fallbackDisplay(stats.DeviceName)))
	if stats.HostAPIName != "" {
		b.WriteString(fmt.Sprintf("Host API: %s\n", stats.HostAPIName))
	}
	b.WriteString(fmt.Sprintf("Sample rate: %d Hz\n", stats.SampleRate))
	b.WriteString(fmt.Sprintf("Channels: %d\n", stats.Channels))
	b.WriteString(fmt.Sprintf("Frames/buffer: %d\n", stats.FramesPerBuffer))
	b.WriteString(fmt.Sprintf("Buffer capacity: %d\n", stats.BufferCapacity))
	b.WriteString(fmt.Sprintf("Callbacks: %d\n", stats.Callbacks))
	b.WriteString(fmt.Sprintf("Frames captured: %d\n", stats.FramesCaptured))
	b.WriteString(fmt.Sprintf("PortAudio overflows: %d\n", stats.PortOverflow))
	b.WriteString(fmt.Sprintf("Dropped buffers: %d\n", stats.DroppedBuffers))
	b.WriteString(fmt.Sprintf("Queue peak: %s\n", stats.QueueSummary()))
	return b.String()
}

func fallbackDisplay(s string) string {
	if s == "" {
		return "(unknown)"
	}
	return s
}

func formatLevelMeter(peak float64, recording bool, clipping bool) string {
	if peak == 0 && !recording {
		return "Ready"
	}
	clipped := ""
	if clipping {
		clipped = " CLIP"
	}
	val := (peak + 60) / 60
	if val < 0 {
		val = 0
	}
	if val > 1 {
		val = 1
	}
	width := 24
	fill := int(val * float64(width))
	if fill < 0 {
		fill = 0
	}
	if fill > width {
		fill = width
	}
	return fmt.Sprintf("Input Level [%s] Peak: %.1f dB%s",
		strings.Repeat("█", fill)+strings.Repeat("░", width-fill),
		peak,
		clipped,
	)
}

func statusBadge(status string) string {
	trimmed := strings.TrimSpace(status)
	if trimmed == "" {
		return ""
	}
	lower := strings.ToLower(trimmed)
	switch {
	case strings.Contains(lower, "record"):
		return recordingStyle.Render("● " + trimmed)
	case strings.Contains(lower, "final"):
		return warningStyle.Render("● " + trimmed)
	case strings.Contains(lower, "convert"):
		return convertingStyle.Render("● " + trimmed)
	case strings.Contains(lower, "saved"), strings.Contains(lower, "ready"):
		return readyStyle.Render("● " + trimmed)
	case strings.Contains(lower, "interrupt"), strings.Contains(lower, "overflow"):
		return warningStyle.Render("● " + trimmed)
	case strings.Contains(lower, "fail"), strings.Contains(lower, "error"), strings.Contains(lower, "not found"):
		return errorStyle.Render("● " + trimmed)
	default:
		return trimmed
	}
}
