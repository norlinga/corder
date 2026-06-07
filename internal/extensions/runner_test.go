package extensions

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"corder/internal/jobs"
)

func TestRunMapsJSONLinesToJobUpdates(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script test")
	}
	dir := t.TempDir()
	script := writeScript(t, dir, `#!/bin/sh
printf '%s\n' '{"type":"status","message":"Transcribing"}'
printf '%s\n' '{"type":"progress","message":"Transcribing","percent":42}'
printf '%s\n' '{"type":"result","message":"Transcript saved","paths":["/recordings/a.txt"]}'
`)
	action := RegisteredAction{
		PluginID: "transcribe-openai",
		ActionID: "transcribe",
		Label:    "transcribe",
		Command:  script,
		Args:     []string{"--file", "{{path}}"},
	}
	updates := make(chan jobs.Update, 8)

	err := Run(context.Background(), Invocation{
		Action:    action,
		Path:      "/recordings/a.mp3",
		MetaPath:  "/recordings/a.json",
		ConfigDir: "/config/corder",
	}, updates)
	close(updates)

	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	got := drain(updates)
	if len(got) != 4 {
		t.Fatalf("updates = %d, want 4: %+v", len(got), got)
	}
	last := got[len(got)-1]
	if last.Kind != "plugin:transcribe-openai.transcribe" || last.ID != jobs.ID(last.Kind, "/recordings/a.mp3") {
		t.Fatalf("job id fields = %+v", last)
	}
	if last.Status != jobs.StatusDone || last.Message != "Transcript saved" || last.Destination != "/recordings/a.txt" {
		t.Fatalf("last update = %+v", last)
	}
	if got[2].Percent != 42 {
		t.Fatalf("progress update = %+v", got[2])
	}
}

func TestRunUsesStderrForProcessFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script test")
	}
	dir := t.TempDir()
	script := writeScript(t, dir, `#!/bin/sh
printf '%s\n' 'missing api key' >&2
exit 7
`)
	action := RegisteredAction{PluginID: "p", ActionID: "a", Label: "upload", Command: script}
	updates := make(chan jobs.Update, 8)

	err := Run(context.Background(), Invocation{Action: action, Path: "/recordings/a.wav"}, updates)
	close(updates)

	if err == nil {
		t.Fatal("Run returned nil, want failure")
	}
	got := drain(updates)
	last := got[len(got)-1]
	if last.Status != jobs.StatusFailed || !strings.Contains(last.Message, "missing api key") {
		t.Fatalf("last update = %+v", last)
	}
}

func TestRunTreatsErrorEventAsFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script test")
	}
	dir := t.TempDir()
	script := writeScript(t, dir, `#!/bin/sh
printf '%s\n' '{"type":"error","message":"OPENAI_API_KEY is not set"}'
`)
	action := RegisteredAction{PluginID: "p", ActionID: "a", Label: "transcribe", Command: script}
	updates := make(chan jobs.Update, 8)

	err := Run(context.Background(), Invocation{Action: action, Path: "/recordings/a.wav"}, updates)
	close(updates)

	if err == nil {
		t.Fatal("Run returned nil, want failure")
	}
	got := drain(updates)
	last := got[len(got)-1]
	if last.Status != jobs.StatusFailed || last.Message != "OPENAI_API_KEY is not set" {
		t.Fatalf("last update = %+v", last)
	}
}

func writeScript(t *testing.T, dir, content string) string {
	t.Helper()
	path := filepath.Join(dir, "plugin.sh")
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func drain(ch <-chan jobs.Update) []jobs.Update {
	var updates []jobs.Update
	for update := range ch {
		updates = append(updates, update)
	}
	return updates
}
