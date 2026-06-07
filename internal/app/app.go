package app

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"corder/internal/audio"
	"corder/internal/extensions"
	"corder/internal/jobs"
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

type model struct {
	cfg           settings.Config
	backend       *audio.Backend
	records       []storage.Recording
	selected      int
	screen        screen
	message       string
	devices       []audio.Device
	deviceIndex   int
	recording     bool
	paused        bool
	session       recording.Session
	currentPath   string
	peakDB        float64
	clipping      bool
	overflow      bool
	captureStats  audio.CaptureStats
	lastCapture   audio.CaptureStats
	jobs          jobs.Tracker
	diagnostics   audio.Diagnostics
	extensions    extensions.LoadResult
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
	configDir     string
	actions       []recordingAction
}

func Run() error {
	cfg, err := settings.Load()
	if err != nil {
		return err
	}
	configDir, err := settings.ConfigDir()
	if err != nil {
		return err
	}
	extensionResult := extensions.Load(extensions.LoadOptions{
		ConfigDir:   configDir,
		BuiltinKeys: builtinActionKeys(),
	})
	backend := &audio.Backend{}
	m := &model{
		cfg:        cfg,
		backend:    backend,
		jobs:       jobs.NewTracker(),
		extensions: extensionResult,
		updates:    make(chan tea.Msg, 128),
		platform:   platform.New(),
		configDir:  configDir,
		actions:    recordingActionsFromExtensions(extensionResult),
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
	case tea.KeyMsg:
		return m.handleKey(msg)
	case tickMsg:
		return m, m.tickCmd()
	case refreshMsg:
		return m, m.refreshCmd()
	case recordsMsg:
		if msg.err != nil {
			m.message = msg.err.Error()
			return m, nil
		}
		m.records = msg.recs
		if m.selected >= len(m.records) {
			m.selected = max(0, len(m.records)-1)
		}
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
		if lev.RecordingPath == m.currentPath {
			m.peakDB = lev.PeakDB
			m.clipping = lev.Clipping
			m.paused = lev.Paused
			m.overflow = lev.Overflow
			m.captureStats = lev.Stats
			m.lastCapture = lev.Stats
		}
		return m, m.listenUpdatesCmd()
	case jobMsg:
		update := jobs.Update(msg)
		m.jobs.Set(update)
		if update.Finished() {
			m.jobs.Delete(update.ID)
			m.message = update.Message
			return m, tea.Batch(m.listenUpdatesCmd(), m.refreshCmd())
		}
		m.message = update.DisplayStatus()
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
			m.jobs.Set(jobs.Update{
				Kind:        jobs.KindConversion,
				ID:          jobs.ID(jobs.KindConversion, msg.path),
				Path:        msg.path,
				Destination: msg.destination,
				Percent:     0,
				Message:     "Converting",
				Status:      jobs.StatusQueued,
			})
			m.message = "Converting to MP3"
		} else {
			m.message = "Saved WAV"
		}
		return m, tea.Batch(m.listenUpdatesCmd(), m.refreshCmd())
	case interruptedConversionMsg:
		if msg.err != nil {
			m.message = msg.err.Error()
			return m, nil
		}
		m.jobs.Set(jobs.Update{
			Kind:        jobs.KindConversion,
			ID:          jobs.ID(jobs.KindConversion, msg.path),
			Path:        msg.path,
			Destination: msg.destination,
			Percent:     0,
			Message:     "Converting",
			Status:      jobs.StatusQueued,
		})
		m.message = "Converting to MP3"
		return m, tea.Batch(
			m.listenUpdatesCmd(),
			m.refreshCmd(),
			m.runConversionCmd(msg.path, msg.startedAt, msg.duration, msg.bitrateKbps, msg.retainWAV),
		)
	case renameResultMsg:
		if msg.err != nil {
			m.message = msg.err.Error()
			return m, nil
		}
		m.screen = screenMain
		m.editing = ""
		m.editBuffer = nil
		m.message = renameSuccessMessage(msg.newPath)
		return m, tea.Batch(m.refreshCmd())
	case deleteResultMsg:
		if msg.err != nil {
			m.message = msg.err.Error()
			return m, nil
		}
		m.screen = screenMain
		m.deleteTarget = ""
		m.message = deleteSuccessMessage(msg.path)
		return m, tea.Batch(m.refreshCmd())
	case actionResultMsg:
		if msg.err != nil {
			m.message = msg.err.Error()
			return m, nil
		}
		m.message = msg.message
		return m, nil
	}
	return m, nil
}
