package app

import (
	"strings"
	"testing"
)

func TestFileActionForKey(t *testing.T) {
	tests := []struct {
		key string
		id  string
	}{
		{key: "enter", id: "open"},
		{key: "r", id: "reveal"},
		{key: "R", id: "reveal"},
		{key: "p", id: "copy-path"},
		{key: "P", id: "copy-path"},
		{key: "c", id: "copy-file"},
		{key: "C", id: "copy-file"},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			action, ok := fileActionForKey(tt.key)
			if !ok {
				t.Fatalf("fileActionForKey(%q) not found", tt.key)
			}
			if action.id != tt.id {
				t.Fatalf("action id = %q, want %q", action.id, tt.id)
			}
		})
	}

	if _, ok := fileActionForKey("q"); ok {
		t.Fatal("q unexpectedly matched a file action")
	}
}

func TestFileActionFooter(t *testing.T) {
	footer := fileActionFooter()
	for _, want := range []string{"Enter: open", "R: reveal", "P: copy path", "C: copy file"} {
		if !strings.Contains(footer, want) {
			t.Fatalf("footer missing %q in %q", want, footer)
		}
	}
}

func TestHandleFileActionKeyWithoutSelection(t *testing.T) {
	m := &model{}

	cmd, ok := m.handleFileActionKey("enter")
	if !ok {
		t.Fatal("enter should be handled as a file action")
	}
	if cmd != nil {
		t.Fatal("expected nil command without selected recording")
	}
}
