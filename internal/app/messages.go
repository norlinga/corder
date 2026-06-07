package app

import (
	"time"

	"corder/internal/audio"
	"corder/internal/jobs"
	"corder/internal/recording"
	"corder/internal/storage"
)

type tickMsg struct{}
type refreshMsg struct{}

type recordsMsg struct {
	recs []storage.Recording
	err  error
}

type devicesMsg struct {
	devices []audio.Device
	err     error
}

type levelMsg audio.LevelUpdate
type jobMsg jobs.Update

type diagnosticMsg struct {
	info  audio.Diagnostics
	probe audio.ProbeResult
	err   error
}

type recordStartedMsg struct {
	session    recording.Session
	path       string
	deviceName string
}

type recordStoppedMsg struct {
	path        string
	destination string
	duration    time.Duration
	queued      bool
	err         error
}

type renameResultMsg struct {
	oldPath string
	newPath string
	err     error
}

type deleteResultMsg struct {
	path string
	err  error
}

type openResultMsg struct {
	path string
	err  error
}

type revealResultMsg struct {
	path string
	err  error
}

type copyResultMsg struct {
	text string
	file bool
	err  error
}
