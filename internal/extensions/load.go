package extensions

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
)

type LoadOptions struct {
	ConfigDir               string
	BuiltinKeys             []string
	StrictCommandResolution bool
}

func Load(options LoadOptions) LoadResult {
	result := LoadResult{}
	if options.ConfigDir == "" {
		result.Issues = append(result.Issues, Issue{Message: "config directory is empty"})
		return result
	}
	paths, err := discoverManifests(options.ConfigDir)
	if err != nil {
		result.Issues = append(result.Issues, Issue{Message: err.Error()})
		return result
	}
	usedKeys := map[string]string{}
	for _, key := range options.BuiltinKeys {
		usedKeys[key] = "builtin"
	}
	for _, path := range paths {
		manifest, issues := readManifest(path)
		result.Issues = append(result.Issues, issues...)
		if len(issues) > 0 {
			continue
		}
		actions, issues := validateManifest(manifest, usedKeys, options.StrictCommandResolution)
		result.Actions = append(result.Actions, actions...)
		result.Issues = append(result.Issues, issues...)
	}
	return result
}

func discoverManifests(configDir string) ([]string, error) {
	pluginsDir := filepath.Join(configDir, "plugins")
	entries, err := os.ReadDir(pluginsDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		path := filepath.Join(pluginsDir, entry.Name())
		if entry.IsDir() {
			manifestPath := filepath.Join(path, "plugin.json")
			if _, err := os.Stat(manifestPath); err == nil {
				paths = append(paths, manifestPath)
			} else if err != nil && !errors.Is(err, os.ErrNotExist) {
				return nil, err
			}
			continue
		}
		if filepath.Ext(entry.Name()) == ".json" {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	return paths, nil
}

func readManifest(path string) (Manifest, []Issue) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, []Issue{{Message: err.Error()}}
	}
	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return Manifest{}, []Issue{{Message: err.Error()}}
	}
	return manifest, nil
}
