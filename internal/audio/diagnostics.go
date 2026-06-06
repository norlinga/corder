package audio

import (
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/gordonklaus/portaudio"
)

type Diagnostics struct {
	Version           string
	DefaultInputName  string
	DefaultOutputName string
	HostAPIs          []string
	Devices           []Device
	BackendLog        string
}

type ProbeResult struct {
	DeviceName      string
	SampleRate      float64
	Channels        int
	FramesPerBuffer int
	Duration        time.Duration
	Reads           int
	Peak            float64
	MeanAbs         float64
	NonZero         bool
	Err             error
}

func (b *Backend) Diagnostics() (Diagnostics, error) {
	if err := b.Init(); err != nil {
		return Diagnostics{}, err
	}
	var devs []*portaudio.DeviceInfo
	var err error
	log := captureNativeStderr(func() {
		devs, err = portaudio.Devices()
	})
	if err != nil {
		return Diagnostics{}, err
	}
	var apiInfo []*portaudio.HostApiInfo
	log += captureNativeStderr(func() {
		apiInfo, err = portaudio.HostApis()
	})
	if err != nil {
		return Diagnostics{}, err
	}
	info := Diagnostics{
		Version:    portaudio.VersionText(),
		Devices:    make([]Device, 0, len(devs)),
		BackendLog: log,
	}
	for _, api := range apiInfo {
		info.HostAPIs = append(info.HostAPIs, fmt.Sprintf("%s: %s", api.Type.String(), api.Name))
	}
	var input *portaudio.DeviceInfo
	log = captureNativeStderr(func() {
		input, err = portaudio.DefaultInputDevice()
	})
	info.BackendLog += log
	if err == nil && input != nil {
		info.DefaultInputName = input.Name
	}
	var output *portaudio.DeviceInfo
	log = captureNativeStderr(func() {
		output, err = portaudio.DefaultOutputDevice()
	})
	info.BackendLog += log
	if err == nil && output != nil {
		info.DefaultOutputName = output.Name
	}
	for _, d := range devs {
		if d.MaxInputChannels < 1 {
			continue
		}
		info.Devices = append(info.Devices, Device{
			Index:                  d.Index,
			Name:                   d.Name,
			MaxInputChannels:       d.MaxInputChannels,
			DefaultSampleRate:      d.DefaultSampleRate,
			DefaultLowInputLatency: d.DefaultLowInputLatency,
			pa:                     d,
		})
	}
	return info, nil
}

func (b *Backend) Probe(deviceName string, reads int) (ProbeResult, error) {
	if reads <= 0 {
		reads = 8
	}
	device, err := b.ResolveDevice(deviceName)
	if err != nil {
		return ProbeResult{}, err
	}
	if device.pa == nil {
		return ProbeResult{}, fmt.Errorf("device %q is not bound to PortAudio metadata", device.Name)
	}
	sampleRate := int(math.Round(device.pa.DefaultSampleRate))
	if sampleRate <= 0 {
		sampleRate = 44100
	}
	channels := 1
	params := portaudio.LowLatencyParameters(device.pa, nil)
	params.Input.Channels = channels
	params.SampleRate = float64(sampleRate)
	params.FramesPerBuffer = 2048
	result := ProbeResult{
		DeviceName:      device.Name,
		SampleRate:      float64(sampleRate),
		Channels:        channels,
		FramesPerBuffer: 2048,
	}
	var mu sync.Mutex
	var stream *portaudio.Stream
	_ = captureNativeStderr(func() {
		stream, err = portaudio.OpenStream(params, func(in []float32) {
			mu.Lock()
			defer mu.Unlock()
			result.Reads++
			for _, sample := range in {
				v := math.Abs(float64(sample))
				result.MeanAbs += v
				if v > result.Peak {
					result.Peak = v
				}
			}
		})
	})
	if err != nil {
		return ProbeResult{}, err
	}
	defer stream.Close()
	if err := stream.Start(); err != nil {
		return ProbeResult{}, err
	}
	defer stream.Stop()
	start := time.Now()
	time.Sleep(time.Duration(reads) * 150 * time.Millisecond)
	_ = stream.Abort()
	mu.Lock()
	result.Duration = time.Since(start)
	if result.Reads > 0 {
		result.MeanAbs = result.MeanAbs / float64(result.Reads*channels*2048)
	}
	result.NonZero = result.Peak > 0
	mu.Unlock()
	return result, nil
}

func (d Diagnostics) Format() string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("PortAudio: %s\n", d.Version))
	b.WriteString(fmt.Sprintf("Default input: %s\n", fallbackString(d.DefaultInputName)))
	b.WriteString(fmt.Sprintf("Default output: %s\n", fallbackString(d.DefaultOutputName)))
	b.WriteString("Host APIs:\n")
	for _, api := range d.HostAPIs {
		b.WriteString("  - ")
		b.WriteString(api)
		b.WriteString("\n")
	}
	b.WriteString("Input devices:\n")
	for _, dev := range d.Devices {
		b.WriteString(fmt.Sprintf("  - [%d] %s (%d ch, %.0f Hz)\n", dev.Index, dev.Name, dev.MaxInputChannels, dev.DefaultSampleRate))
	}
	if d.BackendLog != "" {
		b.WriteString("Backend probe log: captured\n")
	}
	return b.String()
}

func (p ProbeResult) Format() string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Selected device: %s\n", fallbackString(p.DeviceName)))
	b.WriteString(fmt.Sprintf("Sample rate: %.0f Hz\n", p.SampleRate))
	b.WriteString(fmt.Sprintf("Channels: %d\n", p.Channels))
	b.WriteString(fmt.Sprintf("Frames/buffer: %d\n", p.FramesPerBuffer))
	b.WriteString(fmt.Sprintf("Reads: %d\n", p.Reads))
	b.WriteString(fmt.Sprintf("Peak: %.6f\n", p.Peak))
	b.WriteString(fmt.Sprintf("Mean abs: %.6f\n", p.MeanAbs))
	b.WriteString(fmt.Sprintf("Non-zero audio: %t\n", p.NonZero))
	if p.Err != nil {
		b.WriteString(fmt.Sprintf("Error: %v\n", p.Err))
	}
	return b.String()
}

func fallbackString(s string) string {
	if s == "" {
		return "(none)"
	}
	return s
}
