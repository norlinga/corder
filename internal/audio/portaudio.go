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
	HostAPIName            string
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
	Stats         CaptureStats
}

type CaptureStats struct {
	DeviceName       string
	HostAPIName      string
	SampleRate       int
	Channels         int
	FramesPerBuffer  int
	BufferCapacity   int
	Callbacks        int64
	PortOverflow     int64
	DroppedBuffers   int64
	MaxQueuedBuffers int
	FramesCaptured   int64
}

func (s CaptureStats) HasIssues() bool {
	return s.PortOverflow > 0 || s.DroppedBuffers > 0
}

func (s CaptureStats) QueueSummary() string {
	return fmt.Sprintf("%d/%d", s.MaxQueuedBuffers, s.BufferCapacity)
}

type Session struct {
	Path       string
	MetaPath   string
	SampleRate int
	Channels   int
	StartedAt  time.Time
	writer     *wavWriter
	stream     *portaudio.Stream
	sampleCh   chan []float32
	freeCh     chan []float32
	writerDone chan struct{}
	writerErr  error
	mu         sync.Mutex
	paused     bool
	stopped    bool
	closed     bool
	lastLevel  time.Time
	frames     int64
	stats      CaptureStats
	updates    chan LevelUpdate
	doneCh     chan struct{}
}

const (
	captureFramesPerBuffer = 4096
	captureBufferCount     = 128
)

type Backend struct {
	mu          sync.Mutex
	initialized bool
	lastLog     string
}

type SessionInfo struct {
	Path       string
	StartedAt  time.Time
	SampleRate int
	Channels   int
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
			HostAPIName:            hostAPIName(d),
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
				if isRawALSADevice(devs[i]) {
					if preferred := preferredInputDevice(devs); preferred != nil {
						return preferred, nil
					}
				}
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
				if isRawALSADevice(devs[i]) {
					if preferred := preferredInputDevice(devs); preferred != nil {
						return preferred, nil
					}
				}
				return &devs[i], nil
			}
		}
	}
	if len(devs) == 0 {
		return nil, errors.New("no input devices available")
	}
	if preferred := preferredInputDevice(devs); preferred != nil {
		return preferred, nil
	}
	return &devs[0], nil
}

func (b *Backend) StartCapture(device Device, path string, updates chan LevelUpdate) (*Session, error) {
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
	params := portaudio.HighLatencyParameters(paDevice, nil)
	params.Input.Channels = channels
	params.SampleRate = float64(sampleRate)
	params.FramesPerBuffer = captureFramesPerBuffer
	sess := &Session{
		Path:       path,
		MetaPath:   storage.MetaPathFor(path),
		SampleRate: sampleRate,
		Channels:   channels,
		StartedAt:  time.Now(),
		writer:     writer,
		sampleCh:   make(chan []float32, captureBufferCount),
		freeCh:     make(chan []float32, captureBufferCount),
		writerDone: make(chan struct{}),
		updates:    updates,
		doneCh:     make(chan struct{}),
		stats: CaptureStats{
			DeviceName:      device.Name,
			HostAPIName:     device.HostAPIName,
			SampleRate:      sampleRate,
			Channels:        channels,
			FramesPerBuffer: captureFramesPerBuffer,
			BufferCapacity:  captureBufferCount,
		},
	}
	for i := 0; i < captureBufferCount; i++ {
		sess.freeCh <- make([]float32, channels*captureFramesPerBuffer)
	}
	var stream *portaudio.Stream
	log := captureNativeStderr(func() {
		stream, err = portaudio.OpenStream(params, sess.captureCallback)
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
	sess.stream = stream
	go sess.writeLoop()
	if err := stream.Start(); err != nil {
		_ = stream.Close()
		close(sess.sampleCh)
		<-sess.writerDone
		_ = writer.Close()
		return nil, err
	}
	return sess, nil
}

func (s *Session) writeLoop() {
	defer close(s.writerDone)
	for samples := range s.sampleCh {
		if err := s.writer.WriteFloat32Interleaved(samples); err != nil {
			s.mu.Lock()
			s.writerErr = err
			s.mu.Unlock()
			return
		}
		select {
		case s.freeCh <- samples:
		default:
		}
	}
}

func (s *Session) captureCallback(in []float32, _ portaudio.StreamCallbackTimeInfo, flags portaudio.StreamCallbackFlags) {
	s.mu.Lock()
	stopped := s.stopped || s.closed
	paused := s.paused
	s.stats.Callbacks++
	if flags&portaudio.InputOverflow != 0 {
		s.stats.PortOverflow++
	}
	shouldReport := time.Since(s.lastLevel) >= 100*time.Millisecond
	if shouldReport {
		s.lastLevel = time.Now()
	}
	s.mu.Unlock()
	if stopped {
		return
	}
	peak := 0.0
	clipping := false
	overflow := flags&portaudio.InputOverflow != 0
	if !paused {
		var samples []float32
		select {
		case samples = <-s.freeCh:
		default:
			overflow = true
			s.noteDroppedBuffer(len(s.sampleCh))
		}
		if samples != nil {
			if cap(samples) < len(in) {
				overflow = true
				s.noteDroppedBuffer(len(s.sampleCh))
				select {
				case s.freeCh <- samples:
				default:
				}
			} else {
				samples = samples[:len(in)]
				copy(samples, in)
				select {
				case s.sampleCh <- samples:
					s.mu.Lock()
					s.frames += int64(len(samples) / s.Channels)
					s.stats.FramesCaptured = s.frames
					if queued := len(s.sampleCh); queued > s.stats.MaxQueuedBuffers {
						s.stats.MaxQueuedBuffers = queued
					}
					s.mu.Unlock()
				default:
					overflow = true
					s.noteDroppedBuffer(len(s.sampleCh))
					select {
					case s.freeCh <- samples:
					default:
					}
				}
				if shouldReport {
					for _, sample := range in {
						v := math.Abs(float64(sample))
						if v > peak {
							peak = v
						}
						if v >= 1.0 {
							clipping = true
						}
					}
				}
			}
		}
	}
	if !shouldReport {
		return
	}
	peakDB := -120.0
	if peak > 0 {
		peakDB = 20 * math.Log10(peak)
	}
	duration := s.Duration()
	stats := s.Stats()
	select {
	case s.updates <- LevelUpdate{
		RecordingPath: s.Path,
		Duration:      duration,
		PeakDB:        peakDB,
		Clipping:      clipping,
		Paused:        paused,
		Overflow:      overflow,
		Stats:         stats,
	}:
	default:
	}
}

func (s *Session) noteDroppedBuffer(queueDepth int) {
	s.mu.Lock()
	s.stats.DroppedBuffers++
	if queueDepth > s.stats.MaxQueuedBuffers {
		s.stats.MaxQueuedBuffers = queueDepth
	}
	s.mu.Unlock()
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
	if s.SampleRate <= 0 {
		return 0
	}
	return time.Duration(float64(s.frames) / float64(s.SampleRate) * float64(time.Second))
}

func (s *Session) Info() SessionInfo {
	return SessionInfo{
		Path:       s.Path,
		StartedAt:  s.StartedAt,
		SampleRate: s.SampleRate,
		Channels:   s.Channels,
	}
}

func (s *Session) Stats() CaptureStats {
	s.mu.Lock()
	defer s.mu.Unlock()
	stats := s.stats
	stats.FramesCaptured = s.frames
	return stats
}

func (s *Session) Stop() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		<-s.doneCh
		return nil
	}
	s.stopped = true
	stream := s.stream
	s.mu.Unlock()

	if stream != nil {
		_ = stream.Abort()
		_ = stream.Close()
	}

	s.mu.Lock()
	s.closed = true
	close(s.sampleCh)
	s.mu.Unlock()

	<-s.writerDone

	s.mu.Lock()
	err := s.writerErr
	s.mu.Unlock()
	if closeErr := s.writer.Close(); err == nil {
		err = closeErr
	}
	close(s.updates)
	close(s.doneCh)
	return err
}

func (d Device) String() string {
	return fmt.Sprintf("%s (%d ch)", d.Name, d.MaxInputChannels)
}

func preferredInputDevice(devs []Device) *Device {
	for _, preferred := range []string{"default", "pipewire", "pulse"} {
		for i := range devs {
			if strings.EqualFold(devs[i].Name, preferred) {
				return &devs[i]
			}
		}
	}
	for i := range devs {
		if !isRawALSADevice(devs[i]) {
			return &devs[i]
		}
	}
	return nil
}

func isRawALSADevice(device Device) bool {
	name := strings.ToLower(device.Name)
	host := strings.ToLower(device.HostAPIName)
	return strings.Contains(host, "alsa") && (strings.Contains(name, "(hw:") || strings.HasPrefix(name, "hw:"))
}

func hostAPIName(device *portaudio.DeviceInfo) string {
	if device == nil || device.HostApi == nil {
		return ""
	}
	return device.HostApi.Name
}
