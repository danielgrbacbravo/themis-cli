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

	for cmd != nil {
		msg := cmd()
		updated, next := m.Update(msg)
		m = updated.(Model)
		cmd = next
	}

	if m.mode != "download" {
		t.Fatalf("expected download mode to remain for progress summary")
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
	m.adjustDownloadOffset()

	view := m.renderDetailsForSize(48, 14)
	if !strings.Contains(view, "Download") {
		t.Fatalf("expected download header visible")
	}
	if !strings.Contains(view, "> [x] 40.in") {
		t.Fatalf("expected cursor line for last item visible")
	}
}

func TestRenderDetails_ShowsStatsSummary(t *testing.T) {
	now := time.Date(2026, 3, 18, 12, 0, 0, 0, time.UTC)
	st := baseStateForTUI(now)
	lab := st.Nodes["url:lab1"]
	lab.Details = map[string]any{
		"links": map[string]any{
			"status_page": "https://themis.housing.rug.nl/stats/2025-2026/os/lab1",
		},
		"stats": map[string]any{
			"status_page": "https://themis.housing.rug.nl/stats/2025-2026/os/lab1",
			"summary": map[string]any{
				"status": "passed",
				"grade":  "20.00",
				"group":  "D. Grbac Bravo",
			},
			"counts": map[string]any{
				"total":  8,
				"passed": 1,
			},
			"submission_refs": map[string]any{
				"leading": map[string]any{
					"title": "Exercise 1 / s5482585-7",
					"url":   "https://themis.housing.rug.nl/submission/2025-2026/os/lab1/s5482585-7",
				},
			},
		},
	}
	st.Nodes["url:lab1"] = lab

	m, err := NewModel(Config{State: st})
	if err != nil {
		t.Fatalf("new model failed: %v", err)
	}
	// Move selection from root to lab1.
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = updated.(Model)

	view := m.renderDetailsForSize(120, 40)
	assertContains := func(needle string) {
		if !strings.Contains(view, needle) {
			t.Fatalf("expected details view to contain %q\nview:\n%s", needle, view)
		}
	}

	assertContains("Stats:")
	assertContains("- Page: https://themis.housing.rug.nl/stats/2025-2026/os/lab1")
	assertContains("- Status: passed")
	assertContains("- Grade: 20.00")
	assertContains("- Group: D. Grbac Bravo")
	assertContains("- Counts: total=8, passed=1")
	assertContains("- Leading: Exercise 1 / s5482585-7")
}

func TestRenderDetails_ShowsStatsLoadHintWhenOnlyStatusPageExists(t *testing.T) {
	now := time.Date(2026, 3, 18, 12, 0, 0, 0, time.UTC)
	st := baseStateForTUI(now)
	lab := st.Nodes["url:lab1"]
	lab.Details = map[string]any{
		"links": map[string]any{
			"status_page": "https://themis.housing.rug.nl/stats/2025-2026/os/lab1",
		},
	}
	st.Nodes["url:lab1"] = lab

	m, err := NewModel(Config{State: st})
	if err != nil {
		t.Fatalf("new model failed: %v", err)
	}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = updated.(Model)

	view := m.renderDetailsForSize(120, 30)
	if !strings.Contains(view, "Status page: https://themis.housing.rug.nl/stats/2025-2026/os/lab1") {
		t.Fatalf("expected status page in details view\nview:\n%s", view)
	}
	if !strings.Contains(view, "Stats: not loaded yet (refresh this node)") {
		t.Fatalf("expected stats load hint in details view\nview:\n%s", view)
	}
}

func TestRenderTree_AssignmentResultLabelsAndFreshnessIndicator(t *testing.T) {
	now := time.Date(2026, 3, 18, 12, 0, 0, 0, time.UTC)
	st := baseStateForTUI(now)

	root := st.Nodes["url:root"]
	root.ChildIDs = []string{"url:lab1", "url:lab2", "url:lab3", "url:lab4"}
	st.Nodes["url:root"] = root

	lab1 := st.Nodes["url:lab1"]
	lab1.Details = map[string]any{
		"links": map[string]any{"status_page": "https://themis.housing.rug.nl/stats/2025-2026/os/lab1"},
		"stats": map[string]any{"summary": map[string]any{"status": "passed"}},
	}
	lab1.Status = state.StatusOK
	st.Nodes["url:lab1"] = lab1

	lab2 := st.Nodes["url:lab2"]
	lab2.Details = map[string]any{
		"links": map[string]any{"status_page": "https://themis.housing.rug.nl/stats/2025-2026/os/lab2"},
		"stats": map[string]any{"summary": map[string]any{"status": "failed"}},
	}
	lab2.Status = state.StatusStale
	st.Nodes["url:lab2"] = lab2

	st.Nodes["url:lab3"] = state.Node{
		ID:           "url:lab3",
		Title:        "Lab 3",
		Kind:         "assignment",
		CanonicalURL: "https://themis.housing.rug.nl/course/2025-2026/os/lab3",
		ParentIDs:    []string{"url:root"},
		Status:       state.StatusOK,
		Details: map[string]any{
			"links": map[string]any{"status_page": "https://themis.housing.rug.nl/stats/2025-2026/os/lab3"},
		},
	}
	st.Nodes["url:lab4"] = state.Node{
		ID:           "url:lab4",
		Title:        "Lab 4",
		Kind:         "assignment",
		CanonicalURL: "https://themis.housing.rug.nl/course/2025-2026/os/lab4",
		ParentIDs:    []string{"url:root"},
		Status:       state.StatusOK,
		Details: map[string]any{
			"links": map[string]any{"status_page": "https://themis.housing.rug.nl/stats/2025-2026/os/lab4"},
			"stats": map[string]any{
				"summary": map[string]any{
					"grade": "17.50",
				},
			},
		},
	}

	m, err := NewModel(Config{State: st})
	if err != nil {
		t.Fatalf("new model failed: %v", err)
	}
	tree := m.renderTree()

	assertContains := func(needle string) {
		if !strings.Contains(tree, needle) {
			t.Fatalf("expected tree to contain %q\ntree:\n%s", needle, tree)
		}
	}
	assertContains("[passed] Lab 1")
	assertContains("[failing] Lab 2")
	assertContains("(stale)")
	assertContains("[not_submitted] Lab 3")
	assertContains("[17.50] Lab 4")
}
