package extensions

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

const SupportedSchema = 1

type Manifest struct {
	Schema  int      `json:"schema"`
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Version string   `json:"version"`
	Actions []Action `json:"actions"`
}

type Action struct {
	ID             string   `json:"id"`
	Key            string   `json:"key"`
	Label          string   `json:"label"`
	Command        string   `json:"command"`
	Args           []string `json:"args"`
	Formats        []string `json:"formats"`
	Job            bool     `json:"job"`
	TimeoutSeconds int      `json:"timeout_seconds"`
}

type RegisteredAction struct {
	PluginID       string
	PluginName     string
	ActionID       string
	Key            string
	Label          string
	Command        string
	Args           []string
	Formats        []string
	Job            bool
	TimeoutSeconds int
}

func (a RegisteredAction) FullID() string {
	return a.PluginID + "." + a.ActionID
}

func (a RegisteredAction) Kind() string {
	return "plugin:" + a.FullID()
}

func (a RegisteredAction) Timeout() time.Duration {
	if a.TimeoutSeconds <= 0 {
		return 0
	}
	return time.Duration(a.TimeoutSeconds) * time.Second
}

func (a RegisteredAction) AppliesTo(path string) bool {
	if len(a.Formats) == 0 {
		return true
	}
	ext := strings.ToLower(filepath.Ext(path))
	for _, format := range a.Formats {
		if ext == format {
			return true
		}
	}
	return false
}

type LoadResult struct {
	Actions []RegisteredAction
	Issues  []Issue
}

type Issue struct {
	PluginID string
	ActionID string
	Message  string
}

func (i Issue) String() string {
	switch {
	case i.PluginID != "" && i.ActionID != "":
		return fmt.Sprintf("%s.%s: %s", i.PluginID, i.ActionID, i.Message)
	case i.PluginID != "":
		return fmt.Sprintf("%s: %s", i.PluginID, i.Message)
	default:
		return i.Message
	}
}
