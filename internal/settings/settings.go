package settings

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

const (
	appName    = "corder"
	configName = "config.json"
)

type Config struct {
	RecordingDir          string `json:"recording_dir"`
	InputDeviceName       string `json:"input_device_name"`
	MP3BitrateKbps        int    `json:"mp3_bitrate_kbps"`
	RetainWAVAfterConvert bool   `json:"retain_wav_after_convert"`
}

func Default() Config {
	return Config{
		RecordingDir:          defaultRecordingDir(),
		MP3BitrateKbps:        128,
		RetainWAVAfterConvert: false,
	}
}

func defaultRecordingDir() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "Recordings"
	}
	return filepath.Join(home, "Recordings")
}

func ConfigPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, configName), nil
}

func ConfigDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, appName), nil
}

func Load() (Config, error) {
	cfg := Default()
	path, err := ConfigPath()
	if err != nil {
		return cfg, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, nil
		}
		return cfg, err
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}
	if cfg.RecordingDir == "" {
		cfg.RecordingDir = defaultRecordingDir()
	}
	if cfg.MP3BitrateKbps <= 0 {
		cfg.MP3BitrateKbps = 128
	}
	return cfg, nil
}

func Save(cfg Config) error {
	path, err := ConfigPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
