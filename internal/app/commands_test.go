package app

import (
	"errors"
	"testing"

	"corder/internal/jobs"
)

func TestConversionFailureUpdate(t *testing.T) {
	err := errors.New("ffmpeg failed")

	update := conversionFailureUpdate("/recordings/a.wav", "/recordings/a.mp3", err)

	if update.ID != "/recordings/a.wav" || update.Path != "/recordings/a.wav" || update.Destination != "/recordings/a.mp3" {
		t.Fatalf("paths = %+v", update)
	}
	if update.Kind != jobs.KindConversion || update.Status != jobs.StatusFailed || update.Message != "Conversion failed" {
		t.Fatalf("status = %+v", update)
	}
	if !errors.Is(update.Err, err) {
		t.Fatalf("err = %v, want %v", update.Err, err)
	}
}

func TestDeleteCmdReturnsResult(t *testing.T) {
	msg := deleteCmd("/tmp/does-not-exist.mp3")()
	result, ok := msg.(deleteResultMsg)
	if !ok {
		t.Fatalf("msg = %T, want deleteResultMsg", msg)
	}
	if result.path != "/tmp/does-not-exist.mp3" || result.err != nil {
		t.Fatalf("delete result = %+v", result)
	}
}
