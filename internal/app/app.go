package app

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"corder/internal/audio"
	"corder/internal/conversion"
	"corder/internal/platform"
	"corder/internal/recording"
	"corder/internal/settings"
	"corder/internal/storage"
)

var (
	titleStyle      = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("45"))
	panelStyle      = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("240")).Padding(0, 1)
	headerStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Bold(true)
	selectedStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("229")).Bold(true)
	footerStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	readyStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true)
	recordingStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
	convertingStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true)
	warningStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("208")).Bold(true)
	errorStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Bold(true)
)

type screen int

const (
	screenMain screen = iota
	screenSettings
	screenDiagnostics
	screenRename
	screenDeleteConfirm
)

type tickMsg struct{}
type refreshMsg struct{}
type recordsMsg struct {
	recs []storage.Recording
	err  error
}
type devicesMsg struct {
	devices []audio.Device
	err     error
}
type levelMsg audio.LevelUpdate
type conversionMsg conversion.Progress
type diagnosticMsg struct {
	info  audio.Diagnostics
	probe audio.ProbeResult
	err   error
}
type recordStartedMsg struct {
	session    recording.Session
	path       string
	deviceName string
}
type recordStoppedMsg struct {
	path        string
	destination string
	duration    time.Duration
	queued      bool
	err         error
}
type renameResultMsg struct {
	oldPath string
	newPath string
	err     error
}
type deleteResultMsg struct {
	path string
	err  error
}
type openResultMsg struct {
	path string
	err  error
}
type copyResultMsg struct {
	text string
	file bool
	err  error
}

type model struct {
	cfg           settings.Config
	backend       *audio.Backend
	records       []storage.Recording
	selected      int
	screen        screen
	message       string
	ready         bool
	width         int
	height        int
	loading       bool
	devices       []audio.Device
	deviceIndex   int
	recording     bool
	paused        bool
	session       recording.Session
	currentPath   string
	levelDB       float64
	peakDB        float64
	clipping      bool
	overflow      bool
	captureStats  audio.CaptureStats
	lastCapture   audio.CaptureStats
	lastUpdate    time.Time
	converting    map[string]conversion.Progress
	diagnostics   audio.Diagnostics
	probe         audio.ProbeResult
	diagnosticErr error
	diagnosticRun bool
	updates       chan tea.Msg
	workflow      recording.Workflow
	platform      platform.OS
	editing       string
	editBuffer    []rune
	deleteTarget  string
	stopRequested bool
	err           error
}

func Run() error {
	cfg, err := settings.Load()
	if err != nil {
		return err
	}
	backend := &audio.Backend{}
	m := &model{
		cfg:        cfg,
		backend:    backend,
		converting: map[string]conversion.Progress{},
		updates:    make(chan tea.Msg, 128),
		platform:   platform.New(),
		workflow: recording.Workflow{
			Recorder:  audioRecorder{backend: backend},
			Converter: recording.SystemConverter{},
			Store:     recording.SystemStore{},
			Clock:     recording.SystemClock{},
		},
	}
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		return err
	}
	return nil
}

func (m *model) Init() tea.Cmd {
	return tea.Batch(m.refreshCmd(), m.listenUpdatesCmd(), m.tickCmd())
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tea.KeyMsg:
		return m.handleKey(msg)
	case tickMsg:
		return m, m.tickCmd()
	case refreshMsg:
		return m, m.refreshCmd()
	case recordsMsg:
		m.loading = false
		if msg.err != nil {
			m.message = msg.err.Error()
			m.err = msg.err
			return m, nil
		}
		m.records = msg.recs
		if m.selected >= len(m.records) {
			m.selected = max(0, len(m.records)-1)
		}
		m.applyStatuses()
		return m, nil
	case devicesMsg:
		if msg.err != nil {
			m.message = msg.err.Error()
			return m, nil
		}
		m.devices = msg.devices
		if m.cfg.InputDeviceName == "" && len(m.devices) > 0 {
			m.cfg.InputDeviceName = m.devices[0].Name
		}
		return m, nil
	case levelMsg:
		lev := audio.LevelUpdate(msg)
		m.lastUpdate = time.Now()
		if lev.RecordingPath == m.currentPath {
			m.levelDB = lev.PeakDB
			m.peakDB = lev.PeakDB
			m.clipping = lev.Clipping
			m.paused = lev.Paused
			m.overflow = lev.Overflow
			m.captureStats = lev.Stats
			m.lastCapture = lev.Stats
		}
		return m, m.listenUpdatesCmd()
	case conversionMsg:
		p := conversion.Progress(msg)
		m.converting[p.Source] = p
		if p.Done {
			delete(m.converting, p.Source)
			m.message = p.Message
			return m, tea.Batch(m.listenUpdatesCmd(), m.refreshCmd())
		}
		m.message = fmt.Sprintf("%s %.0f%%", p.Message, p.Percent)
		return m, m.listenUpdatesCmd()
	case diagnosticMsg:
		m.diagnosticRun = true
		if msg.err != nil {
			m.diagnosticErr = msg.err
			m.message = msg.err.Error()
			return m, nil
		}
		m.diagnostics = msg.info
		m.probe = msg.probe
		m.diagnosticErr = nil
		return m, nil
	case recordStartedMsg:
		m.session = msg.session
		m.recording = true
		m.paused = false
		m.overflow = false
		m.captureStats = audio.CaptureStats{}
		m.lastCapture = audio.CaptureStats{}
		m.currentPath = msg.path
		m.cfg.InputDeviceName = msg.deviceName
		m.message = "Recording"
		return m, tea.Batch(m.listenUpdatesCmd(), m.refreshCmd())
	case recordStoppedMsg:
		m.recording = false
		m.paused = false
		m.overflow = false
		if m.captureStats.FramesCaptured > 0 || m.captureStats.Callbacks > 0 {
			m.lastCapture = m.captureStats
		}
		m.captureStats = audio.CaptureStats{}
		m.stopRequested = false
		m.session = nil
		m.currentPath = ""
		if msg.err != nil {
			m.message = msg.err.Error()
			return m, m.listenUpdatesCmd()
		}
		if msg.queued {
			m.converting[msg.path] = conversion.Progress{
				Source:      msg.path,
				Destination: msg.destination,
				Percent:     0,
				Message:     "Converting",
			}
			m.message = "Converting to MP3"
		} else {
			m.message = "Saved WAV"
		}
		return m, tea.Batch(m.listenUpdatesCmd(), m.refreshCmd())
	case renameResultMsg:
		if msg.err != nil {
			m.message = msg.err.Error()
			return m, nil
		}
		m.screen = screenMain
		m.editing = ""
		m.editBuffer = nil
		m.message = fmt.Sprintf("✓ Renamed to %s", filepath.Base(msg.newPath))
		return m, tea.Batch(m.refreshCmd())
	case deleteResultMsg:
		if msg.err != nil {
			m.message = msg.err.Error()
			return m, nil
		}
		m.screen = screenMain
		m.deleteTarget = ""
		m.message = fmt.Sprintf("✓ Deleted %s", filepath.Base(msg.path))
		return m, tea.Batch(m.refreshCmd())
	case openResultMsg:
		if msg.err != nil {
			m.message = msg.err.Error()
			return m, nil
		}
		m.message = fmt.Sprintf("Opened %s", filepath.Base(msg.path))
		return m, nil
	case copyResultMsg:
		if msg.err != nil {
			m.message = msg.err.Error()
			return m, nil
		}
		if msg.file {
			m.message = fmt.Sprintf("Copied file %s", filepath.Base(msg.text))
		} else {
			m.message = "Copied path"
		}
		return m, nil
	}
	return m, nil
}

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
	case "enter":
		if len(m.records) == 0 {
			return m, nil
		}
		return m, m.openFileCmd(m.records[m.selected].Path)
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
		if len(m.records) == 0 {
			return m, nil
		}
		m.screen = screenRename
		m.editing = m.records[m.selected].Name
		m.editBuffer = []rune(strings.TrimSuffix(m.records[m.selected].Name, filepath.Ext(m.records[m.selected].Name)))
		m.message = ""
		return m, nil
	case "d":
		if len(m.records) == 0 {
			return m, nil
		}
		m.screen = screenDeleteConfirm
		m.deleteTarget = m.records[m.selected].Path
		m.message = ""
		return m, nil
	case "p":
		if len(m.records) == 0 {
			return m, nil
		}
		return m, m.copyPathCmd(m.records[m.selected].Path)
	case "c":
		if len(m.records) == 0 {
			return m, nil
		}
		return m, m.copyFileCmd(m.records[m.selected].Path)
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
	if len(m.records) == 0 {
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
		return m, renameCmd(m.records[m.selected].Path, newName)
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
			if p, ok := m.converting[rec.Path]; ok && p.Destination != "" {
				displayName = filepath.Base(p.Destination)
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
	b.WriteString(footerStyle.Render("Space: start/pause  Esc/X: stop  Enter: open  N: rename  D: delete  P: copy path  C: copy file  S: settings  I: diagnostics  Q: quit"))
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
		lines = append(lines, m.levelMeter())
	} else if m.message != "" {
		lines = append(lines, statusBadge(m.message))
	} else {
		lines = append(lines, statusBadge("Ready"))
	}
	return strings.Join(lines, "\n")
}

func captureIssueSummary(stats audio.CaptureStats) string {
	return fmt.Sprintf("Capture stats: port overflows %d, dropped buffers %d, queue peak %s",
		stats.PortOverflow,
		stats.DroppedBuffers,
		stats.QueueSummary(),
	)
}

func formatCaptureStats(stats audio.CaptureStats) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Device: %s\n", fallbackDisplay(stats.DeviceName)))
	if stats.HostAPIName != "" {
		b.WriteString(fmt.Sprintf("Host API: %s\n", stats.HostAPIName))
	}
	b.WriteString(fmt.Sprintf("Sample rate: %d Hz\n", stats.SampleRate))
	b.WriteString(fmt.Sprintf("Channels: %d\n", stats.Channels))
	b.WriteString(fmt.Sprintf("Frames/buffer: %d\n", stats.FramesPerBuffer))
	b.WriteString(fmt.Sprintf("Buffer capacity: %d\n", stats.BufferCapacity))
	b.WriteString(fmt.Sprintf("Callbacks: %d\n", stats.Callbacks))
	b.WriteString(fmt.Sprintf("Frames captured: %d\n", stats.FramesCaptured))
	b.WriteString(fmt.Sprintf("PortAudio overflows: %d\n", stats.PortOverflow))
	b.WriteString(fmt.Sprintf("Dropped buffers: %d\n", stats.DroppedBuffers))
	b.WriteString(fmt.Sprintf("Queue peak: %s\n", stats.QueueSummary()))
	return b.String()
}

func fallbackDisplay(s string) string {
	if s == "" {
		return "(unknown)"
	}
	return s
}

func (m *model) levelMeter() string {
	peak := m.peakDB
	if peak == 0 && !m.recording {
		return "Ready"
	}
	clipped := ""
	if m.clipping {
		clipped = " CLIP"
	}
	val := (peak + 60) / 60
	if val < 0 {
		val = 0
	}
	if val > 1 {
		val = 1
	}
	width := 24
	fill := int(val * float64(width))
	if fill < 0 {
		fill = 0
	}
	if fill > width {
		fill = width
	}
	return fmt.Sprintf("Input Level [%s] Peak: %.1f dB%s",
		strings.Repeat("█", fill)+strings.Repeat("░", width-fill),
		peak,
		clipped,
	)
}

func statusBadge(status string) string {
	trimmed := strings.TrimSpace(status)
	if trimmed == "" {
		return ""
	}
	switch {
	case strings.Contains(strings.ToLower(trimmed), "record"):
		return recordingStyle.Render("● " + trimmed)
	case strings.Contains(strings.ToLower(trimmed), "final"):
		return warningStyle.Render("● " + trimmed)
	case strings.Contains(strings.ToLower(trimmed), "convert"):
		return convertingStyle.Render("● " + trimmed)
	case strings.Contains(strings.ToLower(trimmed), "saved"), strings.Contains(strings.ToLower(trimmed), "ready"):
		return readyStyle.Render("● " + trimmed)
	case strings.Contains(strings.ToLower(trimmed), "interrupt"), strings.Contains(strings.ToLower(trimmed), "overflow"):
		return warningStyle.Render("● " + trimmed)
	case strings.Contains(strings.ToLower(trimmed), "fail"), strings.Contains(strings.ToLower(trimmed), "error"), strings.Contains(strings.ToLower(trimmed), "not found"):
		return errorStyle.Render("● " + trimmed)
	default:
		return trimmed
	}
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

func (m *model) applyStatuses() {
}

func (m *model) displayStatus(rec storage.Recording) string {
	if p, ok := m.converting[rec.Path]; ok {
		return fmt.Sprintf("%s %.0f%%", p.Message, p.Percent)
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
	updates := make(chan conversion.Progress, 32)
	go func() {
		if err := conversion.Run(ctx, job, updates); err != nil {
			select {
			case updates <- conversion.Progress{Source: source, Destination: dst, Err: err, Done: true, Message: "Conversion failed"}:
			default:
			}
		}
		close(updates)
	}()
	for p := range updates {
		m.updates <- conversionMsg(p)
	}
}

func renameCmd(oldPath, newName string) tea.Cmd {
	return func() tea.Msg {
		newPath, err := renameDestination(oldPath, newName)
		if err != nil {
			return renameResultMsg{oldPath: oldPath, err: err}
		}
		if err := storage.RenameWithMeta(oldPath, newPath); err != nil {
			return renameResultMsg{oldPath: oldPath, newPath: newPath, err: friendlyRenameError(err)}
		}
		return renameResultMsg{oldPath: oldPath, newPath: newPath}
	}
}

func renameDestination(oldPath, newName string) (string, error) {
	if err := validateRenameInput(newName); err != nil {
		return "", err
	}
	name := filepath.Base(strings.TrimSpace(newName))
	if !strings.Contains(name, ".") {
		name += filepath.Ext(oldPath)
	}
	return filepath.Join(filepath.Dir(oldPath), name), nil
}

func validateRenameInput(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("Name cannot be empty")
	}
	if name == "." || name == ".." {
		return errors.New("Name must include filename text")
	}
	if filepath.Base(name) != name {
		return errors.New("Name cannot include folders")
	}
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	if strings.TrimSpace(stem) == "" {
		return errors.New("Name must include filename text")
	}
	return nil
}

func friendlyRenameError(err error) error {
	if errors.Is(err, storage.ErrDestinationExists) {
		return errors.New("A recording with that name already exists")
	}
	return err
}

func deleteCmd(path string) tea.Cmd {
	return func() tea.Msg {
		if err := storage.DeleteRecording(path); err != nil {
			return deleteResultMsg{path: path, err: err}
		}
		return deleteResultMsg{path: path}
	}
}

func (m *model) openFileCmd(path string) tea.Cmd {
	return func() tea.Msg {
		return openResultMsg{path: path, err: m.platform.Open(path)}
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

func msgRunes(key string) []rune {
	if len(key) == 1 {
		return []rune(key)
	}
	return nil
}

func truncate(s string, width int) string {
	if len(s) <= width {
		return s
	}
	if width <= 3 {
		return s[:width]
	}
	return s[:width-3] + "..."
}

func clampBitrate(n int) int {
	values := []int{96, 128, 160, 192, 256, 320}
	best := values[0]
	for _, v := range values {
		if n <= v {
			return v
		}
		best = v
	}
	return best
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
