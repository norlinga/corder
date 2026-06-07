package app

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"corder/internal/audio"
	"corder/internal/conversion"
	"corder/internal/jobs"
	"corder/internal/recording"
	"corder/internal/storage"
)

func (m *model) refreshCmd() tea.Cmd {
	return func() tea.Msg {
		if err := storage.EnsureDir(m.cfg.RecordingDir); err != nil {
			return recordsMsg{err: err}
		}
		recs, err := storage.Scan(m.cfg.RecordingDir)
		return recordsMsg{recs: recs, err: err}
	}
}

func (m *model) tickCmd() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(time.Time) tea.Msg {
		return tickMsg{}
	})
}

func (m *model) listenUpdatesCmd() tea.Cmd {
	return func() tea.Msg {
		msg := <-m.updates
		return msg
	}
}

func (m *model) loadDevicesCmd() tea.Cmd {
	return func() tea.Msg {
		devs, err := m.backend.Devices()
		if err != nil {
			return devicesMsg{err: err}
		}
		return devicesMsg{devices: devs}
	}
}

func (m *model) loadDiagnosticsCmd() tea.Cmd {
	return func() tea.Msg {
		info, err := m.backend.Diagnostics()
		if err != nil {
			return diagnosticMsg{err: err}
		}
		probe, err := m.backend.Probe(m.cfg.InputDeviceName, 8)
		if err != nil {
			return diagnosticMsg{info: info, probe: probe, err: err}
		}
		return diagnosticMsg{info: info, probe: probe}
	}
}

func (m *model) startRecordingCmd() tea.Cmd {
	return func() tea.Msg {
		audioUpdates := make(chan audio.LevelUpdate, 128)
		result, err := m.workflow.Start(recording.StartRequest{
			Config:  m.cfg,
			Updates: audioUpdates,
		})
		if err != nil {
			close(audioUpdates)
			return recordStoppedMsg{err: err}
		}
		go func() {
			for upd := range audioUpdates {
				m.updates <- levelMsg(upd)
			}
		}()
		return recordStartedMsg{session: result.Session, path: result.Path, deviceName: result.DeviceName}
	}
}

func (m *model) stopRecordingCmd() tea.Cmd {
	return func() tea.Msg {
		if m.session == nil {
			return recordStoppedMsg{}
		}
		result, err := m.workflow.Stop(recording.StopRequest{
			Session: m.session,
			Config:  m.cfg,
		})
		if err != nil {
			return recordStoppedMsg{path: result.Path, duration: result.Duration, err: err}
		}
		if result.Queue {
			go m.runConversion(result.Path, result.StartedAt, result.Duration, result.BitrateKbps, result.RetainWAV)
		}
		return recordStoppedMsg{path: result.Path, destination: result.Destination, duration: result.Duration, queued: result.Queue}
	}
}

func (m *model) runConversion(source string, startedAt time.Time, duration time.Duration, bitrate int, retainWAV bool) {
	ctx := context.Background()
	dst := conversion.DestFor(source)
	job := conversion.Job{
		Source:      source,
		Destination: dst,
		StartedAt:   startedAt,
		Duration:    duration,
		BitrateKbps: bitrate,
		RetainWAV:   retainWAV,
	}
	updates := make(chan jobs.Update, 32)
	go func() {
		_ = conversion.Run(ctx, job, updates)
		close(updates)
	}()
	for p := range updates {
		m.updates <- jobMsg(p)
	}
}

func deleteCmd(path string) tea.Cmd {
	return func() tea.Msg {
		if err := storage.DeleteRecording(path); err != nil {
			return deleteResultMsg{path: path, err: err}
		}
		return deleteResultMsg{path: path}
	}
}
