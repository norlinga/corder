package recording

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"corder/internal/audio"
	"corder/internal/settings"
	"corder/internal/storage"
)

func TestWorkflowStartWritesRecordingMeta(t *testing.T) {
	now := time.Date(2026, 6, 6, 12, 34, 56, 0, time.UTC)
	store := &fakeStore{}
	session := &fakeSession{
		info: SessionInfo{
			StartedAt:  now,
			SampleRate: 48000,
			Channels:   1,
		},
	}
	recorder := &fakeRecorder{session: session}
	w := Workflow{
		Recorder:  recorder,
		Converter: fakeConverter{},
		Store:     store,
		Clock:     fakeClock{now: now},
	}

	result, err := w.Start(StartRequest{
		Config: settings.Config{
			RecordingDir:          "/recordings",
			InputDeviceName:       "default",
			MP3BitrateKbps:        128,
			RetainWAVAfterConvert: false,
		},
		Updates: make(chan audio.LevelUpdate),
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	wantPath := filepath.Join("/recordings", "2026-06-06_123456.wav")
	if result.Path != wantPath {
		t.Fatalf("Path = %q, want %q", result.Path, wantPath)
	}
	if recorder.startedPath != wantPath {
		t.Fatalf("startedPath = %q, want %q", recorder.startedPath, wantPath)
	}
	meta := store.metaFor(wantPath)
	if meta.Status != storage.StatusRecording || meta.SampleRate != 48000 || meta.Channels != 1 {
		t.Fatalf("meta = %+v, want recording/48000/1", meta)
	}
}

func TestWorkflowStartStopsSessionWhenInitialMetaWriteFails(t *testing.T) {
	now := time.Date(2026, 6, 6, 12, 34, 56, 0, time.UTC)
	session := &fakeSession{info: SessionInfo{StartedAt: now, SampleRate: 44100, Channels: 1}}
	wantErr := errors.New("write failed")
	w := Workflow{
		Recorder:  &fakeRecorder{session: session},
		Converter: fakeConverter{},
		Store:     &fakeStore{writeErr: wantErr},
		Clock:     fakeClock{now: now},
	}

	_, err := w.Start(StartRequest{Config: settings.Config{RecordingDir: "/recordings"}})
	if !errors.Is(err, wantErr) {
		t.Fatalf("Start error = %v, want %v", err, wantErr)
	}
	if !session.stopped {
		t.Fatal("session was not stopped after metadata write failure")
	}
}

func TestWorkflowStopNoAudioWritesEmptyMeta(t *testing.T) {
	startedAt := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)
	session := &fakeSession{
		duration: 0,
		info: SessionInfo{
			Path:       "/recordings/a.wav",
			StartedAt:  startedAt,
			SampleRate: 44100,
			Channels:   1,
		},
	}
	store := &fakeStore{}
	w := Workflow{Converter: fakeConverter{}, Store: store}

	result, err := w.Stop(StopRequest{
		Session: session,
		Config:  settings.Config{MP3BitrateKbps: 128},
	})
	if !errors.Is(err, ErrNoAudioCaptured) {
		t.Fatalf("Stop error = %v, want ErrNoAudioCaptured", err)
	}
	if result.Queue {
		t.Fatal("Queue = true for empty recording")
	}
	meta := store.metaFor("/recordings/a.wav")
	if meta.Status != storage.StatusEmpty || meta.DurationSeconds != 0 {
		t.Fatalf("meta = %+v, want empty/0", meta)
	}
}

func TestWorkflowStopFFmpegMissingLeavesWAVReady(t *testing.T) {
	wantErr := errors.New("ffmpeg missing")
	session := &fakeSession{
		duration: 2 * time.Second,
		info: SessionInfo{
			Path:       "/recordings/a.wav",
			StartedAt:  time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC),
			SampleRate: 44100,
			Channels:   1,
		},
	}
	store := &fakeStore{}
	w := Workflow{Converter: fakeConverter{checkErr: wantErr}, Store: store}

	result, err := w.Stop(StopRequest{
		Session: session,
		Config:  settings.Config{MP3BitrateKbps: 128},
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("Stop error = %v, want ffmpeg error", err)
	}
	if result.Queue {
		t.Fatal("Queue = true when ffmpeg is missing")
	}
	meta := store.metaFor("/recordings/a.wav")
	if meta.Status != storage.StatusReady || meta.DurationSeconds != 2 {
		t.Fatalf("meta = %+v, want ready/2s", meta)
	}
}

func TestWorkflowStopQueuesConversion(t *testing.T) {
	session := &fakeSession{
		duration: 3 * time.Second,
		info: SessionInfo{
			Path:       "/recordings/a.wav",
			StartedAt:  time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC),
			SampleRate: 44100,
			Channels:   1,
		},
	}
	store := &fakeStore{}
	w := Workflow{Converter: fakeConverter{}, Store: store}

	result, err := w.Stop(StopRequest{
		Session: session,
		Config:  settings.Config{MP3BitrateKbps: 160, RetainWAVAfterConvert: true},
	})
	if err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if !result.Queue {
		t.Fatal("Queue = false, want true")
	}
	if result.Destination != "/recordings/a.mp3" || result.BitrateKbps != 160 || !result.RetainWAV {
		t.Fatalf("result = %+v, want destination/bitrate/retain", result)
	}
	meta := store.metaFor("/recordings/a.wav")
	if meta.Status != storage.StatusConverting || meta.DurationSeconds != 3 {
		t.Fatalf("meta = %+v, want converting/3s", meta)
	}
}

type fakeRecorder struct {
	device      audio.Device
	session     Session
	startedPath string
}

func (r *fakeRecorder) ResolveDevice(string) (*audio.Device, error) {
	if r.device.Name == "" {
		r.device.Name = "default"
	}
	return &r.device, nil
}

func (r *fakeRecorder) StartRecording(_ audio.Device, path string, _ chan audio.LevelUpdate) (Session, error) {
	r.startedPath = path
	if s, ok := r.session.(*fakeSession); ok {
		s.info.Path = path
	}
	return r.session, nil
}

type fakeSession struct {
	info     SessionInfo
	duration time.Duration
	stopped  bool
	paused   bool
}

func (s *fakeSession) Stop() error {
	s.stopped = true
	return nil
}

func (s *fakeSession) Duration() time.Duration {
	return s.duration
}

func (s *fakeSession) TogglePause() bool {
	s.paused = !s.paused
	return s.paused
}

func (s *fakeSession) Info() SessionInfo {
	return s.info
}

type fakeConverter struct {
	checkErr error
}

func (c fakeConverter) Check() error {
	return c.checkErr
}

func (fakeConverter) DestinationFor(source string) string {
	return source[:len(source)-len(filepath.Ext(source))] + ".mp3"
}

type fakeStore struct {
	writeErr error
	metas    map[string]storage.Meta
}

func (s *fakeStore) EnsureDir(string) error {
	return nil
}

func (s *fakeStore) WriteMeta(path string, meta storage.Meta) error {
	if s.writeErr != nil {
		return s.writeErr
	}
	if s.metas == nil {
		s.metas = map[string]storage.Meta{}
	}
	s.metas[path] = meta
	return nil
}

func (s *fakeStore) metaFor(path string) storage.Meta {
	if s.metas == nil {
		return storage.Meta{}
	}
	return s.metas[path]
}

type fakeClock struct {
	now time.Time
}

func (c fakeClock) Now() time.Time {
	return c.now
}
