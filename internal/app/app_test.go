package app

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"corder/internal/audio"
	"corder/internal/jobs"
	"corder/internal/platform"
	"corder/internal/recording"
	"corder/internal/storage"
)

func TestDisplayStatusPrefersTransientConversionState(t *testing.T) {
	rec := storage.Recording{
		Path:   "/recordings/a.wav",
		Status: storage.StatusReady,
	}
	m := &model{
		jobs: jobs.NewTracker(),
	}
	m.jobs.Set(jobs.Update{Kind: jobs.KindConversion, ID: jobs.ID(jobs.KindConversion, rec.Path), Path: rec.Path, Message: "Converting", Percent: 37, Status: jobs.StatusRunning})

	if got := m.displayStatus(rec); got != "Converting 37%" {
		t.Fatalf("displayStatus = %q, want Converting 37%%", got)
	}
	if rec.Status != storage.StatusReady {
		t.Fatalf("displayStatus mutated record status to %q", rec.Status)
	}
}

func TestDisplayStatusUsesRecordingOverlay(t *testing.T) {
	rec := storage.Recording{
		Path:   "/recordings/a.wav",
		Status: storage.StatusReady,
	}
	m := &model{
		recording:   true,
		currentPath: rec.Path,
		jobs:        jobs.NewTracker(),
	}

	if got := m.displayStatus(rec); got != "Recording" {
		t.Fatalf("displayStatus = %q, want Recording", got)
	}
	m.paused = true
	if got := m.displayStatus(rec); got != "Paused" {
		t.Fatalf("displayStatus paused = %q, want Paused", got)
	}
	m.stopRequested = true
	if got := m.displayStatus(rec); got != "Finalizing" {
		t.Fatalf("displayStatus finalizing = %q, want Finalizing", got)
	}
}

func TestDisplayStatusFallsBackToStorageStatus(t *testing.T) {
	rec := storage.Recording{
		Path:   "/recordings/a.wav",
		Status: storage.StatusInterrupted,
	}
	m := &model{jobs: jobs.NewTracker()}

	if got := m.displayStatus(rec); got != storage.StatusInterrupted.String() {
		t.Fatalf("displayStatus = %q, want %q", got, storage.StatusInterrupted)
	}
}

func TestCaptureIssueSummary(t *testing.T) {
	stats := audio.CaptureStats{
		PortOverflow:     2,
		DroppedBuffers:   1,
		MaxQueuedBuffers: 9,
		BufferCapacity:   128,
	}
	got := captureIssueSummary(stats)
	want := "Capture stats: port overflows 2, dropped buffers 1, queue peak 9/128"
	if got != want {
		t.Fatalf("captureIssueSummary = %q, want %q", got, want)
	}
}

func TestFormatCaptureStats(t *testing.T) {
	stats := audio.CaptureStats{
		DeviceName:       "default",
		HostAPIName:      "ALSA",
		SampleRate:       44100,
		Channels:         1,
		FramesPerBuffer:  4096,
		BufferCapacity:   128,
		Callbacks:        10,
		FramesCaptured:   40960,
		PortOverflow:     1,
		DroppedBuffers:   2,
		MaxQueuedBuffers: 7,
	}
	got := formatCaptureStats(stats)
	for _, want := range []string{
		"Device: default",
		"Host API: ALSA",
		"Sample rate: 44100 Hz",
		"Frames/buffer: 4096",
		"Callbacks: 10",
		"Frames captured: 40960",
		"PortAudio overflows: 1",
		"Dropped buffers: 2",
		"Queue peak: 7/128",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("formatCaptureStats missing %q in:\n%s", want, got)
		}
	}
}

func TestRecordStoppedRetainsLastCaptureStats(t *testing.T) {
	stats := audio.CaptureStats{Callbacks: 10, FramesCaptured: 40960}
	m := &model{captureStats: stats}

	_, _ = m.Update(recordStoppedMsg{})

	if m.captureStats.Callbacks != 0 {
		t.Fatalf("captureStats was not cleared: %+v", m.captureStats)
	}
	if m.lastCapture != stats {
		t.Fatalf("lastCapture = %+v, want %+v", m.lastCapture, stats)
	}
}

func TestUpdateRecordStoppedQueuesConversion(t *testing.T) {
	m := &model{
		recording:     true,
		paused:        true,
		overflow:      true,
		stopRequested: true,
		session:       &testSession{},
		currentPath:   "/recordings/a.wav",
		jobs:          jobs.NewTracker(),
	}

	_, _ = m.Update(recordStoppedMsg{
		path:        "/recordings/a.wav",
		destination: "/recordings/a.mp3",
		queued:      true,
	})

	if m.recording || m.paused || m.overflow || m.stopRequested || m.session != nil || m.currentPath != "" {
		t.Fatalf("recording state not cleared: %+v", m)
	}
	if m.message != "Converting to MP3" {
		t.Fatalf("message = %q, want Converting to MP3", m.message)
	}
	progress, ok := m.jobs.Get(jobs.ID(jobs.KindConversion, "/recordings/a.wav"))
	if !ok {
		t.Fatal("conversion job not queued")
	}
	if progress.Destination != "/recordings/a.mp3" || progress.Message != "Converting" {
		t.Fatalf("queued job = %+v", progress)
	}
}

func TestUpdateRecordStoppedErrorDoesNotQueueConversion(t *testing.T) {
	m := &model{
		recording:   true,
		session:     &testSession{},
		currentPath: "/recordings/a.wav",
		jobs:        jobs.NewTracker(),
	}

	_, _ = m.Update(recordStoppedMsg{
		path:   "/recordings/a.wav",
		queued: true,
		err:    errTestStop,
	})

	if m.message != errTestStop.Error() {
		t.Fatalf("message = %q, want stop error", m.message)
	}
	if m.jobs.Len() != 0 {
		t.Fatalf("conversion was queued on error: %+v", m.jobs)
	}
}

func TestUpdateRecordStartedSetsRecordingState(t *testing.T) {
	session := &testSession{}
	m := &model{}

	_, _ = m.Update(recordStartedMsg{
		session:    session,
		path:       "/recordings/a.wav",
		deviceName: "default",
	})

	if !m.recording || m.session != session || m.currentPath != "/recordings/a.wav" {
		t.Fatalf("recording state not set: recording=%t session=%v path=%q", m.recording, m.session, m.currentPath)
	}
	if m.cfg.InputDeviceName != "default" {
		t.Fatalf("InputDeviceName = %q, want default", m.cfg.InputDeviceName)
	}
	if m.message != "Recording" {
		t.Fatalf("message = %q, want Recording", m.message)
	}
}

func TestUpdateLevelOnlyAppliesCurrentRecording(t *testing.T) {
	m := &model{currentPath: "/recordings/current.wav"}
	other := audio.LevelUpdate{
		RecordingPath: "/recordings/other.wav",
		PeakDB:        -1,
		Overflow:      true,
		Stats:         audio.CaptureStats{Callbacks: 1},
	}
	current := audio.LevelUpdate{
		RecordingPath: "/recordings/current.wav",
		PeakDB:        -8.5,
		Paused:        true,
		Overflow:      true,
		Stats:         audio.CaptureStats{Callbacks: 2, FramesCaptured: 4096},
	}

	_, _ = m.Update(levelMsg(other))
	if m.peakDB != 0 || m.overflow {
		t.Fatalf("other recording update affected state: peak=%f overflow=%t", m.peakDB, m.overflow)
	}
	_, _ = m.Update(levelMsg(current))
	if m.peakDB != -8.5 || !m.paused || !m.overflow || m.captureStats.Callbacks != 2 {
		t.Fatalf("current recording update not applied: peak=%f paused=%t overflow=%t stats=%+v", m.peakDB, m.paused, m.overflow, m.captureStats)
	}
}

func TestUpdateConversionProgressAndDone(t *testing.T) {
	m := &model{jobs: jobs.NewTracker()}
	progress := jobs.Update{
		Kind:        jobs.KindConversion,
		ID:          jobs.ID(jobs.KindConversion, "/recordings/a.wav"),
		Path:        "/recordings/a.wav",
		Destination: "/recordings/a.mp3",
		Percent:     37,
		Message:     "Converting",
		Status:      jobs.StatusRunning,
	}

	_, _ = m.Update(jobMsg(progress))
	if m.message != "Converting 37%" {
		t.Fatalf("message = %q, want Converting 37%%", m.message)
	}
	if got, ok := m.jobs.Get(progress.ID); !ok || got.Percent != 37 {
		t.Fatalf("conversion job = %+v, ok=%t, want 37%%", got, ok)
	}

	progress.Status = jobs.StatusDone
	progress.Percent = 100
	progress.Message = "Saved"
	_, _ = m.Update(jobMsg(progress))
	if _, ok := m.jobs.Get(progress.ID); ok {
		t.Fatal("conversion job was not cleared on done")
	}
	if m.message != "Saved" {
		t.Fatalf("message = %q, want Saved", m.message)
	}
}

func TestHandleMainSpaceTogglesPause(t *testing.T) {
	session := &testSession{}
	m := &model{recording: true, session: session}

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeySpace})
	if cmd != nil {
		t.Fatal("pause toggle returned command")
	}
	if !m.paused || !session.paused || m.message != "Paused" {
		t.Fatalf("pause state = model:%t session:%t message:%q", m.paused, session.paused, m.message)
	}

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})
	if m.paused || session.paused || m.message != "Recording" {
		t.Fatalf("resume state = model:%t session:%t message:%q", m.paused, session.paused, m.message)
	}
}

func TestHandleMainEscMarksFinalizingOnce(t *testing.T) {
	m := &model{recording: true, session: &testSession{}, workflow: recording.Workflow{}}

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("first esc did not return stop command")
	}
	if !m.stopRequested || m.message != "Finalizing WAV" {
		t.Fatalf("stop state = requested:%t message:%q", m.stopRequested, m.message)
	}

	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd != nil {
		t.Fatal("second esc returned another stop command")
	}
}

func TestHandleMainXStopsRecording(t *testing.T) {
	m := &model{recording: true, session: &testSession{}, workflow: recording.Workflow{}}

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	if cmd == nil {
		t.Fatal("x did not return stop command")
	}
	if !m.stopRequested || m.message != "Finalizing WAV" {
		t.Fatalf("stop state = requested:%t message:%q", m.stopRequested, m.message)
	}
}

func TestRenameDestination(t *testing.T) {
	oldPath := filepath.Join("/recordings", "old.mp3")
	got, err := renameDestination(oldPath, "new")
	if err != nil {
		t.Fatalf("renameDestination: %v", err)
	}
	if got != filepath.Join("/recordings", "new.mp3") {
		t.Fatalf("renameDestination = %q", got)
	}

	got, err = renameDestination(oldPath, "new.wav")
	if err != nil {
		t.Fatalf("renameDestination explicit ext: %v", err)
	}
	if got != filepath.Join("/recordings", "new.wav") {
		t.Fatalf("renameDestination explicit ext = %q", got)
	}
}

func TestValidateRenameInput(t *testing.T) {
	for _, name := range []string{"", "   ", ".", "..", ".mp3", "nested/name", "../name"} {
		if err := validateRenameInput(name); err == nil {
			t.Fatalf("validateRenameInput(%q) = nil, want error", name)
		}
	}
	for _, name := range []string{"clip", "clip.mp3", "clip.final.wav"} {
		if err := validateRenameInput(name); err != nil {
			t.Fatalf("validateRenameInput(%q) = %v, want nil", name, err)
		}
	}
}

func TestFriendlyRenameError(t *testing.T) {
	err := friendlyRenameError(storage.ErrDestinationExists)
	if err == nil || err.Error() != "A recording with that name already exists" {
		t.Fatalf("friendlyRenameError = %v", err)
	}
	other := errors.New("other")
	if friendlyRenameError(other) != other {
		t.Fatal("friendlyRenameError did not preserve non-destination error")
	}
}

func TestRenameResultSuccessUpdatesMessageAndScreen(t *testing.T) {
	m := &model{screen: screenRename, editing: "old", editBuffer: []rune("new")}

	_, _ = m.Update(renameResultMsg{
		oldPath: "/recordings/old.mp3",
		newPath: "/recordings/new.mp3",
	})

	if m.screen != screenMain {
		t.Fatalf("screen = %v, want screenMain", m.screen)
	}
	if m.editing != "" || m.editBuffer != nil {
		t.Fatalf("editing state not cleared: editing=%q buffer=%q", m.editing, string(m.editBuffer))
	}
	if m.message != "✓ Renamed to new.mp3" {
		t.Fatalf("message = %q, want rename confirmation", m.message)
	}
}

func TestRenameResultErrorStaysInRenameView(t *testing.T) {
	m := &model{screen: screenRename, editing: "old", editBuffer: []rune("bad")}

	_, _ = m.Update(renameResultMsg{
		oldPath: "/recordings/old.mp3",
		err:     errors.New("rename failed"),
	})

	if m.screen != screenRename {
		t.Fatalf("screen = %v, want screenRename", m.screen)
	}
	if m.message != "rename failed" {
		t.Fatalf("message = %q, want rename failed", m.message)
	}
}

func TestHandleRenameRejectsInvalidName(t *testing.T) {
	m := &model{
		screen:     screenRename,
		selected:   0,
		records:    []storage.Recording{{Path: "/recordings/old.mp3", Name: "old.mp3"}},
		editBuffer: []rune(".mp3"),
	}

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Fatal("invalid rename returned command")
	}
	if m.screen != screenRename {
		t.Fatalf("screen = %v, want screenRename", m.screen)
	}
	if m.message != "Name must include filename text" {
		t.Fatalf("message = %q", m.message)
	}
}

func TestOpenRenameClearsStaleMessage(t *testing.T) {
	m := &model{
		screen:   screenMain,
		message:  "Ready",
		selected: 0,
		records:  []storage.Recording{{Path: "/recordings/old.mp3", Name: "old.mp3"}},
	}

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})

	if m.screen != screenRename {
		t.Fatalf("screen = %v, want screenRename", m.screen)
	}
	if m.message != "" {
		t.Fatalf("message = %q, want cleared", m.message)
	}
}

func TestOpenDeleteClearsStaleMessage(t *testing.T) {
	m := &model{
		screen:   screenMain,
		message:  "Ready",
		selected: 0,
		records:  []storage.Recording{{Path: "/recordings/old.mp3", Name: "old.mp3"}},
	}

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})

	if m.screen != screenDeleteConfirm {
		t.Fatalf("screen = %v, want screenDeleteConfirm", m.screen)
	}
	if m.deleteTarget != "/recordings/old.mp3" {
		t.Fatalf("deleteTarget = %q", m.deleteTarget)
	}
	if m.message != "" {
		t.Fatalf("message = %q, want cleared", m.message)
	}
}

func TestDeleteCancelClearsTarget(t *testing.T) {
	m := &model{screen: screenDeleteConfirm, deleteTarget: "/recordings/old.mp3"}

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	if cmd != nil {
		t.Fatal("cancel delete returned command")
	}
	if m.screen != screenMain || m.deleteTarget != "" {
		t.Fatalf("cancel state = screen:%v target:%q", m.screen, m.deleteTarget)
	}
}

func TestDeleteResultSuccessUpdatesMessageAndScreen(t *testing.T) {
	m := &model{screen: screenDeleteConfirm, deleteTarget: "/recordings/old.mp3"}

	_, _ = m.Update(deleteResultMsg{path: "/recordings/old.mp3"})

	if m.screen != screenMain {
		t.Fatalf("screen = %v, want screenMain", m.screen)
	}
	if m.deleteTarget != "" {
		t.Fatalf("deleteTarget = %q, want cleared", m.deleteTarget)
	}
	if m.message != "✓ Deleted old.mp3" {
		t.Fatalf("message = %q, want delete confirmation", m.message)
	}
}

func TestDeleteResultErrorStaysInDeleteView(t *testing.T) {
	m := &model{screen: screenDeleteConfirm, deleteTarget: "/recordings/old.mp3"}

	_, _ = m.Update(deleteResultMsg{path: "/recordings/old.mp3", err: errors.New("delete failed")})

	if m.screen != screenDeleteConfirm {
		t.Fatalf("screen = %v, want screenDeleteConfirm", m.screen)
	}
	if m.deleteTarget != "/recordings/old.mp3" {
		t.Fatalf("deleteTarget = %q", m.deleteTarget)
	}
	if m.message != "delete failed" {
		t.Fatalf("message = %q, want delete failed", m.message)
	}
}

func TestOpenAndCopyResultMessages(t *testing.T) {
	m := &model{}

	_, _ = m.Update(actionResultMsg{actionID: "open", path: "/recordings/a.mp3", message: openSuccessMessage("/recordings/a.mp3")})
	if m.message != "Opened a.mp3" {
		t.Fatalf("open message = %q", m.message)
	}

	_, _ = m.Update(actionResultMsg{actionID: "reveal", path: "/recordings/a.mp3", message: revealSuccessMessage("/recordings/a.mp3")})
	if m.message != "Revealed a.mp3" {
		t.Fatalf("reveal message = %q", m.message)
	}

	_, _ = m.Update(actionResultMsg{actionID: "copy-path", path: "/recordings/a.mp3", message: copySuccessMessage("/recordings/a.mp3", false)})
	if m.message != "Copied path" {
		t.Fatalf("copy message = %q", m.message)
	}

	_, _ = m.Update(actionResultMsg{actionID: "copy-file", path: "/recordings/a.mp3", message: copySuccessMessage("/recordings/a.mp3", true)})
	if m.message != "Copied file a.mp3" {
		t.Fatalf("copy file message = %q", m.message)
	}
}

func TestOpenAndCopyResultErrors(t *testing.T) {
	m := &model{}

	_, _ = m.Update(actionResultMsg{actionID: "open", path: "/recordings/a.mp3", err: errors.New("open failed")})
	if m.message != "open failed" {
		t.Fatalf("open error message = %q", m.message)
	}

	_, _ = m.Update(actionResultMsg{actionID: "reveal", path: "/recordings/a.mp3", err: errors.New("reveal failed")})
	if m.message != "reveal failed" {
		t.Fatalf("reveal error message = %q", m.message)
	}

	_, _ = m.Update(actionResultMsg{actionID: "copy-path", path: "/recordings/a.mp3", err: errors.New("copy failed")})
	if m.message != "copy failed" {
		t.Fatalf("copy error message = %q", m.message)
	}
}

func TestHandleMainRevealDispatchesCommand(t *testing.T) {
	runner := &testCommandRunner{}
	m := &model{
		records: []storage.Recording{{Path: "/recordings/a.mp3"}},
		platform: platform.OS{
			GOOS:   "linux",
			Runner: runner,
		},
	}

	_, cmd := m.handleMainKey("r")
	if cmd == nil {
		t.Fatal("expected reveal command")
	}
	msg := cmd()
	result, ok := msg.(actionResultMsg)
	if !ok {
		t.Fatalf("message = %T, want actionResultMsg", msg)
	}
	if result.actionID != "reveal" || result.err != nil {
		t.Fatalf("reveal result = %+v", result)
	}
	if runner.startedName != "xdg-open" || len(runner.startedArgs) != 1 || runner.startedArgs[0] != "/recordings" {
		t.Fatalf("started = %q %#v", runner.startedName, runner.startedArgs)
	}
}

type testSession struct {
	info     recording.SessionInfo
	duration time.Duration
	paused   bool
	stopped  bool
}

var errTestStop = testError("stop failed")

type testError string

func (e testError) Error() string {
	return string(e)
}

func (s *testSession) Stop() error {
	s.stopped = true
	return nil
}

func (s *testSession) Duration() time.Duration {
	return s.duration
}

func (s *testSession) TogglePause() bool {
	s.paused = !s.paused
	return s.paused
}

func (s *testSession) Info() recording.SessionInfo {
	return s.info
}

type testCommandRunner struct {
	startedName string
	startedArgs []string
}

func (r *testCommandRunner) Start(name string, args ...string) error {
	r.startedName = name
	r.startedArgs = append([]string(nil), args...)
	return nil
}

func (r *testCommandRunner) Run(name string, args ...string) error {
	return nil
}

func (r *testCommandRunner) RunWithInput(input string, name string, args ...string) error {
	return nil
}
