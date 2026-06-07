package app

import (
	"strings"
	"testing"
)

func TestSuccessMessages(t *testing.T) {
	tests := []struct {
		name string
		got  string
		want string
	}{
		{name: "rename", got: renameSuccessMessage("/recordings/new.mp3"), want: "✓ Renamed to new.mp3"},
		{name: "delete", got: deleteSuccessMessage("/recordings/old.mp3"), want: "✓ Deleted old.mp3"},
		{name: "open", got: openSuccessMessage("/recordings/a.mp3"), want: "Opened a.mp3"},
		{name: "reveal", got: revealSuccessMessage("/recordings/a.mp3"), want: "Revealed a.mp3"},
		{name: "copy path", got: copySuccessMessage("/recordings/a.mp3", false), want: "Copied path"},
		{name: "copy file", got: copySuccessMessage("/recordings/a.mp3", true), want: "Copied file a.mp3"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Fatalf("message = %q, want %q", tt.got, tt.want)
			}
		})
	}
}

func TestFormatLevelMeter(t *testing.T) {
	if got := formatLevelMeter(0, false, false); got != "Ready" {
		t.Fatalf("idle level meter = %q, want Ready", got)
	}

	got := formatLevelMeter(-30, true, true)
	for _, want := range []string{"Input Level", "Peak: -30.0 dB", "CLIP"} {
		if !strings.Contains(got, want) {
			t.Fatalf("level meter missing %q in %q", want, got)
		}
	}
}
