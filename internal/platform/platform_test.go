package platform

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestOpenCommand(t *testing.T) {
	tests := []struct {
		goos string
		name string
		args []string
	}{
		{goos: "darwin", name: "open", args: []string{"/tmp/a.mp3"}},
		{goos: "windows", name: "cmd", args: []string{"/c", "start", "", "/tmp/a.mp3"}},
		{goos: "linux", name: "xdg-open", args: []string{"/tmp/a.mp3"}},
	}
	for _, tt := range tests {
		name, args := OpenCommand(tt.goos, "/tmp/a.mp3")
		if name != tt.name || !reflect.DeepEqual(args, tt.args) {
			t.Fatalf("OpenCommand(%q) = %q %#v, want %q %#v", tt.goos, name, args, tt.name, tt.args)
		}
	}
}

func TestClipboardCommands(t *testing.T) {
	if got := ClipboardCommands("darwin"); !reflect.DeepEqual(got, [][]string{{"pbcopy"}}) {
		t.Fatalf("darwin clipboard commands = %#v", got)
	}
	if got := ClipboardCommands("windows"); !reflect.DeepEqual(got, [][]string{{"cmd", "/c", "clip"}}) {
		t.Fatalf("windows clipboard commands = %#v", got)
	}
	if got := ClipboardCommands("linux"); len(got) != 3 || got[0][0] != "wl-copy" {
		t.Fatalf("linux clipboard commands = %#v", got)
	}
}

func TestFileClipboardCommands(t *testing.T) {
	linux, err := FileClipboardCommands("linux", "/tmp/a.mp3")
	if err != nil {
		t.Fatalf("linux FileClipboardCommands: %v", err)
	}
	if len(linux) != 2 || linux[0].Name != "wl-copy" || !reflect.DeepEqual(linux[0].Args, []string{"--type", "x-special/gnome-copied-files"}) {
		t.Fatalf("linux file clipboard commands = %#v", linux)
	}
	if !strings.Contains(linux[0].Input, "file:///tmp/a.mp3") {
		t.Fatalf("linux file clipboard input = %q", linux[0].Input)
	}

	darwin, err := FileClipboardCommands("darwin", "/tmp/a.mp3")
	if err != nil {
		t.Fatalf("darwin FileClipboardCommands: %v", err)
	}
	if len(darwin) != 1 || darwin[0].Name != "osascript" || !strings.Contains(darwin[0].Input, "POSIX file") {
		t.Fatalf("darwin file clipboard commands = %#v", darwin)
	}

	windows, err := FileClipboardCommands("windows", `C:\tmp\a.mp3`)
	if err != nil {
		t.Fatalf("windows FileClipboardCommands: %v", err)
	}
	if len(windows) != 2 || windows[0].Name != "powershell" || !strings.Contains(windows[0].Args[len(windows[0].Args)-1], "SetFileDropList") {
		t.Fatalf("windows file clipboard commands = %#v", windows)
	}
}

func TestOSOpenUsesRunner(t *testing.T) {
	runner := &fakeRunner{}
	os := OS{GOOS: "linux", Runner: runner}

	if err := os.Open("/tmp/a.mp3"); err != nil {
		t.Fatalf("Open: %v", err)
	}
	if runner.startedName != "xdg-open" || !reflect.DeepEqual(runner.startedArgs, []string{"/tmp/a.mp3"}) {
		t.Fatalf("started = %q %#v", runner.startedName, runner.startedArgs)
	}
}

func TestCopyToClipboardFallsBack(t *testing.T) {
	runner := &fakeRunner{runInputFailures: 1}
	os := OS{GOOS: "linux", Runner: runner}

	if err := os.CopyToClipboard("/tmp/a.mp3"); err != nil {
		t.Fatalf("CopyToClipboard: %v", err)
	}
	if runner.runInputCalls != 2 {
		t.Fatalf("runInputCalls = %d, want 2", runner.runInputCalls)
	}
	if runner.input != "/tmp/a.mp3" {
		t.Fatalf("input = %q", runner.input)
	}
}

func TestCopyToClipboardReturnsUnavailable(t *testing.T) {
	runner := &fakeRunner{runInputFailures: 10}
	os := OS{GOOS: "darwin", Runner: runner}

	err := os.CopyToClipboard("/tmp/a.mp3")
	if !errors.Is(err, ErrClipboardUnavailable) {
		t.Fatalf("CopyToClipboard error = %v, want ErrClipboardUnavailable", err)
	}
}

func TestCopyFileReferenceFallsBack(t *testing.T) {
	runner := &fakeRunner{runInputFailures: 1}
	os := OS{GOOS: "linux", Runner: runner}

	if err := os.CopyFileReference("/tmp/a.mp3"); err != nil {
		t.Fatalf("CopyFileReference: %v", err)
	}
	if runner.runInputCalls != 2 {
		t.Fatalf("runInputCalls = %d, want 2", runner.runInputCalls)
	}
	if !strings.Contains(runner.input, "file:///tmp/a.mp3") {
		t.Fatalf("input = %q", runner.input)
	}
}

func TestCopyFileReferenceReturnsUnavailable(t *testing.T) {
	runner := &fakeRunner{runInputFailures: 10}
	os := OS{GOOS: "darwin", Runner: runner}

	err := os.CopyFileReference("/tmp/a.mp3")
	if !errors.Is(err, ErrFileClipboardUnavailable) {
		t.Fatalf("CopyFileReference error = %v, want ErrFileClipboardUnavailable", err)
	}
}

type fakeRunner struct {
	startedName      string
	startedArgs      []string
	runFailures      int
	runInputFailures int
	runCalls         int
	runInputCalls    int
	input            string
}

func (r *fakeRunner) Start(name string, args ...string) error {
	r.startedName = name
	r.startedArgs = append([]string(nil), args...)
	return nil
}

func (r *fakeRunner) Run(name string, args ...string) error {
	r.runCalls++
	if r.runCalls <= r.runFailures {
		return errors.New("failed")
	}
	return nil
}

func (r *fakeRunner) RunWithInput(input string, name string, args ...string) error {
	r.runInputCalls++
	r.input = input
	if r.runInputCalls <= r.runInputFailures {
		return errors.New("failed")
	}
	return nil
}
