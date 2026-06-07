package app

import (
	"time"

	"corder/internal/audio"
	"corder/internal/recording"
)

type audioRecorder struct {
	backend *audio.Backend
}

func (r audioRecorder) ResolveDevice(name string) (*audio.Device, error) {
	return r.backend.ResolveDevice(name)
}

func (r audioRecorder) StartRecording(device audio.Device, path string, updates chan audio.LevelUpdate) (recording.Session, error) {
	session, err := r.backend.StartCapture(device, path, updates)
	if err != nil {
		return nil, err
	}
	return audioSession{session: session}, nil
}

type audioSession struct {
	session *audio.Session
}

func (s audioSession) Stop() error {
	return s.session.Stop()
}

func (s audioSession) Duration() time.Duration {
	return s.session.Duration()
}

func (s audioSession) TogglePause() bool {
	return s.session.TogglePause()
}

func (s audioSession) Info() recording.SessionInfo {
	info := s.session.Info()
	return recording.SessionInfo{
		Path:       info.Path,
		StartedAt:  info.StartedAt,
		SampleRate: info.SampleRate,
		Channels:   info.Channels,
	}
}
