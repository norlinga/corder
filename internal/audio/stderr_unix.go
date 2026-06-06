//go:build !windows

package audio

import (
	"bytes"
	"io"
	"os"
	"sync"
	"syscall"
)

var stderrCaptureMu sync.Mutex

func captureNativeStderr(fn func()) string {
	stderrCaptureMu.Lock()
	defer stderrCaptureMu.Unlock()

	original, err := syscall.Dup(int(os.Stderr.Fd()))
	if err != nil {
		fn()
		return ""
	}
	defer syscall.Close(original)

	r, w, err := os.Pipe()
	if err != nil {
		fn()
		return ""
	}
	defer r.Close()

	if err := syscall.Dup2(int(w.Fd()), int(os.Stderr.Fd())); err != nil {
		_ = w.Close()
		fn()
		return ""
	}

	var buf bytes.Buffer
	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(&buf, r)
		close(done)
	}()

	fn()

	_ = syscall.Dup2(original, int(os.Stderr.Fd()))
	_ = w.Close()
	<-done
	return buf.String()
}
