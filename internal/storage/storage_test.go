package storage

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWriteReadMeta(t *testing.T) {
	dir := t.TempDir()
	audioPath := filepath.Join(dir, "recording.wav")
	meta := Meta{
		CreatedAt:       time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC),
		SampleRate:      48000,
		Channels:        1,
		DurationSeconds: 12.5,
		Status:          StatusReady,
		MP3BitrateKbps:  128,
	}

	if err := WriteMeta(audioPath, meta); err != nil {
		t.Fatalf("WriteMeta: %v", err)
	}
	got, err := ReadMeta(audioPath)
	if err != nil {
		t.Fatalf("ReadMeta: %v", err)
	}
	if got.Version != 1 {
		t.Fatalf("Version = %d, want 1", got.Version)
	}
	if got.SampleRate != meta.SampleRate || got.Channels != meta.Channels || got.Status != meta.Status {
		t.Fatalf("ReadMeta = %+v, want key fields from %+v", got, meta)
	}
}

func TestWriteMetaReplacesExistingMeta(t *testing.T) {
	dir := t.TempDir()
	audioPath := filepath.Join(dir, "recording.wav")
	if err := WriteMeta(audioPath, Meta{Status: StatusRecording}); err != nil {
		t.Fatalf("first WriteMeta: %v", err)
	}
	if err := WriteMeta(audioPath, Meta{Status: StatusReady, DurationSeconds: 3}); err != nil {
		t.Fatalf("second WriteMeta: %v", err)
	}
	got, err := ReadMeta(audioPath)
	if err != nil {
		t.Fatalf("ReadMeta: %v", err)
	}
	if got.Status != StatusReady || got.DurationSeconds != 3 {
		t.Fatalf("meta = %+v, want ready/3s", got)
	}
}

func TestRenameWithMetaMovesAudioAndMetadata(t *testing.T) {
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "old.wav")
	newPath := filepath.Join(dir, "new.wav")
	if err := os.WriteFile(oldPath, []byte("audio"), 0o644); err != nil {
		t.Fatalf("write old audio: %v", err)
	}
	if err := WriteMeta(oldPath, Meta{Status: StatusReady, SampleRate: 44100, Channels: 1}); err != nil {
		t.Fatalf("WriteMeta: %v", err)
	}

	if err := RenameWithMeta(oldPath, newPath); err != nil {
		t.Fatalf("RenameWithMeta: %v", err)
	}
	if _, err := os.Stat(oldPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("old audio still exists or unexpected error: %v", err)
	}
	if _, err := os.Stat(newPath); err != nil {
		t.Fatalf("new audio missing: %v", err)
	}
	if _, err := os.Stat(MetaPathFor(oldPath)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("old meta still exists or unexpected error: %v", err)
	}
	if _, err := os.Stat(MetaPathFor(newPath)); err != nil {
		t.Fatalf("new meta missing: %v", err)
	}
}

func TestRenameWithMetaRefusesOverwrite(t *testing.T) {
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "old.wav")
	newPath := filepath.Join(dir, "new.wav")
	if err := os.WriteFile(oldPath, []byte("old"), 0o644); err != nil {
		t.Fatalf("write old audio: %v", err)
	}
	if err := os.WriteFile(newPath, []byte("new"), 0o644); err != nil {
		t.Fatalf("write new audio: %v", err)
	}

	err := RenameWithMeta(oldPath, newPath)
	if !errors.Is(err, ErrDestinationExists) {
		t.Fatalf("RenameWithMeta error = %v, want ErrDestinationExists", err)
	}
	data, err := os.ReadFile(newPath)
	if err != nil {
		t.Fatalf("read new audio: %v", err)
	}
	if string(data) != "new" {
		t.Fatalf("destination was overwritten: %q", data)
	}
}

func TestRenameWithMetaRefusesMetadataOverwrite(t *testing.T) {
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "old.wav")
	newPath := filepath.Join(dir, "new.mp3")
	if err := os.WriteFile(oldPath, []byte("old"), 0o644); err != nil {
		t.Fatalf("write old audio: %v", err)
	}
	if err := WriteMeta(oldPath, Meta{Status: StatusReady}); err != nil {
		t.Fatalf("write old meta: %v", err)
	}
	if err := WriteMeta(newPath, Meta{Status: StatusReady}); err != nil {
		t.Fatalf("write new meta: %v", err)
	}

	err := RenameWithMeta(oldPath, newPath)
	if !errors.Is(err, ErrDestinationExists) {
		t.Fatalf("RenameWithMeta error = %v, want ErrDestinationExists", err)
	}
	if _, err := os.Stat(oldPath); err != nil {
		t.Fatalf("old audio should remain after refused rename: %v", err)
	}
}

func TestDeleteRecordingDeletesAudioAndMetadata(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "recording.mp3")
	if err := os.WriteFile(path, []byte("audio"), 0o644); err != nil {
		t.Fatalf("write audio: %v", err)
	}
	if err := WriteMeta(path, Meta{Status: StatusReady}); err != nil {
		t.Fatalf("write meta: %v", err)
	}

	if err := DeleteRecording(path); err != nil {
		t.Fatalf("DeleteRecording: %v", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("audio stat = %v, want not exist", err)
	}
	if _, err := os.Stat(MetaPathFor(path)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("meta stat = %v, want not exist", err)
	}
}

func TestDeleteRecordingAllowsMissingFiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.mp3")
	if err := DeleteRecording(path); err != nil {
		t.Fatalf("DeleteRecording missing file: %v", err)
	}
}

func TestScanUsesMetaAndMarksInterruptedActiveStatuses(t *testing.T) {
	dir := t.TempDir()
	wav := filepath.Join(dir, "active.wav")
	mp3 := filepath.Join(dir, "done.mp3")
	if err := os.WriteFile(wav, make([]byte, 44+44100*2), 0o644); err != nil {
		t.Fatalf("write wav: %v", err)
	}
	if err := os.WriteFile(mp3, []byte("mp3"), 0o644); err != nil {
		t.Fatalf("write mp3: %v", err)
	}
	if err := WriteMeta(wav, Meta{Status: StatusRecording, SampleRate: 44100, Channels: 1}); err != nil {
		t.Fatalf("write wav meta: %v", err)
	}
	if err := WriteMeta(mp3, Meta{Status: StatusReady, DurationSeconds: 2}); err != nil {
		t.Fatalf("write mp3 meta: %v", err)
	}

	recs, err := Scan(dir)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(recs) != 2 {
		t.Fatalf("len(recs) = %d, want 2", len(recs))
	}
	byName := map[string]Recording{}
	for _, rec := range recs {
		byName[rec.Name] = rec
	}
	if byName["active.wav"].Status != StatusInterrupted {
		t.Fatalf("active status = %q, want Interrupted", byName["active.wav"].Status)
	}
	if byName["active.wav"].Duration != time.Second {
		t.Fatalf("active duration = %v, want 1s", byName["active.wav"].Duration)
	}
	if byName["done.mp3"].Status != StatusReady {
		t.Fatalf("done status = %q, want ready", byName["done.mp3"].Status)
	}
}

func TestFormatDurationAndSize(t *testing.T) {
	if got := FormatDuration(0); got != "--" {
		t.Fatalf("FormatDuration(0) = %q, want --", got)
	}
	if got := FormatDuration(4*time.Minute + 21*time.Second); got != "4m 21s" {
		t.Fatalf("FormatDuration = %q, want 4m 21s", got)
	}
	if got := FormatDuration(42*time.Minute + 11*time.Second); got != "42m 11s" {
		t.Fatalf("FormatDuration = %q, want 42m 11s", got)
	}
	if got := FormatSize(38 * 1024 * 1024); got != "38.0 MB" {
		t.Fatalf("FormatSize = %q, want 38.0 MB", got)
	}
}
