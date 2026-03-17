package app

import (
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"themis-cli/internal/state"
)

type RefreshScope string

const (
	RefreshScopeNode    RefreshScope = "node"
	RefreshScopeSubtree RefreshScope = "subtree"
	RefreshScopeFull    RefreshScope = "full"
)

type RefreshRequest struct {
	Scope        RefreshScope
	TargetNodeID string
	TargetURL    string
	Depth        int
}

type RefreshOutcome struct {
	State        state.State
	Scope        RefreshScope
	TargetNodeID string
	UpdatedNodes int
	DurationMs   int64
	Warnings     []string
	Err          error
}

type RefreshExecutor func(st state.State, req RefreshRequest) RefreshOutcome

type refreshFinishedMsg struct {
	Outcome RefreshOutcome
}

type Model struct {
	st                  state.State
	rootNodeID          string
	linkedRootNodeID    string
	subtreeRefreshDepth int
	refreshExecutor     RefreshExecutor
	refreshInFlight     bool
	expanded            map[string]bool
	flat                []treeRow
	selectedIndex       int
	selectedNodeID      string
	width               int
	height              int
	mode                string
	filter              string
	statusText          string
}

type treeRow struct {
	NodeID      string
	Depth       int
	Title       string
	URL         string
	Status      state.Status
	HasChildren bool
	Expanded    bool
	ParentID    string
}

func NewModel(cfg Config) (Model, error) {
	st := cfg.State
	if len(st.Nodes) == 0 {
		return Model{}, fmt.Errorf("state has no nodes; run discovery first")
	}

	resolvedRootID := cfg.RootNodeID
	if resolvedRootID == "" {
		resolvedRootID = defaultRootNodeID(st)
	}
	if resolvedRootID == "" {
		return Model{}, fmt.Errorf("could not resolve root node for tui")
	}
	if _, ok := st.Nodes[resolvedRootID]; !ok {
		return Model{}, fmt.Errorf("resolved root node %s not found in state", resolvedRootID)
	}

	now := time.Now().UTC()
	_, _ = state.ApplyStateStalePolicy(&st, now, 2*time.Hour)

	depth := cfg.SubtreeRefreshDepth
	if depth <= 0 {
		depth = 1
	}

	m := Model{
		st:                  st,
		rootNodeID:          resolvedRootID,
		linkedRootNodeID:    strings.TrimSpace(cfg.LinkedRootNodeID),
		subtreeRefreshDepth: depth,
		refreshExecutor:     cfg.RefreshExecutor,
		expanded: map[string]bool{
			resolvedRootID: true,
		},
		selectedNodeID: resolvedRootID,
		selectedIndex:  0,
		width:          120,
		height:         30,
		mode:           "browse",
		filter:         "",
		statusText:     "Cached view (refresh actions enabled)",
	}
	m.rebuildFlat()
	return m, nil
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case refreshFinishedMsg:
		m.refreshInFlight = false
		out := msg.Outcome
		if out.Err != nil {
			m.statusText = fmt.Sprintf("refresh failed (%s): %v", out.Scope, out.Err)
			return m, nil
		}
		m.st = out.State
		m.ensureVisible(out.TargetNodeID)
		m.rebuildFlat()
		m.selectedNodeID = out.TargetNodeID
		m.syncSelectedIndex()
		m.statusText = fmt.Sprintf("refresh finished: scope=%s updated=%d duration=%dms", out.Scope, out.UpdatedNodes, out.DurationMs)
		if len(out.Warnings) > 0 {
			m.statusText += fmt.Sprintf(" warnings=%d", len(out.Warnings))
		}
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "up", "k":
			if m.selectedIndex > 0 {
				m.selectedIndex--
				m.syncSelectedNodeID()
			}
		case "down", "j":
			if m.selectedIndex < len(m.flat)-1 {
				m.selectedIndex++
				m.syncSelectedNodeID()
			}
		case "left", "h":
			m.collapseOrMoveToParent()
		case "right", "l", "enter":
			m.expandSelection()
		case "g":
			m.selectedIndex = 0
			m.syncSelectedNodeID()
		case "G":
			if len(m.flat) > 0 {
				m.selectedIndex = len(m.flat) - 1
				m.syncSelectedNodeID()
			}
		case "p":
			if m.linkedRootNodeID == "" {
				m.statusText = "no linked project root configured"
				return m, nil
			}
			if !m.ensureVisible(m.linkedRootNodeID) {
				m.statusText = "linked project root not found in state"
				return m, nil
			}
			m.selectedNodeID = m.linkedRootNodeID
			m.syncSelectedIndex()
			m.statusText = "jumped to linked project root"
			return m, nil
		case "r":
			return m.startRefresh(RefreshScopeNode, 0)
		case "R":
			return m.startRefresh(RefreshScopeSubtree, m.subtreeRefreshDepth)
		case "f":
			return m.startRefresh(RefreshScopeFull, m.subtreeRefreshDepth)
		}
	}
	return m, nil
}

func (m Model) View() string {
	if len(m.flat) == 0 {
		return "No nodes to display"
	}

	treeWidth := m.width / 2
	if treeWidth < 32 {
		treeWidth = 32
	}
	detailsWidth := m.width - treeWidth - 1
	if detailsWidth < 30 {
		detailsWidth = 30
	}

	treePane := lipgloss.NewStyle().Width(treeWidth).Height(maxInt(1, m.height-2)).Border(lipgloss.RoundedBorder()).Render(m.renderTree())
	detailsPane := lipgloss.NewStyle().Width(detailsWidth).Height(maxInt(1, m.height-2)).Border(lipgloss.RoundedBorder()).Render(m.renderDetails())
	statusPane := lipgloss.NewStyle().Width(m.width).Border(lipgloss.RoundedBorder()).Render(m.renderStatus())

	top := lipgloss.JoinHorizontal(lipgloss.Top, treePane, detailsPane)
	return lipgloss.JoinVertical(lipgloss.Left, top, statusPane)
}

func (m Model) startRefresh(scope RefreshScope, depth int) (tea.Model, tea.Cmd) {
	if m.refreshInFlight {
		m.statusText = "refresh already in progress"
		return m, nil
	}
	if m.refreshExecutor == nil {
		m.statusText = "refresh unavailable (read-only mode)"
		return m, nil
	}
	node := m.selectedNode()
	if node == nil {
		m.statusText = "no selected node"
		return m, nil
	}
	targetID := node.ID
	targetURL := node.CanonicalURL
	if scope == RefreshScopeFull {
		targetID = m.rootNodeID
		targetNode := m.st.Nodes[targetID]
		targetURL = targetNode.CanonicalURL
	}
	if depth < 0 {
		depth = 0
	}

	m.refreshInFlight = true
	m.statusText = fmt.Sprintf("refreshing %s...", scope)
	stSnapshot := m.st
	exec := m.refreshExecutor
	req := RefreshRequest{
		Scope:        scope,
		TargetNodeID: targetID,
		TargetURL:    targetURL,
		Depth:        depth,
	}

	cmd := func() tea.Msg {
		out := exec(stSnapshot, req)
		return refreshFinishedMsg{Outcome: out}
	}
	return m, cmd
}

func (m *Model) rebuildFlat() {
	rows := make([]treeRow, 0)
	visited := map[string]bool{}

	var walk func(nodeID string, parentID string, depth int)
	walk = func(nodeID string, parentID string, depth int) {
		node, ok := m.st.Nodes[nodeID]
		if !ok {
			return
		}
		row := treeRow{
			NodeID:      nodeID,
			Depth:       depth,
			Title:       displayTitle(node),
			URL:         node.CanonicalURL,
			Status:      node.Status,
			HasChildren: len(node.ChildIDs) > 0,
			Expanded:    m.expanded[nodeID],
			ParentID:    parentID,
		}
		rows = append(rows, row)

		if !m.expanded[nodeID] {
			return
		}

		for _, childID := range sortedNodeChildren(m.st, node.ChildIDs) {
			if visited[childID] {
				continue
			}
			visited[childID] = true
			walk(childID, nodeID, depth+1)
		}
	}

	visited[m.rootNodeID] = true
	walk(m.rootNodeID, "", 0)
	m.flat = rows
	m.syncSelectedIndex()
}

func (m *Model) ensureVisible(nodeID string) bool {
	if nodeID == "" {
		return false
	}
	node, ok := m.st.Nodes[nodeID]
	if !ok {
		return false
	}
	current := node
	for len(current.ParentIDs) > 0 {
		parentID := current.ParentIDs[0]
		m.expanded[parentID] = true
		parent, ok := m.st.Nodes[parentID]
		if !ok {
			break
		}
		current = parent
	}
	return true
}

func (m *Model) syncSelectedIndex() {
	if len(m.flat) == 0 {
		m.selectedIndex = 0
		m.selectedNodeID = ""
		return
	}
	if m.selectedNodeID != "" {
		for i := range m.flat {
			if m.flat[i].NodeID == m.selectedNodeID {
				m.selectedIndex = i
				return
			}
		}
	}
	if m.selectedIndex < 0 {
		m.selectedIndex = 0
	}
	if m.selectedIndex >= len(m.flat) {
		m.selectedIndex = len(m.flat) - 1
	}
	m.selectedNodeID = m.flat[m.selectedIndex].NodeID
}

func (m *Model) syncSelectedNodeID() {
	if len(m.flat) == 0 {
		m.selectedNodeID = ""
		return
	}
	if m.selectedIndex < 0 {
		m.selectedIndex = 0
	}
	if m.selectedIndex >= len(m.flat) {
		m.selectedIndex = len(m.flat) - 1
	}
	m.selectedNodeID = m.flat[m.selectedIndex].NodeID
}

func (m *Model) selectedRow() *treeRow {
	if len(m.flat) == 0 || m.selectedIndex < 0 || m.selectedIndex >= len(m.flat) {
		return nil
	}
	row := m.flat[m.selectedIndex]
	return &row
}

func (m *Model) selectedNode() *state.Node {
	row := m.selectedRow()
	if row == nil {
		return nil
	}
	node, ok := m.st.Nodes[row.NodeID]
	if !ok {
		return nil
	}
	return &node
}

func (m *Model) expandSelection() {
	row := m.selectedRow()
	if row == nil || !row.HasChildren {
		return
	}
	m.expanded[row.NodeID] = true
	m.rebuildFlat()
}

func (m *Model) collapseOrMoveToParent() {
	row := m.selectedRow()
	if row == nil {
		return
	}
	if row.HasChildren && m.expanded[row.NodeID] {
		delete(m.expanded, row.NodeID)
		m.rebuildFlat()
		return
	}
	if row.ParentID != "" {
		m.selectedNodeID = row.ParentID
		m.rebuildFlat()
	}
}

func (m Model) renderTree() string {
	lines := make([]string, 0, len(m.flat)+1)
	lines = append(lines, "Tree")
	for i, row := range m.flat {
		prefix := "  "
		if i == m.selectedIndex {
			prefix = "> "
		}
		indent := strings.Repeat("  ", row.Depth)
		expandGlyph := "-"
		if row.HasChildren {
			if row.Expanded {
				expandGlyph = "v"
			} else {
				expandGlyph = ">"
			}
		}
		line := fmt.Sprintf("%s%s%s [%s] %s", prefix, indent, expandGlyph, row.Status, row.Title)
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func (m Model) renderDetails() string {
	node := m.selectedNode()
	if node == nil {
		return "Details\n(no selection)"
	}

	lines := []string{
		"Details",
		fmt.Sprintf("ID: %s", node.ID),
		fmt.Sprintf("Title: %s", displayTitle(*node)),
		fmt.Sprintf("Kind: %s", node.Kind),
		fmt.Sprintf("Status: %s", node.Status),
		fmt.Sprintf("URL: %s", node.CanonicalURL),
		fmt.Sprintf("Nav API: %s", node.NavAPIURL),
		fmt.Sprintf("Children: %d", len(node.ChildIDs)),
		fmt.Sprintf("Assets: %d", len(node.Assets)),
	}
	if node.LastSuccessAt != nil {
		lines = append(lines, fmt.Sprintf("Last success: %s", node.LastSuccessAt.Format(time.RFC3339)))
	}
	if node.LastFetchedAt != nil {
		lines = append(lines, fmt.Sprintf("Last fetched: %s", node.LastFetchedAt.Format(time.RFC3339)))
	}
	if node.LastError != "" {
		lines = append(lines, fmt.Sprintf("Last error: %s", node.LastError))
	}
	if breadcrumb, ok := node.Details["breadcrumb"]; ok {
		lines = append(lines, fmt.Sprintf("Breadcrumb: %v", breadcrumb))
	}
	if links, ok := node.Details["links"]; ok {
		lines = append(lines, fmt.Sprintf("Links: %v", links))
	}
	return strings.Join(lines, "\n")
}

func (m Model) renderStatus() string {
	selected := ""
	if row := m.selectedRow(); row != nil {
		selected = row.NodeID
	}
	inFlight := "idle"
	if m.refreshInFlight {
		inFlight = "refreshing"
	}
	return fmt.Sprintf("Status | nodes:%d visible:%d selected:%s | mode:%s/%s | keys: up/down left/right enter r R f p g G q | %s", len(m.st.Nodes), len(m.flat), selected, m.mode, inFlight, m.statusText)
}

func defaultRootNodeID(st state.State) string {
	if len(st.Roots) > 0 {
		if _, ok := st.Nodes[st.Roots[0].NodeID]; ok {
			return st.Roots[0].NodeID
		}
	}

	candidates := make([]string, 0)
	for id, node := range st.Nodes {
		if len(node.ParentIDs) == 0 {
			candidates = append(candidates, id)
		}
	}
	if len(candidates) == 0 {
		for id := range st.Nodes {
			candidates = append(candidates, id)
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		left := st.Nodes[candidates[i]]
		right := st.Nodes[candidates[j]]
		lt := strings.ToLower(displayTitle(left))
		rt := strings.ToLower(displayTitle(right))
		if lt != rt {
			return lt < rt
		}
		return left.CanonicalURL < right.CanonicalURL
	})
	if len(candidates) == 0 {
		return ""
	}
	return candidates[0]
}

func sortedNodeChildren(st state.State, childIDs []string) []string {
	ids := make([]string, 0, len(childIDs))
	for _, id := range childIDs {
		if _, ok := st.Nodes[id]; ok {
			ids = append(ids, id)
		}
	}
	sort.SliceStable(ids, func(i, j int) bool {
		left := st.Nodes[ids[i]]
		right := st.Nodes[ids[j]]
		lt := strings.ToLower(displayTitle(left))
		rt := strings.ToLower(displayTitle(right))
		if lt != rt {
			return lt < rt
		}
		return left.CanonicalURL < right.CanonicalURL
	})
	return ids
}

func displayTitle(node state.Node) string {
	title := strings.TrimSpace(node.Title)
	if title != "" {
		return title
	}
	if node.CanonicalURL != "" {
		return node.CanonicalURL
	}
	return node.ID
}

func maxInt(a int, b int) int {
	if a > b {
		return a
	}
	return b
}
