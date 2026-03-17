package app

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"themis-cli/internal/state"
)

type Config struct {
	State               state.State
	RootNodeID          string
	LinkedRootNodeID    string
	SubtreeRefreshDepth int
	RefreshExecutor     RefreshExecutor
}

func Run(cfg Config) error {
	model, err := NewModel(cfg)
	if err != nil {
		return err
	}

	program := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := program.Run(); err != nil {
		return fmt.Errorf("run tui: %w", err)
	}
	return nil
}
