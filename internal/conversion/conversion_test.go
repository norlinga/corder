package conversion

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestDestFor(t *testing.T) {
	if got := DestFor("/tmp/recording.wav"); got != "/tmp/recording.mp3" {
		t.Fatalf("DestFor = %q, want /tmp/recording.mp3", got)
	}
	if got := DestFor("/tmp/recording"); got != "/tmp/recording.mp3" {
		t.Fatalf("DestFor no extension = %q, want /tmp/recording.mp3", got)
	}
}

func TestFFmpegArgs(t *testing.T) {
	job := Job{
		Source:      "in.wav",
		Destination: "out.mp3",
		BitrateKbps: 128,
	}
	want := []string{
		"-y",
		"-hide_banner",
		"-loglevel", "error",
		"-i", "in.wav",
		"-b:a", "128k",
		"-progress", "pipe:1",
		"-nostats",
		"out.mp3",
	}
	if got := ffmpegArgs(job); !reflect.DeepEqual(got, want) {
		t.Fatalf("ffmpegArgs = %#v, want %#v", got, want)
	}
}

func TestProgressPercent(t *testing.T) {
	pct, ok := progressPercent("out_time_ms=5000000", 10*time.Second)
	if !ok {
		t.Fatal("progressPercent ok = false, want true")
	}
	if pct != 50 {
		t.Fatalf("progressPercent = %f, want 50", pct)
	}
	pct, ok = progressPercent("out_time_ms=12000000", 10*time.Second)
	if !ok || pct != 99 {
		t.Fatalf("progressPercent clamp = %f, %t; want 99, true", pct, ok)
	}
	if _, ok := progressPercent("progress=continue", 10*time.Second); ok {
		t.Fatal("progressPercent accepted non-time line")
	}
}

func TestSourceHasDistinctMeta(t *testing.T) {
	sameBase := Job{
		Source:      "/tmp/recording.wav",
		Destination: "/tmp/recording.mp3",
	}
	if sourceHasDistinctMeta(sameBase) {
		t.Fatal("sourceHasDistinctMeta = true for same-base wav/mp3; would delete converted MP3 sidecar")
	}

	differentBase := Job{
		Source:      "/tmp/source.wav",
		Destination: "/tmp/output.mp3",
	}
	if !sourceHasDistinctMeta(differentBase) {
		t.Fatal("sourceHasDistinctMeta = false for different-base files")
	}
}

func TestStreamProgress(t *testing.T) {
	job := Job{
		Source:      "in.wav",
		Destination: "out.mp3",
		Duration:    10 * time.Second,
	}
	updates := make(chan Progress, 4)
	streamProgress(strings.NewReader("out_time_ms=1000000\nprogress=continue\nout_time_ms=2500000\n"), job, updates)
	close(updates)

	var got []float64
	for upd := range updates {
		got = append(got, upd.Percent)
	}
	want := []float64{10, 25}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("progress updates = %#v, want %#v", got, want)
	}
}
