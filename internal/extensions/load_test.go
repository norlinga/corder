package extensions

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadDiscoversAndValidatesManifests(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "plugins", "transcribe.json"), `{
		"schema": 1,
		"id": "transcribe-openai",
		"name": "Transcribe with OpenAI",
		"version": "0.1.0",
		"actions": [{
			"id": "transcribe",
			"key": "T",
			"label": "transcribe",
			"command": "corder-transcribe-openai",
			"args": ["--file", "{{path}}"],
			"formats": [".MP3", ".wav"],
			"job": true,
			"timeout_seconds": 1800
		}]
	}`)

	result := Load(LoadOptions{ConfigDir: dir, BuiltinKeys: []string{"r"}})

	if len(result.Issues) != 0 {
		t.Fatalf("issues = %#v, want none", result.Issues)
	}
	if len(result.Actions) != 1 {
		t.Fatalf("actions = %d, want 1", len(result.Actions))
	}
	action := result.Actions[0]
	if action.FullID() != "transcribe-openai.transcribe" || action.Kind() != "plugin:transcribe-openai.transcribe" {
		t.Fatalf("action ids = %q %q", action.FullID(), action.Kind())
	}
	if action.Key != "T" || action.Label != "transcribe" || !action.Job || action.TimeoutSeconds != 1800 {
		t.Fatalf("action fields = %+v", action)
	}
	if !action.AppliesTo("/recordings/a.mp3") || !action.AppliesTo("/recordings/a.wav") || action.AppliesTo("/recordings/a.flac") {
		t.Fatalf("format filtering failed for %+v", action.Formats)
	}
}

func TestLoadDisablesInvalidAndConflictingActions(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "plugins", "bad", "plugin.json"), `{
		"schema": 1,
		"id": "uploader",
		"actions": [
			{"id": "upload", "key": "r", "label": "upload", "command": "upload", "formats": [".mp3"]},
			{"id": "bad-format", "key": "U", "label": "bad", "command": "upload", "formats": ["mp3"]},
			{"id": "good", "key": "G", "label": "good", "command": "upload", "formats": [".wav"]}
		]
	}`)

	result := Load(LoadOptions{ConfigDir: dir, BuiltinKeys: []string{"r"}})

	if len(result.Actions) != 1 || result.Actions[0].ActionID != "good" {
		t.Fatalf("actions = %+v, want only good", result.Actions)
	}
	joined := issuesText(result.Issues)
	for _, want := range []string{"conflicts with a built-in action", "invalid format extension"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("issues missing %q in %q", want, joined)
		}
	}
}

func TestLoadUnsupportedSchemaSkipsManifest(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "plugins", "future.json"), `{"schema": 99, "id": "future"}`)

	result := Load(LoadOptions{ConfigDir: dir})

	if len(result.Actions) != 0 {
		t.Fatalf("actions = %+v, want none", result.Actions)
	}
	if len(result.Issues) != 1 || !strings.Contains(result.Issues[0].Message, "unsupported schema") {
		t.Fatalf("issues = %+v", result.Issues)
	}
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func issuesText(issues []Issue) string {
	parts := make([]string, len(issues))
	for i, issue := range issues {
		parts[i] = issue.String()
	}
	return strings.Join(parts, "\n")
}
