package app

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"themis-cli/internal/state"
)

func TestNewModelAndNavigation(t *testing.T) {
	now := time.Date(2026, 3, 17, 12, 0, 0, 0, time.UTC)
	st := state.NewEmptyState()
	st.Nodes = map[string]state.Node{
		"url:root": {
			ID:           "url:root",
			Title:        "Operating Systems",
			Kind:         "course",
			CanonicalURL: "https://themis.housing.rug.nl/course/2025-2026/os",
			ChildIDs:     []string{"url:lab1", "url:lab2"},
			Status:       state.StatusOK,
			LastSuccessAt: func() *time.Time {
				t := now
				return &t
			}(),
		},
		"url:lab1": {
			ID:           "url:lab1",
			Title:        "Lab 1",
			Kind:         "assignment",
			CanonicalURL: "https://themis.housing.rug.nl/course/2025-2026/os/lab1",
			ParentIDs:    []string{"url:root"},
			Status:       state.StatusOK,
			LastSuccessAt: func() *time.Time {
				t := now
				return &t
			}(),
		},
		"url:lab2": {
			ID:           "url:lab2",
			Title:        "Lab 2",
			Kind:         "assignment",
			CanonicalURL: "https://themis.housing.rug.nl/course/2025-2026/os/lab2",
			ParentIDs:    []string{"url:root"},
			Status:       state.StatusOK,
			LastSuccessAt: func() *time.Time {
				t := now
				return &t
			}(),
		},
	}
	st.Roots = []state.RootRef{{NodeID: "url:root", CanonicalURL: st.Nodes["url:root"].CanonicalURL, UpdatedAt: now}}

	m, err := NewModel(st, "")
	if err != nil {
		t.Fatalf("new model failed: %v", err)
	}
	if len(m.flat) != 3 {
		t.Fatalf("expected 3 visible rows, got %d", len(m.flat))
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = updated.(Model)
	if m.selectedNodeID == "url:root" {
		t.Fatalf("expected selection to move down")
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	m = updated.(Model)
	if m.selectedNodeID != "url:root" {
		t.Fatalf("expected selection to move to parent")
	}
}

func TestCollapseAndExpand(t *testing.T) {
	now := time.Date(2026, 3, 17, 12, 0, 0, 0, time.UTC)
	st := state.NewEmptyState()
	st.Nodes = map[string]state.Node{
		"url:root": {
			ID:           "url:root",
			Title:        "Root",
			CanonicalURL: "https://themis.housing.rug.nl/course",
			ChildIDs:     []string{"url:child"},
			Status:       state.StatusOK,
			LastSuccessAt: func() *time.Time {
				t := now
				return &t
			}(),
		},
		"url:child": {
			ID:           "url:child",
			Title:        "Child",
			CanonicalURL: "https://themis.housing.rug.nl/course/2025-2026",
			ParentIDs:    []string{"url:root"},
			Status:       state.StatusOK,
			LastSuccessAt: func() *time.Time {
				t := now
				return &t
			}(),
		},
	}
	st.Roots = []state.RootRef{{NodeID: "url:root", CanonicalURL: st.Nodes["url:root"].CanonicalURL, UpdatedAt: now}}

	m, err := NewModel(st, "url:root")
	if err != nil {
		t.Fatalf("new model failed: %v", err)
	}
	if len(m.flat) != 2 {
		t.Fatalf("expected expanded root with 2 rows, got %d", len(m.flat))
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	m = updated.(Model)
	if len(m.flat) != 1 {
		t.Fatalf("expected collapsed root with 1 row, got %d", len(m.flat))
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	m = updated.(Model)
	if len(m.flat) != 2 {
		t.Fatalf("expected expanded root with 2 rows after expand, got %d", len(m.flat))
	}
}
