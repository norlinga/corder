package audio

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/gordonklaus/portaudio"

	"corder/internal/storage"
)

type Device struct {
	Index                  int
	Name                   string
	MaxInputChannels       int
	DefaultSampleRate      float64
	DefaultLowInputLatency time.Duration
	pa                     *portaudio.DeviceInfo
}

type LevelUpdate struct {
	RecordingPath string
	Duration      time.Duration
	PeakDB        float64
	Clipping      bool
	Paused        bool
	Overflow      bool
}

type Session struct {
	Path       string
	MetaPath   string
	SampleRate int
	Channels   int
	StartedAt  time.Time
	writer     *wavWriter
	stream     *portaudio.Stream
	buffer     []float32
	mu         sync.Mutex
	paused     bool
	stopped    bool
	updates    chan LevelUpdate
	stopCh     chan struct{}
	doneCh     chan struct{}
}

type Backend struct {
	mu          sync.Mutex
	initialized bool
	lastLog     string
}

func (b *Backend) Init() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.initialized {
		return nil
	}
	var err error
	b.lastLog = captureNativeStderr(func() {
		err = portaudio.Initialize()
	})
	if err != nil {
		return err
	}
	b.initialized = true
	return nil
}

func (b *Backend) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.initialized {
		return nil
	}
	err := portaudio.Terminate()
	b.initialized = false
	return err
}

func (b *Backend) Devices() ([]Device, error) {
	if err := b.Init(); err != nil {
		return nil, err
	}
	var devs []*portaudio.DeviceInfo
	var err error
	log := captureNativeStderr(func() {
		devs, err = portaudio.Devices()
	})
	if err != nil {
		return nil, err
	}
	if log != "" {
		b.mu.Lock()
		b.lastLog = log
		b.mu.Unlock()
	}
	out := make([]Device, 0, len(devs))
	for _, d := range devs {
		if d.MaxInputChannels < 1 {
			continue
		}
		out = append(out, Device{
			Index:                  d.Index,
			Name:                   d.Name,
			MaxInputChannels:       d.MaxInputChannels,
			DefaultSampleRate:      d.DefaultSampleRate,
			DefaultLowInputLatency: d.DefaultLowInputLatency,
			pa:                     d,
		})
	}
	return out, nil
}

func (b *Backend) ResolveDevice(name string) (*Device, error) {
	devs, err := b.Devices()
	if err != nil {
		return nil, err
	}
	if name != "" {
		for i := range devs {
			if devs[i].Name == name {
				return &devs[i], nil
			}
		}
	}
	var def *portaudio.DeviceInfo
	var defaultErr error
	log := captureNativeStderr(func() {
		def, defaultErr = portaudio.DefaultInputDevice()
	})
	if log != "" {
		b.mu.Lock()
		b.lastLog = log
		b.mu.Unlock()
	}
	if defaultErr == nil && def != nil {
		for i := range devs {
			if devs[i].Index == def.Index {
				return &devs[i], nil
			}
		}
	}
	if len(devs) == 0 {
		return nil, errors.New("no input devices available")
	}
	for _, preferred := range []string{"pipewire", "pulse", "default"} {
		for i := range devs {
			if strings.EqualFold(devs[i].Name, preferred) {
				return &devs[i], nil
			}
		}
	}
	return &devs[0], nil
}

func (b *Backend) StartRecording(device Device, path string, bitrate int, retainWAV bool, updates chan LevelUpdate) (*Session, error) {
	if err := b.Init(); err != nil {
		return nil, err
	}
	paDevice := device.pa
	if paDevice == nil {
		var devs []*portaudio.DeviceInfo
		var err error
		log := captureNativeStderr(func() {
			devs, err = portaudio.Devices()
		})
		if err != nil {
			return nil, err
		}
		if log != "" {
			b.mu.Lock()
			b.lastLog = log
			b.mu.Unlock()
		}
		if device.Index < 0 || device.Index >= len(devs) {
			return nil, errors.New("invalid input device")
		}
		paDevice = devs[device.Index]
	}
	sampleRate := int(math.Round(paDevice.DefaultSampleRate))
	if sampleRate <= 0 {
		sampleRate = 44100
	}
	channels := 1
	if paDevice.MaxInputChannels < 1 {
		channels = 1
	}
	if channels > paDevice.MaxInputChannels {
		channels = paDevice.MaxInputChannels
	}
	if channels < 1 {
		channels = 1
	}
	writer, err := newWAVWriter(path, sampleRate, channels)
	if err != nil {
		return nil, err
	}
	params := portaudio.LowLatencyParameters(paDevice, nil)
	params.Input.Channels = channels
	params.SampleRate = float64(sampleRate)
	params.FramesPerBuffer = 2048
	buffer := make([]float32, channels*2048)
	var stream *portaudio.Stream
	log := captureNativeStderr(func() {
		stream, err = portaudio.OpenStream(params, &buffer)
	})
	if err != nil {
		_ = writer.Close()
		return nil, err
	}
	if log != "" {
		b.mu.Lock()
		b.lastLog = log
		b.mu.Unlock()
	}
	sess := &Session{
		Path:       path,
		MetaPath:   storage.MetaPathFor(path),
		SampleRate: sampleRate,
		Channels:   channels,
		StartedAt:  time.Now(),
		writer:     writer,
		stream:     stream,
		buffer:     buffer,
		updates:    updates,
		stopCh:     make(chan struct{}),
		doneCh:     make(chan struct{}),
	}
	if err := stream.Start(); err != nil {
		_ = stream.Close()
		_ = writer.Close()
		return nil, err
	}
	go sess.run()
	return sess, nil
}

func (s *Session) run() {
	defer close(s.doneCh)
	defer close(s.updates)
	defer func() {
		_ = s.writer.Close()
		_ = s.stream.Stop()
		_ = s.stream.Close()
	}()
	for {
		select {
		case <-s.stopCh:
			return
		default:
		}
		readErr := s.stream.Read()
		overflow := false
		if readErr != nil {
			if readErr == portaudio.InputOverflowed {
				overflow = true
			} else {
				if s.isStopping() {
					return
				}
				return
			}
		}
		s.mu.Lock()
		paused := s.paused
		peak := 0.0
		clipping := false
		if !paused {
			if err := s.writer.WriteFloat32Interleaved(s.buffer); err != nil {
				s.mu.Unlock()
				return
			}
			for _, sample := range s.buffer {
				v := math.Abs(float64(sample))
				if v > peak {
					peak = v
				}
				if v >= 1.0 {
					clipping = true
				}
			}
		}
		duration := time.Duration(s.writer.DurationSeconds() * float64(time.Second))
		s.mu.Unlock()
		peakDB := -120.0
		if peak > 0 {
			peakDB = 20 * math.Log10(peak)
		}
		select {
		case s.updates <- LevelUpdate{
			RecordingPath: s.Path,
			Duration:      duration,
			PeakDB:        peakDB,
			Clipping:      clipping,
			Paused:        paused,
			Overflow:      overflow,
		}:
		default:
		}
	}
}

func (s *Session) isStopping() bool {
	select {
	case <-s.stopCh:
		return true
	default:
		return false
	}
}

func (s *Session) TogglePause() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.paused = !s.paused
	return s.paused
}

func (s *Session) IsPaused() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.paused
}

func (s *Session) Duration() time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.writer == nil {
		return 0
	}
	return time.Duration(s.writer.DurationSeconds() * float64(time.Second))
}

func (s *Session) Stop() error {
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		select {
		case <-s.doneCh:
			return nil
		case <-time.After(3 * time.Second):
			return errors.New("timed out waiting for recorder to stop")
		}
	}
	s.stopped = true
	close(s.stopCh)
	s.mu.Unlock()
	_ = s.stream.Abort()
	select {
	case <-s.doneCh:
		return nil
	case <-time.After(3 * time.Second):
		_ = s.stream.Close()
		return errors.New("timed out waiting for recorder to stop")
	}
}

func (s *Session) UpdateMeta(status string, bitrate int, retainWAV bool) error {
	meta := storage.Meta{
		CreatedAt:             s.StartedAt,
		SampleRate:            float64(s.SampleRate),
		Channels:              s.Channels,
		DurationSeconds:       s.writer.DurationSeconds(),
		Status:                status,
		MP3BitrateKbps:        bitrate,
		RetainWAVAfterConvert: retainWAV,
	}
	return storage.WriteMeta(s.Path, meta)
}

func (s *Session) FinishMeta(status string, bitrate int, retainWAV bool, convertedAt *time.Time, sourceWAV string) error {
	meta := storage.Meta{
		Version:               1,
		CreatedAt:             s.StartedAt,
		SampleRate:            float64(s.SampleRate),
		Channels:              s.Channels,
		DurationSeconds:       s.writer.DurationSeconds(),
		Status:                status,
		MP3BitrateKbps:        bitrate,
		RetainWAVAfterConvert: retainWAV,
		ConvertedAt:           convertedAt,
		SourceWAV:             sourceWAV,
	}
	return storage.WriteMeta(s.Path, meta)
}

func (d Device) String() string {
	return fmt.Sprintf("%s (%d ch)", d.Name, d.MaxInputChannels)
}
