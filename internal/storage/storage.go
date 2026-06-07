package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

var ErrDestinationExists = errors.New("destination already exists")

type Status string

const (
	StatusRecording   Status = "recording"
	StatusConverting  Status = "converting"
	StatusReady       Status = "ready"
	StatusEmpty       Status = "empty"
	StatusInterrupted Status = "Interrupted"
)

func (s Status) String() string {
	return string(s)
}

func (s Status) IsActive() bool {
	return s == StatusRecording || s == StatusConverting
}

type Meta struct {
	Version               int        `json:"version"`
	CreatedAt             time.Time  `json:"created_at"`
	SampleRate            float64    `json:"sample_rate"`
	Channels              int        `json:"channels"`
	DurationSeconds       float64    `json:"duration_seconds"`
	Status                Status     `json:"status"`
	MP3BitrateKbps        int        `json:"mp3_bitrate_kbps"`
	RetainWAVAfterConvert bool       `json:"retain_wav_after_convert"`
	ConvertedAt           *time.Time `json:"converted_at,omitempty"`
	SourceWAV             string     `json:"source_wav,omitempty"`
}

type Recording struct {
	Path        string
	MetaPath    string
	Name        string
	Duration    time.Duration
	Size        int64
	CreatedAt   time.Time
	Status      Status
	ProgressPct float64
}

var audioExt = map[string]bool{
	".mp3": true,
	".wav": true,
}

func MetaPathFor(audioPath string) string {
	ext := filepath.Ext(audioPath)
	return strings.TrimSuffix(audioPath, ext) + ".json"
}

func EnsureDir(dir string) error {
	return os.MkdirAll(dir, 0o755)
}

func WriteMeta(audioPath string, meta Meta) error {
	meta.Version = 1
	metaPath := MetaPathFor(audioPath)
	dir := filepath.Dir(metaPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(metaPath)+".*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpPath, 0o644); err != nil {
		return err
	}
	return os.Rename(tmpPath, metaPath)
}

func ReadMeta(audioPath string) (Meta, error) {
	metaPath := MetaPathFor(audioPath)
	data, err := os.ReadFile(metaPath)
	if err != nil {
		return Meta{}, err
	}
	var meta Meta
	if err := json.Unmarshal(data, &meta); err != nil {
		return Meta{}, err
	}
	return meta, nil
}

func DeleteMeta(audioPath string) error {
	metaPath := MetaPathFor(audioPath)
	if err := os.Remove(metaPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func DeleteRecording(audioPath string) error {
	if err := os.Remove(audioPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return DeleteMeta(audioPath)
}

func RenameWithMeta(oldPath, newPath string) error {
	if oldPath == newPath {
		return nil
	}
	if _, err := os.Stat(newPath); err == nil {
		return fmt.Errorf("%w: %s", ErrDestinationExists, newPath)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	oldMeta := MetaPathFor(oldPath)
	newMeta := MetaPathFor(newPath)
	if oldMeta != newMeta {
		if _, err := os.Stat(newMeta); err == nil {
			return fmt.Errorf("%w: %s", ErrDestinationExists, newMeta)
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	if err := os.Rename(oldPath, newPath); err != nil {
		return err
	}
	if _, err := os.Stat(oldMeta); err == nil {
		if err := os.Rename(oldMeta, newMeta); err != nil && !errors.Is(err, os.ErrNotExist) {
			rollbackErr := os.Rename(newPath, oldPath)
			return errors.Join(err, rollbackErr)
		}
	}
	return nil
}

func Scan(dir string) ([]Recording, error) {
	if err := EnsureDir(dir); err != nil {
		return nil, err
	}
	var recs []Recording
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if !audioExt[ext] {
			return nil
		}
		fi, err := os.Stat(path)
		if err != nil {
			return nil
		}
		rec := Recording{
			Path:      path,
			MetaPath:  MetaPathFor(path),
			Name:      filepath.Base(path),
			Size:      fi.Size(),
			CreatedAt: fi.ModTime(),
		}
		meta, err := ReadMeta(path)
		if err == nil {
			if meta.DurationSeconds > 0 {
				rec.Duration = time.Duration(meta.DurationSeconds * float64(time.Second))
			}
			if meta.Status != "" {
				rec.Status = meta.Status
				if meta.Status.IsActive() {
					rec.Status = StatusInterrupted
				}
			}
		}
		if rec.Duration == 0 && ext == ".wav" {
			if dur, ok := wavDuration(fi.Size(), meta); ok {
				rec.Duration = dur
			}
		}
		rec.Status = Status(strings.TrimSpace(rec.Status.String()))
		recs = append(recs, rec)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(recs, func(i, j int) bool {
		if recs[i].CreatedAt.Equal(recs[j].CreatedAt) {
			return recs[i].Name > recs[j].Name
		}
		return recs[i].CreatedAt.After(recs[j].CreatedAt)
	})
	return recs, nil
}

func wavDuration(size int64, meta Meta) (time.Duration, bool) {
	if meta.SampleRate <= 0 || meta.Channels <= 0 || size <= 44 {
		return 0, false
	}
	samples := float64(size-44) / 2 / float64(meta.Channels)
	seconds := samples / meta.SampleRate
	if seconds <= 0 {
		return 0, false
	}
	return time.Duration(seconds * float64(time.Second)), true
}

func FormatDuration(d time.Duration) string {
	if d <= 0 {
		return "--"
	}
	total := int64(math.Round(d.Seconds()))
	h := total / 3600
	m := (total % 3600) / 60
	s := total % 60
	if h > 0 {
		return fmt.Sprintf("%dh %02dm %02ds", h, m, s)
	}
	return fmt.Sprintf("%dm %02ds", m, s)
}

func FormatSize(size int64) string {
	const unit = 1024
	if size < unit {
		return strconv.FormatInt(size, 10) + " B"
	}
	div, exp := int64(unit), 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	value := float64(size) / float64(div)
	suffixes := []string{"KB", "MB", "GB", "TB", "PB"}
	if exp >= len(suffixes) {
		exp = len(suffixes) - 1
	}
	suffix := suffixes[exp]
	return fmt.Sprintf("%.1f %s", value, suffix)
}

func ResolveAudioName(path string) string {
	return filepath.Base(path)
}
