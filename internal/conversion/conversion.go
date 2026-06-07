package conversion

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"corder/internal/jobs"
	"corder/internal/storage"
)

type Job struct {
	Source      string
	Destination string
	StartedAt   time.Time
	Duration    time.Duration
	BitrateKbps int
	RetainWAV   bool
}

func CheckFFmpeg() error {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return fmt.Errorf("ffmpeg not found: install ffmpeg to convert recordings to MP3")
	}
	return nil
}

func DestFor(source string) string {
	return strings.TrimSuffix(source, filepath.Ext(source)) + ".mp3"
}

func Run(ctx context.Context, job Job, updates chan<- jobs.Update) error {
	cmd := exec.CommandContext(ctx, "ffmpeg", ffmpegArgs(job)...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	select {
	case updates <- conversionUpdate(job, jobs.StatusRunning, 0, "Converting", nil):
	default:
	}
	go streamProgress(stdout, job, updates)
	errData, _ := io.ReadAll(stderr)
	waitErr := cmd.Wait()
	if waitErr != nil {
		if len(errData) > 0 {
			waitErr = fmt.Errorf("%w: %s", waitErr, strings.TrimSpace(string(errData)))
		}
		select {
		case updates <- conversionUpdate(job, jobs.StatusFailed, 100, "Conversion failed", waitErr):
		default:
		}
		return waitErr
	}
	convertedAt := time.Now()
	createdAt := job.StartedAt
	if createdAt.IsZero() {
		createdAt = convertedAt
	}
	if err := storage.WriteMeta(job.Destination, storage.Meta{
		CreatedAt:             createdAt,
		DurationSeconds:       job.Duration.Seconds(),
		Status:                storage.StatusReady,
		MP3BitrateKbps:        job.BitrateKbps,
		RetainWAVAfterConvert: job.RetainWAV,
		ConvertedAt:           &convertedAt,
		SourceWAV:             job.Source,
	}); err != nil {
		return err
	}
	if !job.RetainWAV {
		if sourceHasDistinctMeta(job) {
			return storage.DeleteRecording(job.Source)
		}
		if err := os.Remove(job.Source); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	select {
	case updates <- conversionUpdate(job, jobs.StatusDone, 100, "Saved", nil):
	default:
	}
	return nil
}

func conversionUpdate(job Job, status jobs.Status, percent float64, message string, err error) jobs.Update {
	return jobs.Update{
		Kind:        jobs.KindConversion,
		ID:          jobs.ID(jobs.KindConversion, job.Source),
		Path:        job.Source,
		Destination: job.Destination,
		Percent:     percent,
		Message:     message,
		Status:      status,
		Err:         err,
	}
}

func sourceHasDistinctMeta(job Job) bool {
	if job.Source == "" || job.Destination == "" {
		return false
	}
	return storage.MetaPathFor(job.Source) != storage.MetaPathFor(job.Destination)
}

func ffmpegArgs(job Job) []string {
	return []string{
		"-y",
		"-hide_banner",
		"-loglevel", "error",
		"-i", job.Source,
		"-b:a", fmt.Sprintf("%dk", job.BitrateKbps),
		"-progress", "pipe:1",
		"-nostats",
		job.Destination,
	}
}

func streamProgress(r io.Reader, job Job, updates chan<- jobs.Update) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		pct, ok := progressPercent(scanner.Text(), job.Duration)
		if !ok {
			continue
		}
		select {
		case updates <- conversionUpdate(job, jobs.StatusRunning, pct, "Converting", nil):
		default:
		}
	}
}

func progressPercent(line string, duration time.Duration) (float64, bool) {
	if duration <= 0 || !strings.HasPrefix(line, "out_time_ms=") {
		return 0, false
	}
	n, err := strconv.ParseInt(strings.TrimPrefix(line, "out_time_ms="), 10, 64)
	if err != nil {
		return 0, false
	}
	pct := (float64(n) / 1000000.0) / duration.Seconds() * 100
	if pct > 99.0 {
		pct = 99.0
	}
	if pct < 0 {
		pct = 0
	}
	return pct, true
}
