package audio

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func TestWAVWriterWritesHeaderAndClampedSamples(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.wav")
	writer, err := newWAVWriter(path, 44100, 1)
	if err != nil {
		t.Fatalf("newWAVWriter: %v", err)
	}
	if err := writer.WriteFloat32Interleaved([]float32{-2, -0.5, 0, 0.5, 2}); err != nil {
		t.Fatalf("WriteFloat32Interleaved: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data[0:4]) != "RIFF" || string(data[8:12]) != "WAVE" || string(data[36:40]) != "data" {
		t.Fatalf("invalid WAV identifiers")
	}
	if got := binary.LittleEndian.Uint32(data[40:44]); got != 10 {
		t.Fatalf("data bytes = %d, want 10", got)
	}
	if got := binary.LittleEndian.Uint32(data[4:8]); got != 46 {
		t.Fatalf("riff size = %d, want 46", got)
	}
	if got := int16(binary.LittleEndian.Uint16(data[44:46])); got != -32767 {
		t.Fatalf("first sample = %d, want -32767", got)
	}
	if got := int16(binary.LittleEndian.Uint16(data[52:54])); got != 32767 {
		t.Fatalf("last sample = %d, want 32767", got)
	}
	if got := writer.DurationSeconds(); got <= 0 {
		t.Fatalf("DurationSeconds = %f, want > 0", got)
	}
}
