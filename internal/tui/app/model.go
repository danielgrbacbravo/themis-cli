package app

import (
	"fmt"
	"path/filepath"
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

type DownloadRequest struct {
	NodeID    string
	TargetDir string
	Assets    []state.AssetRef
}

type DownloadedFile struct {
	Name string
	URL  string
	Path string
}

type DownloadOutcome struct {
	NodeID     string
	TargetDir  string
	Downloaded int
	Files      []DownloadedFile
	DurationMs int64
	Err        error
}

type DownloadExecutor func(st state.State, req DownloadRequest) DownloadOutcome
type PersistChoicesFunc func(nodeID string, assetURLs []string, targetDir string) error

type refreshFinishedMsg struct {
	Outcome RefreshOutcome
}

type downloadFinishedMsg struct {
	Outcome DownloadOutcome
}

type Model struct {
	st                  state.State
	rootNodeID          string
	linkedRootNodeID    string
	subtreeRefreshDepth int
	refreshExecutor     RefreshExecutor
	downloadExecutor    DownloadExecutor
	persistChoices      PersistChoicesFunc
	defaultDownloadDir  string
	recentAssetChoices  map[string][]string
	downloadSelection   map[string]bool
	downloadCursor      int
	downloadInFlight    bool
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
		downloadExecutor:    cfg.DownloadExecutor,
		persistChoices:      cfg.PersistChoices,
		defaultDownloadDir:  strings.TrimSpace(cfg.DefaultDownloadDir),
		recentAssetChoices:  cloneChoiceMap(cfg.RecentAssetChoices),
		downloadSelection:   map[string]bool{},
		expanded: map[string]bool{
			resolvedRootID: true,
		},
		selectedNodeID: resolvedRootID,
		selectedIndex:  0,
		width:          0,
		height:         0,
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
	case downloadFinishedMsg:
		m.downloadInFlight = false
		m.mode = "browse"
		out := msg.Outcome
		if out.Err != nil {
			m.statusText = fmt.Sprintf("download failed: %v", out.Err)
			return m, nil
		}
		if m.persistChoices != nil {
			selected := m.selectedAssetURLs()
			if err := m.persistChoices(out.NodeID, selected, out.TargetDir); err != nil {
				m.statusText = fmt.Sprintf("download finished (%d files), persist failed: %v", out.Downloaded, err)
				return m, nil
			}
			m.recentAssetChoices[out.NodeID] = selected
		}
		m.defaultDownloadDir = out.TargetDir
		m.statusText = fmt.Sprintf("download finished: %d files to %s in %dms", out.Downloaded, out.TargetDir, out.DurationMs)
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "up", "k":
			if m.mode == "download" {
				if m.downloadCursor > 0 {
					m.downloadCursor--
				}
				return m, nil
			}
			if m.selectedIndex > 0 {
				m.selectedIndex--
				m.syncSelectedNodeID()
			}
		case "down", "j":
			if m.mode == "download" {
				assets := m.selectedNodeAssets()
				if m.downloadCursor < len(assets)-1 {
					m.downloadCursor++
				}
				return m, nil
			}
			if m.selectedIndex < len(m.flat)-1 {
				m.selectedIndex++
				m.syncSelectedNodeID()
			}
		case "left", "h":
			if m.mode == "download" {
				m.mode = "browse"
				m.statusText = "download mode closed"
				return m, nil
			}
			m.collapseOrMoveToParent()
		case "right", "l":
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
		case "d":
			if m.mode == "download" {
				m.mode = "browse"
				m.statusText = "download mode closed"
				return m, nil
			}
			return m.openDownloadMode()
		case " ":
			if m.mode == "download" {
				m.toggleDownloadSelectionAtCursor()
				return m, nil
			}
		case "a":
			if m.mode == "download" {
				for _, asset := range m.selectedNodeAssets() {
					m.downloadSelection[asset.URL] = true
				}
				return m, nil
			}
		case "c":
			if m.mode == "download" {
				m.downloadSelection = map[string]bool{}
				return m, nil
			}
		case "r":
			return m.startRefresh(RefreshScopeNode, 0)
		case "R":
			return m.startRefresh(RefreshScopeSubtree, m.subtreeRefreshDepth)
		case "f":
			return m.startRefresh(RefreshScopeFull, m.subtreeRefreshDepth)
		case "enter":
			if m.mode == "download" {
				return m.startDownload()
			}
			m.expandSelection()
		}
	}
	return m, nil
}

func (m Model) View() string {
	if len(m.flat) == 0 {
		return "No nodes to display"
	}
	if m.width <= 0 || m.height <= 0 {
		return "Loading UI..."
	}

	availableWidth := maxInt(20, m.width)
	availableHeight := maxInt(6, m.height)

	statusStyle := lipgloss.NewStyle().Border(lipgloss.RoundedBorder())
	statusFrameW, statusFrameH := statusStyle.GetFrameSize()
	statusContentWidth := maxInt(1, availableWidth-statusFrameW)
	statusPane := statusStyle.Width(statusContentWidth).Render(truncateOneLine(m.renderStatus(), statusContentWidth))
	statusTotalHeight := lipgloss.Height(statusPane)
	if statusTotalHeight < statusFrameH+1 {
		statusTotalHeight = statusFrameH + 1
	}

	topHeight := availableHeight - statusTotalHeight
	if topHeight < 3 {
		topHeight = 3
	}

	panelStyle := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Align(lipgloss.Left, lipgloss.Top)
	panelFrameW, panelFrameH := panelStyle.GetFrameSize()
	leftOuterWidth := maxInt(10, availableWidth/2)
	rightOuterWidth := maxInt(10, availableWidth-leftOuterWidth)
	leftContentWidth := maxInt(1, leftOuterWidth-panelFrameW)
	rightContentWidth := maxInt(1, rightOuterWidth-panelFrameW)
	panelContentHeight := maxInt(1, topHeight-panelFrameH)

	treePane := panelStyle.Width(leftContentWidth).Height(panelContentHeight).Render(clipTopLines(m.renderTree(), panelContentHeight))
	detailsPane := panelStyle.Width(rightContentWidth).Height(panelContentHeight).Render(clipTopLines(m.renderDetails(), panelContentHeight))

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

func (m Model) openDownloadMode() (tea.Model, tea.Cmd) {
	if m.downloadInFlight {
		m.statusText = "download already in progress"
		return m, nil
	}
	assets := m.selectedNodeAssets()
	if len(assets) == 0 {
		m.statusText = "selected node has no assets"
		return m, nil
	}
	m.mode = "download"
	m.downloadCursor = 0
	m.downloadSelection = map[string]bool{}

	if recent, ok := m.recentAssetChoices[m.selectedNodeID]; ok && len(recent) > 0 {
		for _, url := range recent {
			m.downloadSelection[url] = true
		}
	} else {
		for _, asset := range assets {
			m.downloadSelection[asset.URL] = true
		}
	}

	m.statusText = fmt.Sprintf("download mode: %d assets selected", len(m.selectedAssetURLs()))
	return m, nil
}

func (m *Model) toggleDownloadSelectionAtCursor() {
	assets := m.selectedNodeAssets()
	if len(assets) == 0 {
		return
	}
	if m.downloadCursor < 0 {
		m.downloadCursor = 0
	}
	if m.downloadCursor >= len(assets) {
		m.downloadCursor = len(assets) - 1
	}
	asset := assets[m.downloadCursor]
	if m.downloadSelection[asset.URL] {
		delete(m.downloadSelection, asset.URL)
	} else {
		m.downloadSelection[asset.URL] = true
	}
}

func (m Model) startDownload() (tea.Model, tea.Cmd) {
	if m.downloadInFlight {
		m.statusText = "download already in progress"
		return m, nil
	}
	if m.downloadExecutor == nil {
		m.statusText = "download unavailable"
		return m, nil
	}
	node := m.selectedNode()
	if node == nil {
		m.statusText = "no selected node"
		return m, nil
	}

	selected := m.selectedAssets()
	if len(selected) == 0 {
		m.statusText = "no assets selected"
		return m, nil
	}

	targetDir := m.defaultDownloadDir
	if strings.TrimSpace(targetDir) == "" {
		targetDir = filepath.Join(".", "tests")
	}

	m.downloadInFlight = true
	m.statusText = fmt.Sprintf("downloading %d assets to %s...", len(selected), targetDir)
	stSnapshot := m.st
	exec := m.downloadExecutor
	req := DownloadRequest{
		NodeID:    node.ID,
		TargetDir: targetDir,
		Assets:    selected,
	}

	cmd := func() tea.Msg {
		out := exec(stSnapshot, req)
		return downloadFinishedMsg{Outcome: out}
	}
	return m, cmd
}

func (m Model) selectedNodeAssets() []state.AssetRef {
	node := m.selectedNode()
	if node == nil {
		return []state.AssetRef{}
	}
	return append([]state.AssetRef{}, node.Assets...)
}

func (m Model) selectedAssets() []state.AssetRef {
	all := m.selectedNodeAssets()
	out := make([]state.AssetRef, 0, len(all))
	for _, asset := range all {
		if m.downloadSelection[asset.URL] {
			out = append(out, asset)
		}
	}
	return out
}

func (m Model) selectedAssetURLs() []string {
	selected := m.selectedAssets()
	out := make([]string, 0, len(selected))
	for _, asset := range selected {
		out = append(out, asset.URL)
	}
	return out
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
	if m.mode == "download" {
		return m.renderDownloadPanel(*node)
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
	selected := "none"
	if row := m.selectedRow(); row != nil {
		selected = row.Title
	}
	inFlight := "idle"
	if m.refreshInFlight {
		inFlight = "refreshing"
	}
	if m.downloadInFlight {
		inFlight = "downloading"
	}
	keys := "j/k move h/l fold enter open r node R subtree f full d download p project q quit"
	if m.mode == "download" {
		keys = "j/k move space toggle a all c clear enter download h/d close q quit"
	}
	msg := strings.TrimSpace(m.statusText)
	if msg != "" {
		return fmt.Sprintf("%s | %s |  %s | %s | %s", strings.ToUpper(m.mode), inFlight, selected, keys, msg)
	}
	return fmt.Sprintf("%s | %s |  %s | %s", strings.ToUpper(m.mode), inFlight, selected, keys)
}

func (m Model) renderDownloadPanel(node state.Node) string {
	assets := m.selectedNodeAssets()
	lines := []string{
		"Download",
		fmt.Sprintf("Node: %s", displayTitle(node)),
		fmt.Sprintf("Target dir: %s", m.defaultDownloadDir),
		"Keys: up/down move, space toggle, a select-all, c clear, enter download, h/d close",
		"",
	}
	if len(assets) == 0 {
		lines = append(lines, "(no assets available)")
		return strings.Join(lines, "\n")
	}
	for i, asset := range assets {
		cursor := " "
		if i == m.downloadCursor {
			cursor = ">"
		}
		mark := " "
		if m.downloadSelection[asset.URL] {
			mark = "x"
		}
		name := strings.TrimSpace(asset.Name)
		if name == "" {
			name = asset.URL
		}
		lines = append(lines, fmt.Sprintf("%s [%s] %s (%s)", cursor, mark, name, asset.Kind))
	}
	lines = append(lines, "")
	lines = append(lines, fmt.Sprintf("Selected: %d/%d", len(m.selectedAssetURLs()), len(assets)))
	return strings.Join(lines, "\n")
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

func cloneChoiceMap(in map[string][]string) map[string][]string {
	out := make(map[string][]string, len(in))
	for k, v := range in {
		out[k] = append([]string(nil), v...)
	}
	return out
}

func clipTopLines(content string, maxLines int) string {
	if maxLines <= 0 {
		return ""
	}
	lines := strings.Split(content, "\n")
	if len(lines) <= maxLines {
		return content
	}
	return strings.Join(lines[:maxLines], "\n")
}

func truncateOneLine(content string, maxWidth int) string {
	line := strings.Split(content, "\n")[0]
	if maxWidth <= 0 {
		return ""
	}
	r := []rune(line)
	if len(r) <= maxWidth {
		return line
	}
	if maxWidth == 1 {
		return "…"
	}
	return string(r[:maxWidth-1]) + "…"
}
