package app

import (
	"testing"
)

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
