package app

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"corder/internal/platform"
	"corder/internal/storage"
)

type fileAction struct {
	id      string
	key     string
	label   string
	aliases []string
	run     func(actionRuntime, storage.Recording) tea.Cmd
}

type actionRuntime struct {
	platform platform.OS
}

func builtinFileActions() []fileAction {
	return []fileAction{
		{
			id:    "open",
			key:   "enter",
			label: "Enter: open",
			run: func(rt actionRuntime, rec storage.Recording) tea.Cmd {
				return openFileCmd(rt, rec.Path)
			},
		},
		{
			id:      "reveal",
			key:     "r",
			label:   "R: reveal",
			aliases: []string{"R"},
			run: func(rt actionRuntime, rec storage.Recording) tea.Cmd {
				return revealFileCmd(rt, rec.Path)
			},
		},
		{
			id:      "copy-path",
			key:     "p",
			label:   "P: copy path",
			aliases: []string{"P"},
			run: func(rt actionRuntime, rec storage.Recording) tea.Cmd {
				return copyPathCmd(rt, rec.Path)
			},
		},
		{
			id:      "copy-file",
			key:     "c",
			label:   "C: copy file",
			aliases: []string{"C"},
			run: func(rt actionRuntime, rec storage.Recording) tea.Cmd {
				return copyFileCmd(rt, rec.Path)
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
	return action.run(m.actionRuntime(), rec), true
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

func (m *model) actionRuntime() actionRuntime {
	return actionRuntime{platform: m.platform}
}

func openFileCmd(rt actionRuntime, path string) tea.Cmd {
	return func() tea.Msg {
		return actionResultMsg{
			actionID: "open",
			path:     path,
			message:  openSuccessMessage(path),
			err:      rt.platform.Open(path),
		}
	}
}

func revealFileCmd(rt actionRuntime, path string) tea.Cmd {
	return func() tea.Msg {
		return actionResultMsg{
			actionID: "reveal",
			path:     path,
			message:  revealSuccessMessage(path),
			err:      rt.platform.Reveal(path),
		}
	}
}

func copyPathCmd(rt actionRuntime, path string) tea.Cmd {
	return func() tea.Msg {
		return actionResultMsg{
			actionID: "copy-path",
			path:     path,
			message:  copySuccessMessage(path, false),
			err:      rt.platform.CopyToClipboard(path),
		}
	}
}

func copyFileCmd(rt actionRuntime, path string) tea.Cmd {
	return func() tea.Msg {
		return actionResultMsg{
			actionID: "copy-file",
			path:     path,
			message:  copySuccessMessage(path, true),
			err:      rt.platform.CopyFileReference(path),
		}
	}
}
