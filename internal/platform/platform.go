package platform

import (
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

var ErrClipboardUnavailable = errors.New("clipboard command unavailable")
var ErrFileClipboardUnavailable = errors.New("file clipboard command unavailable")

type CommandRunner interface {
	Start(name string, args ...string) error
	Run(name string, args ...string) error
	RunWithInput(input string, name string, args ...string) error
}

type OS struct {
	GOOS   string
	Runner CommandRunner
}

func New() OS {
	return OS{GOOS: runtime.GOOS, Runner: execRunner{}}
}

func (o OS) Open(path string) error {
	name, args := OpenCommand(o.goos(), path)
	return o.runner().Start(name, args...)
}

func (o OS) Reveal(path string) error {
	name, args := RevealCommand(o.goos(), path)
	return o.runner().Start(name, args...)
}

func (o OS) CopyToClipboard(text string) error {
	var lastErr error
	for _, cmd := range ClipboardCommands(o.goos()) {
		err := o.runner().RunWithInput(text, cmd[0], cmd[1:]...)
		if err == nil {
			return nil
		}
		lastErr = err
	}
	if lastErr != nil {
		return fmt.Errorf("%w: %v", ErrClipboardUnavailable, lastErr)
	}
	return ErrClipboardUnavailable
}

func (o OS) CopyFileReference(path string) error {
	commands, err := FileClipboardCommands(o.goos(), path)
	if err != nil {
		return err
	}
	var lastErr error
	for _, cmd := range commands {
		if cmd.Input == "" {
			lastErr = o.runner().Run(cmd.Name, cmd.Args...)
		} else {
			lastErr = o.runner().RunWithInput(cmd.Input, cmd.Name, cmd.Args...)
		}
		if lastErr == nil {
			return nil
		}
	}
	if lastErr != nil {
		return fmt.Errorf("%w: %v", ErrFileClipboardUnavailable, lastErr)
	}
	return ErrFileClipboardUnavailable
}

func OpenCommand(goos, path string) (string, []string) {
	switch goos {
	case "darwin":
		return "open", []string{path}
	case "windows":
		return "cmd", []string{"/c", "start", "", path}
	default:
		return "xdg-open", []string{path}
	}
}

func RevealCommand(goos, path string) (string, []string) {
	switch goos {
	case "darwin":
		return "open", []string{"-R", path}
	case "windows":
		return "explorer", []string{"/select," + path}
	default:
		return "xdg-open", []string{filepath.Dir(path)}
	}
}

type ClipboardCommand struct {
	Name  string
	Args  []string
	Input string
}

func FileClipboardCommands(goos, path string) ([]ClipboardCommand, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	switch goos {
	case "darwin":
		return []ClipboardCommand{{
			Name:  "osascript",
			Input: fmt.Sprintf("set the clipboard to POSIX file %q\n", abs),
		}}, nil
	case "windows":
		quoted := strings.ReplaceAll(abs, "'", "''")
		script := "Add-Type -AssemblyName System.Windows.Forms; " +
			"$files = New-Object System.Collections.Specialized.StringCollection; " +
			fmt.Sprintf("[void]$files.Add('%s'); ", quoted) +
			"[System.Windows.Forms.Clipboard]::SetFileDropList($files)"
		return []ClipboardCommand{{
			Name: "powershell",
			Args: []string{"-NoProfile", "-Command", script},
		}, {
			Name: "powershell.exe",
			Args: []string{"-NoProfile", "-Command", script},
		}}, nil
	default:
		input := "copy\n" + fileURI(abs) + "\n"
		return []ClipboardCommand{{
			Name:  "wl-copy",
			Args:  []string{"--type", "x-special/gnome-copied-files"},
			Input: input,
		}, {
			Name:  "xclip",
			Args:  []string{"-selection", "clipboard", "-t", "x-special/gnome-copied-files"},
			Input: input,
		}}, nil
	}
}

func ClipboardCommands(goos string) [][]string {
	switch goos {
	case "darwin":
		return [][]string{{"pbcopy"}}
	case "windows":
		return [][]string{{"cmd", "/c", "clip"}}
	default:
		return [][]string{{"wl-copy"}, {"xclip", "-selection", "clipboard"}, {"xsel", "--clipboard", "--input"}}
	}
}

func (o OS) goos() string {
	if o.GOOS == "" {
		return runtime.GOOS
	}
	return o.GOOS
}

func (o OS) runner() CommandRunner {
	if o.Runner == nil {
		return execRunner{}
	}
	return o.Runner
}

type execRunner struct{}

func (execRunner) Start(name string, args ...string) error {
	return exec.Command(name, args...).Start()
}

func (execRunner) Run(name string, args ...string) error {
	return exec.Command(name, args...).Run()
}

func (execRunner) RunWithInput(input string, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdin = strings.NewReader(input)
	return cmd.Run()
}

func fileURI(path string) string {
	path = filepath.ToSlash(path)
	if strings.HasPrefix(path, "/") {
		return "file://" + path
	}
	return "file:///" + path
}
