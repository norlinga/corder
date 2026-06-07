package app

import (
	"fmt"
	"path/filepath"
	"strings"

	"corder/internal/extensions"
	"corder/internal/jobs"
	"corder/internal/storage"
)

func (m *model) View() string {
	switch m.screen {
	case screenSettings:
		return m.settingsView()
	case screenDiagnostics:
		return m.diagnosticsView()
	case screenRename:
		return m.renameView()
	case screenDeleteConfirm:
		return m.deleteView()
	default:
		return m.mainView()
	}
}

func (m *model) mainView() string {
	var table strings.Builder
	table.WriteString(titleStyle.Render("Recordings"))
	table.WriteString("\n")
	table.WriteString(headerStyle.Render(fmt.Sprintf("%-30s %-12s %-10s %-12s %s", "Name", "Length", "Size", "Date", "Status")))
	table.WriteString("\n")
	if len(m.records) == 0 {
		table.WriteString("No recordings yet. Press Space to start recording.\n")
	} else {
		for i, rec := range m.records {
			prefix := " "
			if i == m.selected {
				prefix = ">"
			}
			status := m.displayStatus(rec)
			displayName := rec.Name
			if job, ok := m.jobs.GetByKindPath(jobs.KindConversion, rec.Path); ok && job.Destination != "" {
				displayName = filepath.Base(job.Destination)
			}
			line := fmt.Sprintf("%s %-29s %-12s %-10s %-12s %s",
				prefix,
				truncate(displayName, 29),
				storage.FormatDuration(rec.Duration),
				storage.FormatSize(rec.Size),
				rec.CreatedAt.Format("Jan 2 2006"),
				statusBadge(status),
			)
			if i == m.selected {
				line = selectedStyle.Render(line)
			}
			table.WriteString(line)
			table.WriteString("\n")
		}
	}
	var b strings.Builder
	b.WriteString(panelStyle.Render(table.String()))
	b.WriteString("\n")
	b.WriteString(panelStyle.Render(m.statusArea()))
	b.WriteString("\n")
	b.WriteString("\n")
	b.WriteString(footerStyle.Render("Space: start/pause  Esc/X: stop/process interrupted  " + m.fileActionFooter() + "  N: rename  D: delete  S: settings  I: diagnostics  Q: quit"))
	b.WriteString("\n")
	return b.String()
}

func (m *model) settingsView() string {
	var b strings.Builder
	b.WriteString("Settings\n")
	b.WriteString(strings.Repeat("-", 80))
	b.WriteString("\n")
	cur := m.currentDeviceName()
	b.WriteString(fmt.Sprintf("1. Recording directory: %s\n", m.cfg.RecordingDir))
	b.WriteString(fmt.Sprintf("2. Input device      : %s\n", cur))
	b.WriteString(fmt.Sprintf("3. MP3 bitrate       : %dkbps\n", m.cfg.MP3BitrateKbps))
	b.WriteString(fmt.Sprintf("4. Retain WAV        : %t\n", m.cfg.RetainWAVAfterConvert))
	if m.editing == "dir" {
		b.WriteString(fmt.Sprintf("\nEditing directory: %s\n", string(m.editBuffer)))
		b.WriteString("Enter to save, Esc to cancel\n")
	} else if m.editing == "bitrate" {
		b.WriteString(fmt.Sprintf("\nEditing bitrate: %s\n", string(m.editBuffer)))
		b.WriteString("Enter to save, Esc to cancel\n")
	} else {
		if len(m.devices) == 0 {
			b.WriteString("\nLoading device list on demand.\n")
		}
		b.WriteString("\nUp/Down: change device  Left/Right: bitrate  Tab: toggle retain WAV  1/3: edit  Esc: save and return\n")
	}
	return b.String()
}

func (m *model) diagnosticsView() string {
	var b strings.Builder
	b.WriteString("Diagnostics\n")
	b.WriteString(strings.Repeat("-", 80))
	b.WriteString("\n")
	if m.diagnosticErr != nil {
		b.WriteString(fmt.Sprintf("Error: %v\n", m.diagnosticErr))
	} else if !m.diagnosticRun {
		b.WriteString("Running probe...\n")
	} else {
		b.WriteString(m.diagnostics.Format())
		b.WriteString("\nProbe:\n")
		b.WriteString(m.probe.Format())
	}
	if m.lastCapture.Callbacks > 0 {
		b.WriteString("\nLast recording capture:\n")
		b.WriteString(formatCaptureStats(m.lastCapture))
	}
	if len(m.extensions.Actions) > 0 || len(m.extensions.Issues) > 0 {
		b.WriteString("\nExtensions:\n")
		b.WriteString(formatExtensionDiagnostics(m.extensions))
	}
	b.WriteString("\nEsc: back  R: rerun probe\n")
	return b.String()
}

func (m *model) renameView() string {
	name := string(m.editBuffer)
	var b strings.Builder
	b.WriteString("Rename recording\n")
	b.WriteString(strings.Repeat("-", 40))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("New name: %s\n", name))
	if m.message != "" {
		b.WriteString(statusBadge(m.message))
		b.WriteString("\n")
	}
	b.WriteString("Enter to save, Esc to cancel\n")
	return b.String()
}

func (m *model) deleteView() string {
	var b strings.Builder
	b.WriteString("Delete recording?\n")
	b.WriteString(strings.Repeat("-", 40))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("%s\n", m.deleteTarget))
	if m.message != "" {
		b.WriteString(statusBadge(m.message))
		b.WriteString("\n")
	}
	b.WriteString("[y/N]\n")
	return b.String()
}

func (m *model) statusArea() string {
	lines := []string{
		fmt.Sprintf("Input Device: %s", m.currentDeviceName()),
		fmt.Sprintf("Recording Directory: %s", m.cfg.RecordingDir),
	}
	if m.recording {
		state := "Recording"
		if m.paused {
			state = "Paused"
		}
		if m.stopRequested {
			state = "Finalizing WAV"
		}
		lines = append(lines, statusBadge(state))
		if m.overflow {
			lines = append(lines, warningStyle.Render("Input overflow"))
		}
		if m.captureStats.HasIssues() {
			lines = append(lines, warningStyle.Render(captureIssueSummary(m.captureStats)))
		}
		lines = append(lines, formatLevelMeter(m.peakDB, m.recording, m.clipping))
	} else if m.message != "" {
		lines = append(lines, statusBadge(m.message))
	} else {
		lines = append(lines, statusBadge("Ready"))
	}
	return strings.Join(lines, "\n")
}

func (m *model) currentDeviceName() string {
	if len(m.devices) == 0 {
		return m.cfg.InputDeviceName
	}
	if m.deviceIndex < 0 || m.deviceIndex >= len(m.devices) {
		m.deviceIndex = 0
	}
	if m.cfg.InputDeviceName != "" {
		for i, d := range m.devices {
			if d.Name == m.cfg.InputDeviceName {
				m.deviceIndex = i
				return d.Name
			}
		}
	}
	return m.devices[m.deviceIndex].Name
}

func (m *model) displayStatus(rec storage.Recording) string {
	if job, ok := m.jobs.GetByKindPath(jobs.KindConversion, rec.Path); ok {
		return job.DisplayStatus()
	}
	if job, ok := m.jobs.GetByPath(rec.Path); ok {
		return job.DisplayStatus()
	}
	if m.recording && rec.Path == m.currentPath {
		if m.stopRequested {
			return "Finalizing"
		}
		if m.paused {
			return "Paused"
		}
		return "Recording"
	}
	return rec.Status.String()
}

func formatExtensionDiagnostics(result extensions.LoadResult) string {
	var lines []string
	if len(result.Actions) > 0 {
		lines = append(lines, fmt.Sprintf("Registered actions: %d", len(result.Actions)))
	}
	for _, issue := range result.Issues {
		lines = append(lines, "Issue: "+issue.String())
	}
	return strings.Join(lines, "\n") + "\n"
}
