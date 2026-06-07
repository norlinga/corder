package extensions

import (
	"path/filepath"
	"testing"
)

func TestExpandArgsOnlyKnownTokens(t *testing.T) {
	ctx := TemplateContext{
		Path:         "/recordings/a.mp3",
		MetaPath:     "/recordings/a.json",
		Name:         "a.mp3",
		RecordingDir: "/recordings",
		ConfigDir:    "/config/corder",
	}

	got := ExpandArgs([]string{"--file", "{{path}}", "{{unknown}}", "{{recording_dir}}/{{name}}", "{{meta_path}}", "{{config_dir}}"}, ctx)
	want := []string{"--file", "/recordings/a.mp3", "{{unknown}}", "/recordings/a.mp3", "/recordings/a.json", "/config/corder"}

	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("arg %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestTemplateContextFor(t *testing.T) {
	path := filepath.Join("/recordings", "clip.wav")
	ctx := TemplateContextFor(path, filepath.Join("/recordings", "clip.json"), "/config/corder")

	if ctx.Name != "clip.wav" || ctx.RecordingDir != "/recordings" || ctx.ConfigDir != "/config/corder" {
		t.Fatalf("context = %+v", ctx)
	}
}
