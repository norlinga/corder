package app

import (
	"testing"

	"corder/internal/audio"
	"corder/internal/settings"
	"corder/internal/storage"
)

func TestHandleMainSettingsLoadsDevicesOnlyWhenNeeded(t *testing.T) {
	empty := &model{}
	_, cmd := empty.handleMainKey("s")
	if empty.screen != screenSettings || empty.message != "Settings" {
		t.Fatalf("settings transition = screen:%v message:%q", empty.screen, empty.message)
	}
	if cmd == nil {
		t.Fatal("expected device load command when devices are empty")
	}

	loaded := &model{devices: []audio.Device{{Name: "default"}}}
	_, cmd = loaded.handleMainKey("s")
	if cmd != nil {
		t.Fatal("did not expect device load command when devices are already loaded")
	}
}

func TestHandleMainDiagnosticsQueuesProbe(t *testing.T) {
	m := &model{}

	_, cmd := m.handleMainKey("i")

	if m.screen != screenDiagnostics || m.message != "Diagnostics" {
		t.Fatalf("diagnostics transition = screen:%v message:%q", m.screen, m.message)
	}
	if cmd == nil {
		t.Fatal("expected diagnostics command")
	}
}

func TestHandleMainRenameAndDeleteUseSelectedRecording(t *testing.T) {
	m := &model{
		selected: 1,
		records: []storage.Recording{
			{Name: "first.mp3", Path: "/recordings/first.mp3"},
			{Name: "second.mp3", Path: "/recordings/second.mp3"},
		},
		message: "stale",
	}

	_, cmd := m.handleMainKey("n")
	if cmd != nil {
		t.Fatal("rename transition returned command")
	}
	if m.screen != screenRename || m.editing != "second.mp3" || string(m.editBuffer) != "second" || m.message != "" {
		t.Fatalf("rename state = screen:%v editing:%q buffer:%q message:%q", m.screen, m.editing, string(m.editBuffer), m.message)
	}

	m.screen = screenMain
	m.message = "stale"
	_, cmd = m.handleMainKey("d")
	if cmd != nil {
		t.Fatal("delete transition returned command")
	}
	if m.screen != screenDeleteConfirm || m.deleteTarget != "/recordings/second.mp3" || m.message != "" {
		t.Fatalf("delete state = screen:%v target:%q message:%q", m.screen, m.deleteTarget, m.message)
	}
}

func TestHandleSettingsEditBuffer(t *testing.T) {
	m := &model{
		cfg:        settings.Config{RecordingDir: "/old", MP3BitrateKbps: 128},
		screen:     screenSettings,
		editing:    "dir",
		editBuffer: []rune("/tmp"),
	}

	_, _ = m.handleSettingsKey("x")
	if string(m.editBuffer) != "/tmpx" {
		t.Fatalf("append buffer = %q", string(m.editBuffer))
	}
	_, _ = m.handleSettingsKey("backspace")
	if string(m.editBuffer) != "/tmp" {
		t.Fatalf("backspace buffer = %q", string(m.editBuffer))
	}
	_, cmd := m.handleSettingsKey("enter")
	if cmd != nil {
		t.Fatal("settings commit returned command")
	}
	if m.cfg.RecordingDir != "/tmp" || m.editing != "" || m.editBuffer != nil {
		t.Fatalf("settings commit = dir:%q editing:%q buffer:%q", m.cfg.RecordingDir, m.editing, string(m.editBuffer))
	}
}

func TestHandleSettingsEscReturnsAndRefreshes(t *testing.T) {
	m := &model{screen: screenSettings}

	_, cmd := m.handleSettingsKey("esc")

	if m.screen != screenMain {
		t.Fatalf("screen = %v, want screenMain", m.screen)
	}
	if cmd == nil {
		t.Fatal("expected refresh command")
	}
}

func TestDiagnosticsRerunVsMainReveal(t *testing.T) {
	diagnostics := &model{screen: screenDiagnostics}
	_, cmd := diagnostics.handleDiagnosticsKey("r")
	if diagnostics.message != "Diagnostics" || cmd == nil {
		t.Fatalf("diagnostics rerun = message:%q cmd nil:%t", diagnostics.message, cmd == nil)
	}

	main := &model{records: []storage.Recording{{Path: "/recordings/a.mp3"}}}
	_, cmd = main.handleMainKey("r")
	if cmd == nil {
		t.Fatal("expected reveal command from main r")
	}
}
