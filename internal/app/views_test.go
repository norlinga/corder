package app

import (
	"strings"
	"testing"
	"time"

	"corder/internal/audio"
	"corder/internal/extensions"
	"corder/internal/jobs"
	"corder/internal/settings"
	"corder/internal/storage"
)

func TestMainViewEmptyStateAndFooter(t *testing.T) {
	m := &model{cfg: settings.Config{RecordingDir: "/recordings"}}

	view := m.mainView()

	for _, want := range []string{"No recordings yet", "Enter: open", "R: reveal", "P: copy path", "C: copy file"} {
		if !strings.Contains(view, want) {
			t.Fatalf("main view missing %q in:\n%s", want, view)
		}
	}
}

func TestMainViewUsesConversionDestinationName(t *testing.T) {
	m := &model{
		cfg:  settings.Config{RecordingDir: "/recordings"},
		jobs: jobs.NewTracker(),
		records: []storage.Recording{{
			Name:      "a.wav",
			Path:      "/recordings/a.wav",
			CreatedAt: time.Date(2026, 6, 1, 1, 2, 3, 0, time.UTC),
			Status:    storage.StatusReady,
		}},
	}
	m.jobs.Set(jobs.Update{
		Path:        "/recordings/a.wav",
		Kind:        jobs.KindConversion,
		ID:          jobs.ID(jobs.KindConversion, "/recordings/a.wav"),
		Destination: "/recordings/a.mp3",
		Message:     "Converting",
		Percent:     50,
		Status:      jobs.StatusRunning,
	})

	view := m.mainView()

	if !strings.Contains(view, "a.mp3") || strings.Contains(view, "a.wav") {
		t.Fatalf("conversion destination not reflected in view:\n%s", view)
	}
	if !strings.Contains(view, "Converting 50%") {
		t.Fatalf("conversion status missing in view:\n%s", view)
	}
}

func TestDisplayStatusUsesPluginJobWhenNoConversion(t *testing.T) {
	rec := storage.Recording{
		Path:   "/recordings/a.mp3",
		Status: storage.StatusReady,
	}
	m := &model{jobs: jobs.NewTracker()}
	m.jobs.Set(jobs.Update{
		Kind:    "plugin:transcribe-openai.transcribe",
		ID:      jobs.ID("plugin:transcribe-openai.transcribe", rec.Path),
		Path:    rec.Path,
		Message: "Transcribing",
		Percent: 42,
		Status:  jobs.StatusRunning,
	})

	if got := m.displayStatus(rec); got != "Transcribing 42%" {
		t.Fatalf("displayStatus = %q, want plugin status", got)
	}
}

func TestDiagnosticsViewIncludesLastCaptureStats(t *testing.T) {
	m := &model{
		diagnosticRun: true,
		lastCapture: audio.CaptureStats{
			DeviceName:       "default",
			SampleRate:       44100,
			Channels:         1,
			FramesPerBuffer:  4096,
			BufferCapacity:   128,
			Callbacks:        2,
			FramesCaptured:   8192,
			MaxQueuedBuffers: 1,
		},
	}

	view := m.diagnosticsView()

	for _, want := range []string{"Last recording capture", "Device: default", "Frames captured: 8192"} {
		if !strings.Contains(view, want) {
			t.Fatalf("diagnostics view missing %q in:\n%s", want, view)
		}
	}
}

func TestDiagnosticsViewIncludesExtensionIssues(t *testing.T) {
	m := &model{
		diagnosticRun: true,
		extensions: extensions.LoadResult{
			Actions: []extensions.RegisteredAction{{PluginID: "p", ActionID: "a"}},
			Issues:  []extensions.Issue{{PluginID: "p", ActionID: "bad", Message: "action key is empty"}},
		},
	}

	view := m.diagnosticsView()

	for _, want := range []string{"Extensions", "Registered actions: 1", "p.bad: action key is empty"} {
		if !strings.Contains(view, want) {
			t.Fatalf("diagnostics view missing %q in:\n%s", want, view)
		}
	}
}

func TestRenameAndDeleteViewsShowFeedback(t *testing.T) {
	rename := &model{editBuffer: []rune("new-name"), message: "Name cannot be empty"}
	renameView := rename.renameView()
	for _, want := range []string{"Rename recording", "New name: new-name", "Name cannot be empty"} {
		if !strings.Contains(renameView, want) {
			t.Fatalf("rename view missing %q in:\n%s", want, renameView)
		}
	}

	del := &model{deleteTarget: "/recordings/a.mp3", message: "delete failed"}
	deleteView := del.deleteView()
	for _, want := range []string{"Delete recording?", "/recordings/a.mp3", "delete failed", "[y/N]"} {
		if !strings.Contains(deleteView, want) {
			t.Fatalf("delete view missing %q in:\n%s", want, deleteView)
		}
	}
}
