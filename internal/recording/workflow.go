package recording

import (
	"errors"
	"path/filepath"
	"time"

	"corder/internal/audio"
	"corder/internal/conversion"
	"corder/internal/settings"
	"corder/internal/storage"
)

var ErrNoAudioCaptured = errors.New("no audio captured")

type Recorder interface {
	ResolveDevice(name string) (*audio.Device, error)
	StartRecording(device audio.Device, path string, updates chan audio.LevelUpdate) (Session, error)
}

type Session interface {
	Stop() error
	Duration() time.Duration
	TogglePause() bool
	Info() SessionInfo
}

type SessionInfo struct {
	Path       string
	StartedAt  time.Time
	SampleRate int
	Channels   int
}

type Converter interface {
	Check() error
	DestinationFor(source string) string
}

type Store interface {
	EnsureDir(dir string) error
	WriteMeta(audioPath string, meta storage.Meta) error
}

type Clock interface {
	Now() time.Time
}

type Workflow struct {
	Recorder  Recorder
	Converter Converter
	Store     Store
	Clock     Clock
}

type StartRequest struct {
	Config  settings.Config
	Updates chan audio.LevelUpdate
}

type StartResult struct {
	Session    Session
	Path       string
	DeviceName string
}

type StopRequest struct {
	Session Session
	Config  settings.Config
}

type StopResult struct {
	Path        string
	Destination string
	Duration    time.Duration
	Queue       bool
	StartedAt   time.Time
	BitrateKbps int
	RetainWAV   bool
}

func (w Workflow) Start(req StartRequest) (StartResult, error) {
	if err := w.Store.EnsureDir(req.Config.RecordingDir); err != nil {
		return StartResult{}, err
	}
	device, err := w.Recorder.ResolveDevice(req.Config.InputDeviceName)
	if err != nil {
		return StartResult{}, err
	}
	now := w.Clock.Now()
	path := filepath.Join(req.Config.RecordingDir, now.Format("2006-01-02_150405")+".wav")
	session, err := w.Recorder.StartRecording(*device, path, req.Updates)
	if err != nil {
		return StartResult{}, err
	}
	info := session.Info()
	meta := storage.Meta{
		CreatedAt:             info.StartedAt,
		SampleRate:            float64(info.SampleRate),
		Channels:              info.Channels,
		DurationSeconds:       0,
		Status:                storage.StatusRecording,
		MP3BitrateKbps:        req.Config.MP3BitrateKbps,
		RetainWAVAfterConvert: req.Config.RetainWAVAfterConvert,
	}
	if err := w.Store.WriteMeta(path, meta); err != nil {
		_ = session.Stop()
		return StartResult{Path: path}, err
	}
	return StartResult{Session: session, Path: path, DeviceName: device.Name}, nil
}

func (w Workflow) Stop(req StopRequest) (StopResult, error) {
	if req.Session == nil {
		return StopResult{}, nil
	}
	info := req.Session.Info()
	result := StopResult{
		Path:        info.Path,
		Destination: w.Converter.DestinationFor(info.Path),
		StartedAt:   info.StartedAt,
		BitrateKbps: req.Config.MP3BitrateKbps,
		RetainWAV:   req.Config.RetainWAVAfterConvert,
	}
	if err := req.Session.Stop(); err != nil {
		return result, err
	}
	duration := req.Session.Duration()
	result.Duration = duration
	if duration <= 0 {
		if err := w.Store.WriteMeta(info.Path, storage.Meta{
			CreatedAt:             info.StartedAt,
			SampleRate:            float64(info.SampleRate),
			Channels:              info.Channels,
			DurationSeconds:       0,
			Status:                storage.StatusEmpty,
			MP3BitrateKbps:        req.Config.MP3BitrateKbps,
			RetainWAVAfterConvert: req.Config.RetainWAVAfterConvert,
			SourceWAV:             info.Path,
		}); err != nil {
			return result, err
		}
		return result, ErrNoAudioCaptured
	}
	if err := w.Store.WriteMeta(info.Path, storage.Meta{
		CreatedAt:             info.StartedAt,
		SampleRate:            float64(info.SampleRate),
		Channels:              info.Channels,
		DurationSeconds:       duration.Seconds(),
		Status:                storage.StatusConverting,
		MP3BitrateKbps:        req.Config.MP3BitrateKbps,
		RetainWAVAfterConvert: req.Config.RetainWAVAfterConvert,
		SourceWAV:             info.Path,
	}); err != nil {
		return result, err
	}
	if err := w.Converter.Check(); err != nil {
		if metaErr := w.Store.WriteMeta(info.Path, storage.Meta{
			CreatedAt:             info.StartedAt,
			SampleRate:            float64(info.SampleRate),
			Channels:              info.Channels,
			DurationSeconds:       duration.Seconds(),
			Status:                storage.StatusReady,
			MP3BitrateKbps:        req.Config.MP3BitrateKbps,
			RetainWAVAfterConvert: req.Config.RetainWAVAfterConvert,
			SourceWAV:             info.Path,
		}); metaErr != nil {
			err = errors.Join(err, metaErr)
		}
		return result, err
	}
	result.Queue = true
	return result, nil
}

type SystemStore struct{}

func (SystemStore) EnsureDir(dir string) error {
	return storage.EnsureDir(dir)
}

func (SystemStore) WriteMeta(audioPath string, meta storage.Meta) error {
	return storage.WriteMeta(audioPath, meta)
}

type SystemConverter struct{}

func (SystemConverter) Check() error {
	return conversion.CheckFFmpeg()
}

func (SystemConverter) DestinationFor(source string) string {
	return conversion.DestFor(source)
}

type SystemClock struct{}

func (SystemClock) Now() time.Time {
	return time.Now()
}
