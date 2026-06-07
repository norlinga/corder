package extensions

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func validateManifest(manifest Manifest, usedKeys map[string]string, strictCommandResolution bool) ([]RegisteredAction, []Issue) {
	pluginID := strings.TrimSpace(manifest.ID)
	if manifest.Schema != SupportedSchema {
		return nil, []Issue{{PluginID: pluginID, Message: fmt.Sprintf("unsupported schema %d", manifest.Schema)}}
	}
	if pluginID == "" {
		return nil, []Issue{{Message: "plugin id is empty"}}
	}

	var actions []RegisteredAction
	var issues []Issue
	for _, action := range manifest.Actions {
		registered, actionIssues := validateAction(manifest, action, usedKeys, strictCommandResolution)
		if len(actionIssues) > 0 {
			issues = append(issues, actionIssues...)
			continue
		}
		actions = append(actions, registered)
		usedKeys[registered.Key] = registered.FullID()
	}
	return actions, issues
}

func validateAction(manifest Manifest, action Action, usedKeys map[string]string, strictCommandResolution bool) (RegisteredAction, []Issue) {
	pluginID := strings.TrimSpace(manifest.ID)
	actionID := strings.TrimSpace(action.ID)
	issue := func(message string) Issue {
		return Issue{PluginID: pluginID, ActionID: actionID, Message: message}
	}

	var issues []Issue
	if actionID == "" {
		issues = append(issues, issue("action id is empty"))
	}
	key := strings.TrimSpace(action.Key)
	if key == "" {
		issues = append(issues, issue("action key is empty"))
	} else if owner, ok := usedKeys[key]; ok {
		if owner == "builtin" {
			issues = append(issues, issue(fmt.Sprintf("action key %q conflicts with a built-in action", key)))
		} else {
			issues = append(issues, issue(fmt.Sprintf("action key %q conflicts with plugin action %s", key, owner)))
		}
	}
	command := strings.TrimSpace(action.Command)
	if command == "" {
		issues = append(issues, issue("action command is empty"))
	} else if strictCommandResolution {
		if err := commandExists(command); err != nil {
			issues = append(issues, issue(fmt.Sprintf("action command cannot be resolved: %v", err)))
		}
	}
	formats, formatIssues := normalizeFormats(action.Formats)
	for _, msg := range formatIssues {
		issues = append(issues, issue(msg))
	}
	if len(issues) > 0 {
		return RegisteredAction{}, issues
	}
	label := strings.TrimSpace(action.Label)
	if label == "" {
		label = actionID
	}
	return RegisteredAction{
		PluginID:       pluginID,
		PluginName:     strings.TrimSpace(manifest.Name),
		ActionID:       actionID,
		Key:            key,
		Label:          label,
		Command:        command,
		Args:           append([]string(nil), action.Args...),
		Formats:        formats,
		Job:            action.Job,
		TimeoutSeconds: action.TimeoutSeconds,
	}, nil
}

func normalizeFormats(formats []string) ([]string, []string) {
	seen := map[string]bool{}
	var normalized []string
	var issues []string
	for _, format := range formats {
		format = strings.ToLower(strings.TrimSpace(format))
		if format == "" {
			issues = append(issues, "format extension is empty")
			continue
		}
		if !strings.HasPrefix(format, ".") || strings.ContainsAny(format, `/\`) {
			issues = append(issues, fmt.Sprintf("invalid format extension %q", format))
			continue
		}
		if seen[format] {
			continue
		}
		seen[format] = true
		normalized = append(normalized, format)
	}
	return normalized, issues
}

func commandExists(command string) error {
	if filepath.Base(command) == command {
		_, err := exec.LookPath(command)
		return err
	}
	info, err := os.Stat(command)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("%s is a directory", command)
	}
	return nil
}
