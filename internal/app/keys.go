package app

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"corder/internal/settings"
)

func (m *model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	switch m.screen {
	case screenMain:
		return m.handleMainKey(key)
	case screenSettings:
		return m.handleSettingsKey(key)
	case screenDiagnostics:
		return m.handleDiagnosticsKey(key)
	case screenRename:
		return m.handleRenameKey(key)
	case screenDeleteConfirm:
		return m.handleDeleteKey(key)
	default:
		return m, nil
	}
}

func (m *model) handleMainKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "up", "k":
		m.selected = max(0, m.selected-1)
		return m, nil
	case "down", "j":
		m.selected = min(max(0, len(m.records)-1), m.selected+1)
		return m, nil
	case "s":
		m.screen = screenSettings
		m.message = "Settings"
		if len(m.devices) == 0 {
			return m, m.loadDevicesCmd()
		}
		return m, nil
	case "i":
		m.screen = screenDiagnostics
		m.message = "Diagnostics"
		return m, m.loadDiagnosticsCmd()
	case "n":
		rec, ok := m.selectedRecording()
		if !ok {
			return m, nil
		}
		m.screen = screenRename
		m.editing = rec.Name
		m.editBuffer = []rune(strings.TrimSuffix(rec.Name, filepath.Ext(rec.Name)))
		m.message = ""
		return m, nil
	case "d":
		rec, ok := m.selectedRecording()
		if !ok {
			return m, nil
		}
		m.screen = screenDeleteConfirm
		m.deleteTarget = rec.Path
		m.message = ""
		return m, nil
	case " ":
		if m.recording {
			if m.session == nil {
				return m, nil
			}
			paused := m.session.TogglePause()
			m.paused = paused
			if paused {
				m.message = "Paused"
			} else {
				m.message = "Recording"
			}
			return m, nil
		}
		return m, m.startRecordingCmd()
	case "esc", "x":
		if m.recording && m.session != nil {
			if m.stopRequested {
				return m, nil
			}
			m.stopRequested = true
			m.message = "Finalizing WAV"
			return m, m.stopRecordingCmd()
		}
		return m, nil
	}
	if cmd, ok := m.handleFileActionKey(key); ok {
		return m, cmd
	}
	return m, nil
}

func (m *model) handleSettingsKey(key string) (tea.Model, tea.Cmd) {
	if m.editing != "" {
		switch key {
		case "esc":
			m.editing = ""
			m.editBuffer = nil
			return m, nil
		case "enter":
			switch m.editing {
			case "dir":
				if len(m.editBuffer) > 0 {
					m.cfg.RecordingDir = string(m.editBuffer)
				}
			case "bitrate":
				if n, err := strconv.Atoi(string(m.editBuffer)); err == nil {
					m.cfg.MP3BitrateKbps = clampBitrate(n)
				}
			}
			m.editing = ""
			m.editBuffer = nil
			_ = settings.Save(m.cfg)
			return m, nil
		case "backspace", "delete":
			if len(m.editBuffer) > 0 {
				m.editBuffer = m.editBuffer[:len(m.editBuffer)-1]
			}
			return m, nil
		default:
			r := msgRunes(key)
			if len(r) == 1 {
				m.editBuffer = append(m.editBuffer, r[0])
			}
			return m, nil
		}
	}
	switch key {
	case "q", "ctrl+c":
		_ = settings.Save(m.cfg)
		return m, tea.Quit
	case "esc":
		m.screen = screenMain
		_ = settings.Save(m.cfg)
		return m, tea.Batch(m.refreshCmd())
	case "up":
		m.deviceIndex = max(0, m.deviceIndex-1)
		m.cfg.InputDeviceName = m.currentDeviceName()
		return m, nil
	case "down":
		m.deviceIndex = min(max(0, len(m.devices)-1), m.deviceIndex+1)
		m.cfg.InputDeviceName = m.currentDeviceName()
		return m, nil
	case "left":
		m.cfg.MP3BitrateKbps = clampBitrate(m.cfg.MP3BitrateKbps - 32)
		return m, nil
	case "right":
		m.cfg.MP3BitrateKbps = clampBitrate(m.cfg.MP3BitrateKbps + 32)
		return m, nil
	case "tab":
		m.cfg.RetainWAVAfterConvert = !m.cfg.RetainWAVAfterConvert
		return m, nil
	case "1":
		m.editing = "dir"
		m.editBuffer = []rune(m.cfg.RecordingDir)
		return m, nil
	case "2":
		m.cfg.InputDeviceName = m.currentDeviceName()
		return m, nil
	case "3":
		m.editing = "bitrate"
		m.editBuffer = []rune(fmt.Sprintf("%d", m.cfg.MP3BitrateKbps))
		return m, nil
	case "4":
		m.cfg.RetainWAVAfterConvert = !m.cfg.RetainWAVAfterConvert
		return m, nil
	}
	return m, nil
}

func (m *model) handleDiagnosticsKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "esc":
		m.screen = screenMain
		return m, tea.Batch(m.refreshCmd())
	case "r":
		m.message = "Diagnostics"
		return m, m.loadDiagnosticsCmd()
	}
	return m, nil
}

func (m *model) handleRenameKey(key string) (tea.Model, tea.Cmd) {
	rec, ok := m.selectedRecording()
	if !ok {
		m.screen = screenMain
		return m, nil
	}
	switch key {
	case "esc":
		m.screen = screenMain
		m.editBuffer = nil
		return m, nil
	case "enter":
		newName := strings.TrimSpace(string(m.editBuffer))
		if err := validateRenameInput(newName); err != nil {
			m.message = err.Error()
			return m, nil
		}
		return m, renameCmd(rec.Path, newName)
	case "backspace", "delete":
		if len(m.editBuffer) > 0 {
			m.editBuffer = m.editBuffer[:len(m.editBuffer)-1]
		}
		return m, nil
	default:
		r := msgRunes(key)
		if len(r) == 1 {
			m.editBuffer = append(m.editBuffer, r[0])
		}
		return m, nil
	}
}

func (m *model) handleDeleteKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "y", "enter":
		target := m.deleteTarget
		return m, deleteCmd(target)
	case "n", "esc":
		m.screen = screenMain
		m.deleteTarget = ""
		return m, nil
	}
	return m, nil
}
