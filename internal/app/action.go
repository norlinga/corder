package app

import (
	"context"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"corder/internal/extensions"
	"corder/internal/jobs"
	"corder/internal/platform"
	"corder/internal/storage"
)

type actionSource string

const (
	actionSourceBuiltin actionSource = "builtin"
	actionSourcePlugin  actionSource = "plugin"
)

type recordingAction struct {
	id       string
	source   actionSource
	pluginID string
	key      string
	label    string
	aliases  []string
	formats  []string
	run      func(actionRuntime, storage.Recording) tea.Cmd
}

type actionRuntime struct {
	platform  platform.OS
	configDir string
	updates   chan tea.Msg
}

func builtinRecordingActions() []recordingAction {
	return []recordingAction{
		{
			id:     "open",
			source: actionSourceBuiltin,
			key:    "enter",
			label:  "Enter: open",
			run: func(rt actionRuntime, rec storage.Recording) tea.Cmd {
				return openFileCmd(rt, rec.Path)
			},
		},
		{
			id:      "reveal",
			source:  actionSourceBuiltin,
			key:     "r",
			label:   "R: reveal",
			aliases: []string{"R"},
			run: func(rt actionRuntime, rec storage.Recording) tea.Cmd {
				return revealFileCmd(rt, rec.Path)
			},
		},
		{
			id:      "copy-path",
			source:  actionSourceBuiltin,
			key:     "p",
			label:   "P: copy path",
			aliases: []string{"P"},
			run: func(rt actionRuntime, rec storage.Recording) tea.Cmd {
				return copyPathCmd(rt, rec.Path)
			},
		},
		{
			id:      "copy-file",
			source:  actionSourceBuiltin,
			key:     "c",
			label:   "C: copy file",
			aliases: []string{"C"},
			run: func(rt actionRuntime, rec storage.Recording) tea.Cmd {
				return copyFileCmd(rt, rec.Path)
			},
		},
	}
}

func builtinActionKeys() []string {
	actions := builtinRecordingActions()
	keys := make([]string, 0, len(actions))
	for _, action := range actions {
		keys = append(keys, action.key)
		keys = append(keys, action.aliases...)
	}
	return keys
}

func (a recordingAction) matches(key string) bool {
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

func (a recordingAction) appliesTo(rec storage.Recording) bool {
	if len(a.formats) == 0 {
		return true
	}
	return extensions.RegisteredAction{Formats: a.formats}.AppliesTo(rec.Path)
}

func (m *model) recordingActions() []recordingAction {
	if m.actions != nil {
		return m.actions
	}
	return builtinRecordingActions()
}

func (m *model) handleFileActionKey(key string) (tea.Cmd, bool) {
	action, ok := m.recordingActionForKey(key)
	if !ok {
		return nil, false
	}
	rec, ok := m.selectedRecording()
	if !ok {
		return nil, true
	}
	if !action.appliesTo(rec) {
		return nil, true
	}
	return action.run(m.actionRuntime(), rec), true
}

func (m *model) recordingActionForKey(key string) (recordingAction, bool) {
	for _, action := range m.recordingActions() {
		if action.matches(key) {
			return action, true
		}
	}
	return recordingAction{}, false
}

func fileActionForKey(key string) (recordingAction, bool) {
	for _, action := range builtinRecordingActions() {
		if action.matches(key) {
			return action, true
		}
	}
	return recordingAction{}, false
}

func (m *model) selectedRecording() (storage.Recording, bool) {
	if len(m.records) == 0 || m.selected < 0 || m.selected >= len(m.records) {
		return storage.Recording{}, false
	}
	return m.records[m.selected], true
}

func fileActionFooter() string {
	labels := make([]string, 0, len(builtinRecordingActions()))
	for _, action := range builtinRecordingActions() {
		labels = append(labels, action.label)
	}
	return strings.Join(labels, "  ")
}

func (m *model) fileActionFooter() string {
	actions := m.recordingActions()
	labels := make([]string, 0, len(actions))
	for _, action := range actions {
		labels = append(labels, action.label)
	}
	return strings.Join(labels, "  ")
}

func (m *model) actionRuntime() actionRuntime {
	return actionRuntime{platform: m.platform, configDir: m.configDir, updates: m.updates}
}

func recordingActionsFromExtensions(result extensions.LoadResult) []recordingAction {
	actions := builtinRecordingActions()
	for _, action := range result.Actions {
		pluginAction := action
		actions = append(actions, recordingAction{
			id:       pluginAction.FullID(),
			source:   actionSourcePlugin,
			pluginID: pluginAction.PluginID,
			key:      pluginAction.Key,
			label:    pluginAction.Key + ": " + pluginAction.Label,
			formats:  pluginAction.Formats,
			run: func(rt actionRuntime, rec storage.Recording) tea.Cmd {
				return pluginActionCmd(rt, rec, pluginAction)
			},
		})
	}
	return actions
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

func pluginActionCmd(rt actionRuntime, rec storage.Recording, action extensions.RegisteredAction) tea.Cmd {
	return func() tea.Msg {
		inv := extensions.Invocation{
			Action:    action,
			Path:      rec.Path,
			MetaPath:  rec.MetaPath,
			ConfigDir: rt.configDir,
		}
		if action.Job && rt.updates != nil {
			updates := make(chan jobs.Update, 32)
			go func() {
				_ = extensions.Run(context.Background(), inv, updates)
				close(updates)
			}()
			go func() {
				for update := range updates {
					rt.updates <- jobMsg(update)
				}
			}()
			return jobMsg(jobs.Update{
				ID:      jobs.ID(action.Kind(), rec.Path),
				Kind:    action.Kind(),
				Path:    rec.Path,
				Message: action.Label,
				Status:  jobs.StatusQueued,
			})
		}

		updates := make(chan jobs.Update, 32)
		done := make(chan error, 1)
		go func() {
			done <- extensions.Run(context.Background(), inv, updates)
			close(updates)
		}()
		var last jobs.Update
		for update := range updates {
			last = update
		}
		err := <-done
		if err != nil {
			if last.Message != "" {
				return actionResultMsg{actionID: action.FullID(), path: rec.Path, err: errPluginMessage(last.Message)}
			}
			return actionResultMsg{actionID: action.FullID(), path: rec.Path, err: err}
		}
		message := last.Message
		if message == "" {
			message = action.Label + " complete"
		}
		return actionResultMsg{actionID: action.FullID(), path: rec.Path, message: message}
	}
}

type errPluginMessage string

func (e errPluginMessage) Error() string {
	return string(e)
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
