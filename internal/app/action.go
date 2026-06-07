package app

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"corder/internal/storage"
)

type fileAction struct {
	id      string
	key     string
	label   string
	aliases []string
	run     func(*model, storage.Recording) tea.Cmd
}

func builtinFileActions() []fileAction {
	return []fileAction{
		{
			id:    "open",
			key:   "enter",
			label: "Enter: open",
			run: func(m *model, rec storage.Recording) tea.Cmd {
				return m.openFileCmd(rec.Path)
			},
		},
		{
			id:      "reveal",
			key:     "r",
			label:   "R: reveal",
			aliases: []string{"R"},
			run: func(m *model, rec storage.Recording) tea.Cmd {
				return m.revealFileCmd(rec.Path)
			},
		},
		{
			id:      "copy-path",
			key:     "p",
			label:   "P: copy path",
			aliases: []string{"P"},
			run: func(m *model, rec storage.Recording) tea.Cmd {
				return m.copyPathCmd(rec.Path)
			},
		},
		{
			id:      "copy-file",
			key:     "c",
			label:   "C: copy file",
			aliases: []string{"C"},
			run: func(m *model, rec storage.Recording) tea.Cmd {
				return m.copyFileCmd(rec.Path)
			},
		},
	}
}

func (a fileAction) matches(key string) bool {
	if key == a.key {
		return true
	}
	for _, alias := range a.aliases {
		if key == alias {
			return true
		}
	}
	return false
}

func (m *model) handleFileActionKey(key string) (tea.Cmd, bool) {
	action, ok := fileActionForKey(key)
	if !ok {
		return nil, false
	}
	rec, ok := m.selectedRecording()
	if !ok {
		return nil, true
	}
	return action.run(m, rec), true
}

func fileActionForKey(key string) (fileAction, bool) {
	for _, action := range builtinFileActions() {
		if action.matches(key) {
			return action, true
		}
	}
	return fileAction{}, false
}

func (m *model) selectedRecording() (storage.Recording, bool) {
	if len(m.records) == 0 || m.selected < 0 || m.selected >= len(m.records) {
		return storage.Recording{}, false
	}
	return m.records[m.selected], true
}

func fileActionFooter() string {
	labels := make([]string, 0, len(builtinFileActions()))
	for _, action := range builtinFileActions() {
		labels = append(labels, action.label)
	}
	return strings.Join(labels, "  ")
}

func (m *model) openFileCmd(path string) tea.Cmd {
	return func() tea.Msg {
		return openResultMsg{path: path, err: m.platform.Open(path)}
	}
}

func (m *model) revealFileCmd(path string) tea.Cmd {
	return func() tea.Msg {
		return revealResultMsg{path: path, err: m.platform.Reveal(path)}
	}
}

func (m *model) copyPathCmd(path string) tea.Cmd {
	return func() tea.Msg {
		return copyResultMsg{text: path, err: m.platform.CopyToClipboard(path)}
	}
}

func (m *model) copyFileCmd(path string) tea.Cmd {
	return func() tea.Msg {
		return copyResultMsg{text: path, file: true, err: m.platform.CopyFileReference(path)}
	}
}
