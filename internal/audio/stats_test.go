package audio

import "testing"

func TestCaptureStatsHelpers(t *testing.T) {
	stats := CaptureStats{MaxQueuedBuffers: 7, BufferCapacity: 128}
	if stats.HasIssues() {
		t.Fatal("fresh stats reported issues")
	}
	if got := stats.QueueSummary(); got != "7/128" {
		t.Fatalf("QueueSummary = %q, want 7/128", got)
	}

	stats.PortOverflow = 1
	if !stats.HasIssues() {
		t.Fatal("stats with port overflow did not report issues")
	}

	stats.PortOverflow = 0
	stats.DroppedBuffers = 1
	if !stats.HasIssues() {
		t.Fatal("stats with dropped buffers did not report issues")
	}
}
