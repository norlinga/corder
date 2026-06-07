package audio

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
)

type wavWriter struct {
	f          *os.File
	sampleRate int
	channels   int
	dataBytes  int64
	buf        []byte
}

func newWAVWriter(path string, sampleRate int, channels int) (*wavWriter, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	w := &wavWriter{f: f, sampleRate: sampleRate, channels: channels}
	if err := w.writeHeader(); err != nil {
		_ = f.Close()
		return nil, err
	}
	return w, nil
}

func (w *wavWriter) writeHeader() error {
	header := make([]byte, 44)
	copy(header[0:4], []byte("RIFF"))
	copy(header[8:12], []byte("WAVE"))
	copy(header[12:16], []byte("fmt "))
	binary.LittleEndian.PutUint32(header[16:20], 16)
	binary.LittleEndian.PutUint16(header[20:22], 1)
	binary.LittleEndian.PutUint16(header[22:24], uint16(w.channels))
	binary.LittleEndian.PutUint32(header[24:28], uint32(w.sampleRate))
	byteRate := uint32(w.sampleRate * w.channels * 2)
	binary.LittleEndian.PutUint32(header[28:32], byteRate)
	binary.LittleEndian.PutUint16(header[32:34], uint16(w.channels*2))
	binary.LittleEndian.PutUint16(header[34:36], 16)
	copy(header[36:40], []byte("data"))
	if _, err := w.f.Write(header); err != nil {
		return err
	}
	return nil
}

func (w *wavWriter) WriteFloat32Interleaved(samples []float32) error {
	need := len(samples) * 2
	if cap(w.buf) < need {
		w.buf = make([]byte, need)
	}
	buf := w.buf[:need]
	for i, sample := range samples {
		clamped := math.Max(-1, math.Min(1, float64(sample)))
		v := int16(math.Round(clamped * 32767))
		binary.LittleEndian.PutUint16(buf[i*2:], uint16(v))
	}
	n, err := w.f.Write(buf)
	w.dataBytes += int64(n)
	return err
}

func (w *wavWriter) Close() error {
	if w.f == nil {
		return nil
	}
	defer func() { _ = w.f.Close() }()
	if _, err := w.f.Seek(4, 0); err != nil {
		return err
	}
	if err := binary.Write(w.f, binary.LittleEndian, uint32(36+w.dataBytes)); err != nil {
		return err
	}
	if _, err := w.f.Seek(40, 0); err != nil {
		return err
	}
	if err := binary.Write(w.f, binary.LittleEndian, uint32(w.dataBytes)); err != nil {
		return err
	}
	return nil
}

func (w *wavWriter) DurationSeconds() float64 {
	if w.sampleRate <= 0 || w.channels <= 0 {
		return 0
	}
	return float64(w.dataBytes) / 2 / float64(w.channels) / float64(w.sampleRate)
}

func (w *wavWriter) String() string {
	return fmt.Sprintf("wavWriter(%dHz,%dch)", w.sampleRate, w.channels)
}
