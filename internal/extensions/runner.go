package extensions

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"corder/internal/jobs"
)

type Invocation struct {
	Action    RegisteredAction
	Path      string
	MetaPath  string
	ConfigDir string
}

func Run(ctx context.Context, inv Invocation, updates chan<- jobs.Update) error {
	if inv.Action.Timeout() > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, inv.Action.Timeout())
		defer cancel()
	}

	kind := inv.Action.Kind()
	id := jobs.ID(kind, inv.Path)
	send := func(update jobs.Update) {
		if updates != nil {
			updates <- update
		}
	}
	send(jobs.Update{
		ID:      id,
		Kind:    kind,
		Path:    inv.Path,
		Message: inv.Action.Label,
		Status:  jobs.StatusRunning,
	})

	template := TemplateContextFor(inv.Path, inv.MetaPath, inv.ConfigDir)
	args := ExpandArgs(inv.Action.Args, template)
	cmd := exec.CommandContext(ctx, inv.Action.Command, args...)
	cmd.Env = append(os.Environ(), env(inv, template)...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		failed := failedUpdate(id, kind, inv.Path, err.Error())
		send(failed)
		return err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		failed := failedUpdate(id, kind, inv.Path, err.Error())
		send(failed)
		return err
	}

	var last Event
	var parseErrs []error
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		parseErrs = readEvents(stdout, func(event Event) {
			last = event
			if update, ok := eventUpdate(event, id, kind, inv.Path, inv.Action.Label); ok {
				send(update)
			}
		})
	}()
	err = cmd.Wait()
	<-readDone

	if len(parseErrs) > 0 && err == nil {
		err = errors.Join(parseErrs...)
	}
	if last.Type == "error" && err == nil {
		err = errors.New(last.Message)
	}
	if ctx.Err() != nil {
		err = ctx.Err()
	}
	if err != nil {
		message := failureMessage(err, stderr.String())
		send(failedUpdate(id, kind, inv.Path, message))
		return err
	}
	if last.Type != "result" {
		send(jobs.Update{
			ID:      id,
			Kind:    kind,
			Path:    inv.Path,
			Percent: 100,
			Message: inv.Action.Label + " complete",
			Status:  jobs.StatusDone,
		})
	}
	return nil
}

func readEvents(reader io.Reader, handle func(Event)) []error {
	var errs []error
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var event Event
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			errs = append(errs, fmt.Errorf("invalid plugin event: %w", err))
			continue
		}
		handle(event)
	}
	if err := scanner.Err(); err != nil {
		errs = append(errs, err)
	}
	return errs
}

func eventUpdate(event Event, id, kind, path, label string) (jobs.Update, bool) {
	message := strings.TrimSpace(event.Message)
	switch event.Type {
	case "status":
		if message == "" {
			message = label
		}
		return jobs.Update{ID: id, Kind: kind, Path: path, Message: message, Status: jobs.StatusRunning}, true
	case "progress":
		if message == "" {
			message = label
		}
		return jobs.Update{ID: id, Kind: kind, Path: path, Message: message, Percent: event.Percent, Status: jobs.StatusRunning}, true
	case "result":
		if message == "" {
			message = label + " complete"
		}
		destination := ""
		if len(event.Paths) > 0 {
			destination = event.Paths[0]
		}
		return jobs.Update{ID: id, Kind: kind, Path: path, Destination: destination, Percent: 100, Message: message, Status: jobs.StatusDone}, true
	case "error":
		if message == "" {
			message = label + " failed"
		}
		return failedUpdate(id, kind, path, message), true
	default:
		return jobs.Update{}, false
	}
}

func failedUpdate(id, kind, path, message string) jobs.Update {
	if strings.TrimSpace(message) == "" {
		message = "plugin failed"
	}
	return jobs.Update{
		ID:      id,
		Kind:    kind,
		Path:    path,
		Message: message,
		Status:  jobs.StatusFailed,
	}
}

func failureMessage(err error, stderr string) string {
	stderr = strings.TrimSpace(stderr)
	if stderr != "" {
		return stderr
	}
	return err.Error()
}

func env(inv Invocation, ctx TemplateContext) []string {
	return []string{
		"CORDER_PLUGIN_ID=" + inv.Action.PluginID,
		"CORDER_ACTION_ID=" + inv.Action.FullID(),
		"CORDER_RECORDING_PATH=" + ctx.Path,
		"CORDER_META_PATH=" + ctx.MetaPath,
		"CORDER_RECORDING_NAME=" + ctx.Name,
		"CORDER_RECORDING_DIR=" + ctx.RecordingDir,
		"CORDER_CONFIG_DIR=" + ctx.ConfigDir,
	}
}
