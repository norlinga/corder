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

	"corder/internal/storage"
)

type Progress struct {
	Source      string
	Destination string
	Percent     float64
	Message     string
	Done        bool
	Err         error
}

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

func Run(ctx context.Context, job Job, updates chan<- Progress) error {
	args := []string{
		"-y",
		"-hide_banner",
		"-loglevel", "error",
		"-i", job.Source,
		"-b:a", fmt.Sprintf("%dk", job.BitrateKbps),
		"-progress", "pipe:1",
		"-nostats",
		job.Destination,
	}
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
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
	case updates <- Progress{Source: job.Source, Destination: job.Destination, Percent: 0, Message: "Converting"}:
	default:
	}
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "out_time_ms=") {
				n, err := strconv.ParseInt(strings.TrimPrefix(line, "out_time_ms="), 10, 64)
				if err == nil && job.Duration > 0 {
					pct := (float64(n) / 1000000.0) / job.Duration.Seconds() * 100
					if pct > 99.0 {
						pct = 99.0
					}
					select {
					case updates <- Progress{Source: job.Source, Destination: job.Destination, Percent: pct, Message: "Converting"}:
					default:
					}
				}
			}
		}
	}()
	errData, _ := io.ReadAll(stderr)
	waitErr := cmd.Wait()
	if waitErr != nil {
		if len(errData) > 0 {
			waitErr = fmt.Errorf("%w: %s", waitErr, strings.TrimSpace(string(errData)))
		}
		select {
		case updates <- Progress{Source: job.Source, Destination: job.Destination, Err: waitErr, Done: true, Message: "Conversion failed"}:
		default:
		}
		return waitErr
	}
	if !job.RetainWAV {
		if err := os.Remove(job.Source); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := storage.DeleteMeta(job.Source); err != nil {
			return err
		}
	}
	convertedAt := time.Now()
	_ = storage.WriteMeta(job.Destination, storage.Meta{
		CreatedAt:             convertedAt,
		DurationSeconds:       job.Duration.Seconds(),
		Status:                "ready",
		MP3BitrateKbps:        job.BitrateKbps,
		RetainWAVAfterConvert: job.RetainWAV,
		ConvertedAt:           &convertedAt,
		SourceWAV:             job.Source,
	})
	select {
	case updates <- Progress{Source: job.Source, Destination: job.Destination, Percent: 100, Done: true, Message: "Saved"}:
	default:
	}
	return nil
}
