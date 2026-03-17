package app

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"themis-cli/internal/state"
)

func baseStateForTUI(now time.Time) state.State {
	st := state.NewEmptyState()
	st.Nodes = map[string]state.Node{
		"url:root": {
			ID:           "url:root",
			Title:        "Operating Systems",
			Kind:         "course",
			CanonicalURL: "https://themis.housing.rug.nl/course/2025-2026/os",
			ChildIDs:     []string{"url:lab1", "url:lab2"},
			Assets: []state.AssetRef{
				{Name: "1.in", URL: "https://themis.housing.rug.nl/file/course/%40tests/1.in", Kind: "file"},
				{Name: "1.out", URL: "https://themis.housing.rug.nl/file/course/%40tests/1.out", Kind: "file"},
			},
			Status: state.StatusOK,
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
	return st
}

func TestNewModelAndNavigation(t *testing.T) {
	now := time.Date(2026, 3, 17, 12, 0, 0, 0, time.UTC)
	m, err := NewModel(Config{State: baseStateForTUI(now)})
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
	m, err := NewModel(Config{State: baseStateForTUI(now), RootNodeID: "url:root"})
	if err != nil {
		t.Fatalf("new model failed: %v", err)
	}
	if len(m.flat) != 3 {
		t.Fatalf("expected expanded root with 3 rows, got %d", len(m.flat))
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	m = updated.(Model)
	if len(m.flat) != 1 {
		t.Fatalf("expected collapsed root with 1 row, got %d", len(m.flat))
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	m = updated.(Model)
	if len(m.flat) != 3 {
		t.Fatalf("expected expanded root with 3 rows after expand, got %d", len(m.flat))
	}
}

func TestRefreshKeyFlowAndJumpProjectRoot(t *testing.T) {
	now := time.Date(2026, 3, 17, 12, 0, 0, 0, time.UTC)
	executorCalled := false
	exec := func(st state.State, req RefreshRequest) RefreshOutcome {
		executorCalled = true
		node := st.Nodes[req.TargetNodeID]
		node.Title = node.Title + " *"
		st.Nodes[req.TargetNodeID] = node
		return RefreshOutcome{State: st, Scope: req.Scope, TargetNodeID: req.TargetNodeID, UpdatedNodes: 1, DurationMs: 5}
	}

	m, err := NewModel(Config{State: baseStateForTUI(now), LinkedRootNodeID: "url:root", RefreshExecutor: exec})
	if err != nil {
		t.Fatalf("new model failed: %v", err)
	}

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	m = updated.(Model)
	if cmd == nil {
		t.Fatalf("expected refresh command")
	}
	msg := cmd()
	updated, _ = m.Update(msg)
	m = updated.(Model)
	if !executorCalled {
		t.Fatalf("expected executor to run")
	}
	if m.st.Nodes["url:root"].Title != "Operating Systems *" {
		t.Fatalf("expected refreshed state applied")
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	m = updated.(Model)
	if m.selectedNodeID != "url:root" {
		t.Fatalf("expected jump to linked root")
	}
}

func TestRefreshFailureStatus(t *testing.T) {
	now := time.Date(2026, 3, 17, 12, 0, 0, 0, time.UTC)
	exec := func(st state.State, req RefreshRequest) RefreshOutcome {
		return RefreshOutcome{State: st, Scope: req.Scope, TargetNodeID: req.TargetNodeID, Err: errors.New("boom")}
	}

	m, err := NewModel(Config{State: baseStateForTUI(now), RefreshExecutor: exec})
	if err != nil {
		t.Fatalf("new model failed: %v", err)
	}

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	m = updated.(Model)
	if cmd == nil {
		t.Fatalf("expected refresh command")
	}
	msg := cmd()
	updated, _ = m.Update(msg)
	m = updated.(Model)
	if m.statusText == "" || m.statusText == "Cached view (refresh actions enabled)" {
		t.Fatalf("expected failure status text")
	}
}

func TestDownloadFlow(t *testing.T) {
	now := time.Date(2026, 3, 17, 12, 0, 0, 0, time.UTC)
	exec := func(st state.State, req DownloadRequest) DownloadOutcome {
		return DownloadOutcome{
			NodeID:     req.NodeID,
			TargetDir:  req.TargetDir,
			Downloaded: len(req.Assets),
			DurationMs: 9,
			Files: []DownloadedFile{
				{Name: "1.in", URL: "https://themis.housing.rug.nl/file/course/%40tests/1.in", Path: "/tmp/tests/1.in"},
			},
		}
	}

	persisted := false
	persist := func(nodeID string, urls []string, targetDir string) error {
		persisted = true
		if nodeID != "url:root" || targetDir == "" || len(urls) == 0 {
			t.Fatalf("unexpected persist payload")
		}
		return nil
	}

	m, err := NewModel(Config{
		State:              baseStateForTUI(now),
		DownloadExecutor:   exec,
		DefaultDownloadDir: "/tmp/tests",
		PersistChoices:     persist,
	})
	if err != nil {
		t.Fatalf("new model failed: %v", err)
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	m = updated.(Model)
	if m.mode != "download" {
		t.Fatalf("expected download mode")
	}

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if cmd == nil {
		t.Fatalf("expected download command")
	}
	msg := cmd()
	updated, _ = m.Update(msg)
	m = updated.(Model)
	if m.mode != "browse" {
		t.Fatalf("expected browse mode after download")
	}
	if !persisted {
		t.Fatalf("expected persisted choices callback")
	}
}

func TestDownloadViewportKeepsHeaderAndCursorVisible(t *testing.T) {
	now := time.Date(2026, 3, 17, 12, 0, 0, 0, time.UTC)
	st := baseStateForTUI(now)
	root := st.Nodes["url:root"]
	root.Assets = make([]state.AssetRef, 0, 40)
	for i := 1; i <= 40; i++ {
		root.Assets = append(root.Assets, state.AssetRef{
			Name: fmt.Sprintf("%d.in", i),
			URL:  fmt.Sprintf("https://themis.housing.rug.nl/file/course/%%40tests/%d.in", i),
			Kind: "file",
		})
	}
	st.Nodes["url:root"] = root

	m, err := NewModel(Config{State: st})
	if err != nil {
		t.Fatalf("new model failed: %v", err)
	}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	m = updated.(Model)
	m.downloadCursor = 39

	view := m.renderDetailsForSize(48, 14)
	if !strings.Contains(view, "Download") {
		t.Fatalf("expected download header visible")
	}
	if !strings.Contains(view, "> [x] 40.in") {
		t.Fatalf("expected cursor line for last item visible")
	}
}
