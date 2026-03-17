package app

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"themis-cli/internal/state"
)

func Run(st state.State, rootNodeID string) error {
	model, err := NewModel(st, rootNodeID)
	if err != nil {
		return err
	}

	program := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := program.Run(); err != nil {
		return fmt.Errorf("run tui: %w", err)
	}
	return nil
}
