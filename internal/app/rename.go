package app

import (
	"errors"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"corder/internal/storage"
)

func renameCmd(oldPath, newName string) tea.Cmd {
	return func() tea.Msg {
		newPath, err := renameDestination(oldPath, newName)
		if err != nil {
			return renameResultMsg{oldPath: oldPath, err: err}
		}
		if err := storage.RenameWithMeta(oldPath, newPath); err != nil {
			return renameResultMsg{oldPath: oldPath, newPath: newPath, err: friendlyRenameError(err)}
		}
		return renameResultMsg{oldPath: oldPath, newPath: newPath}
	}
}

func renameDestination(oldPath, newName string) (string, error) {
	if err := validateRenameInput(newName); err != nil {
		return "", err
	}
	name := filepath.Base(strings.TrimSpace(newName))
	if !strings.Contains(name, ".") {
		name += filepath.Ext(oldPath)
	}
	return filepath.Join(filepath.Dir(oldPath), name), nil
}

func validateRenameInput(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("Name cannot be empty")
	}
	if name == "." || name == ".." {
		return errors.New("Name must include filename text")
	}
	if filepath.Base(name) != name {
		return errors.New("Name cannot include folders")
	}
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	if strings.TrimSpace(stem) == "" {
		return errors.New("Name must include filename text")
	}
	return nil
}

func friendlyRenameError(err error) error {
	if errors.Is(err, storage.ErrDestinationExists) {
		return errors.New("A recording with that name already exists")
	}
	return err
}
