package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"themis-cli/internal/state"
)

var (
	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.AdaptiveColor{Light: "33", Dark: "75"})
	mutedStyle    = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "242", Dark: "245"})
	okStyle       = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "28", Dark: "42"})
	staleStyle    = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "136", Dark: "214"})
	errorStyle    = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "160", Dark: "203"})
	neverStyle    = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "240", Dark: "240"})
	passStyle     = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "28", Dark: "42"})
	failStyle     = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "160", Dark: "203"})
	noneStyle     = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "244", Dark: "246"})
	infoStyle     = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "39", Dark: "81"})
	selectedStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.AdaptiveColor{Light: "39", Dark: "81"})
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

type downloadFileFinishedMsg struct {
	AssetURL string
	Outcome  DownloadOutcome
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
	downloadOffset      int
	downloadInFlight    bool
	downloadQueue       []state.AssetRef
	downloadTargetDir   string
	downloadCurrent     int
	downloadDone        int
	downloadFailed      int
	downloadStatus      map[string]string
	downloadErrorByURL  map[string]string
	downloadStartedAt   time.Time
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
	ResultLabel string
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
		downloadQueue:       []state.AssetRef{},
		downloadStatus:      map[string]string{},
		downloadErrorByURL:  map[string]string{},
		downloadOffset:      0,
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
		if m.mode == "download" {
			m.adjustDownloadOffset()
		}
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
	case downloadFileFinishedMsg:
		m.handleDownloadFileResult(msg.AssetURL, msg.Outcome)
		if m.downloadCurrent < len(m.downloadQueue) {
			return m, m.downloadNextCmd()
		}
		return m.finalizeDownloadBatch(), nil
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "up", "k":
			if m.mode == "download" {
				if m.downloadCursor > 0 {
					m.downloadCursor--
				}
				m.adjustDownloadOffset()
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
				m.adjustDownloadOffset()
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
				m.resetDownloadProgress()
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
				m.resetDownloadProgress()
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

	treePane := panelStyle.Width(leftContentWidth).Height(panelContentHeight).Render(m.renderTreeForHeight(panelContentHeight))
	detailsPane := panelStyle.Width(rightContentWidth).Height(panelContentHeight).Render(m.renderDetailsForSize(rightContentWidth, panelContentHeight))

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
	m.downloadOffset = 0
	m.downloadSelection = map[string]bool{}
	m.resetDownloadProgress()

	if recent, ok := m.recentAssetChoices[m.selectedNodeID]; ok && len(recent) > 0 {
		for _, url := range recent {
			m.downloadSelection[url] = true
		}
	} else {
		for _, asset := range assets {
			m.downloadSelection[asset.URL] = true
		}
	}

	m.adjustDownloadOffset()
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
	m.downloadQueue = append([]state.AssetRef{}, selected...)
	m.downloadTargetDir = targetDir
	m.downloadCurrent = 0
	m.downloadDone = 0
	m.downloadFailed = 0
	m.downloadStartedAt = time.Now()
	m.downloadStatus = map[string]string{}
	m.downloadErrorByURL = map[string]string{}
	for _, a := range m.downloadQueue {
		m.downloadStatus[a.URL] = "pending"
	}
	if len(m.downloadQueue) > 0 {
		m.downloadStatus[m.downloadQueue[0].URL] = "active"
	}
	m.statusText = fmt.Sprintf("downloading 0/%d to %s", len(selected), targetDir)
	return m, m.downloadNextCmd()
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
			ResultLabel: nodeResultLabel(node),
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
	lines = append(lines, titleStyle.Render("Tree"))
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
		resultTag := colorResultTag(row.ResultLabel, row.Status)
		freshTag := colorFreshnessTag(row.Status)
		line := fmt.Sprintf("%s%s%s %s %s", prefix, indent, mutedStyle.Render(expandGlyph), resultTag, row.Title)
		if strings.TrimSpace(freshTag) != "" {
			line += " " + freshTag
		}
		if i == m.selectedIndex {
			line = selectedStyle.Render(line)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func (m Model) renderTreeForHeight(maxLines int) string {
	return clipTopLines(m.renderTree(), maxLines)
}

func (m Model) renderDetailsForSize(maxWidth int, maxLines int) string {
	return clipTopLines(m.renderDetails(maxWidth, maxLines), maxLines)
}

func (m Model) renderDetails(maxWidth int, maxLines int) string {
	node := m.selectedNode()
	if node == nil {
		return titleStyle.Render("Details") + "\n" + mutedStyle.Render("(no selection)")
	}
	if m.mode == "download" {
		return m.renderDownloadPanel(*node, maxWidth, maxLines)
	}

	lines := []string{
		titleStyle.Render("Details"),
		fmt.Sprintf("Name: %s", displayTitle(*node)),
		fmt.Sprintf("Type: %s", readableKind(node.Kind)),
		fmt.Sprintf("Result: %s", colorResultWord(nodeResultLabel(*node))),
		fmt.Sprintf("Freshness: %s", colorStatusWord(node.Status)),
	}
	if node.CanonicalURL != "" {
		lines = append(lines, fmt.Sprintf("Path: %s", node.CanonicalURL))
	}

	lines = append(lines, "")
	lines = append(lines, fmt.Sprintf("Children: %d", len(node.ChildIDs)))
	lines = append(lines, fmt.Sprintf("Files: %d", len(node.Assets)))

	if statusPage := statusPageURL(node.Details); statusPage != "" {
		lines = append(lines, fmt.Sprintf("Status page: %s", statusPage))
	}
	if statsLines := statsSummaryLines(node.Details); len(statsLines) > 0 {
		lines = append(lines, "")
		lines = append(lines, titleStyle.Render("Stats:"))
		lines = append(lines, statsLines...)
	} else if statusPageURL(node.Details) != "" {
		lines = append(lines, mutedStyle.Render("Stats: not loaded yet (refresh this node)"))
	}
	if debugLines := statsDebugLines(node.Details); len(debugLines) > 0 {
		lines = append(lines, "")
		lines = append(lines, titleStyle.Render("Stats Debug:"))
		lines = append(lines, debugLines...)
	}

	if node.LastSuccessAt != nil {
		lines = append(lines, fmt.Sprintf("Fresh at: %s", node.LastSuccessAt.Local().Format("2006-01-02 15:04")))
	}
	if node.LastFetchedAt != nil && node.LastSuccessAt != nil {
		if node.LastFetchedAt.After(*node.LastSuccessAt) {
			lines = append(lines, fmt.Sprintf("Last fetch: %s", node.LastFetchedAt.Local().Format("2006-01-02 15:04")))
		}
	}
	if node.LastError != "" {
		lines = append(lines, fmt.Sprintf("Error: %s", node.LastError))
	}

	if desc, ok := node.Details["description"].(string); ok && strings.TrimSpace(desc) != "" {
		lines = append(lines, "")
		lines = append(lines, titleStyle.Render("Summary:"))
		lines = append(lines, strings.TrimSpace(desc))
	}

	if breadcrumb := breadcrumbString(node.Details); breadcrumb != "" {
		lines = append(lines, "")
		lines = append(lines, fmt.Sprintf("Location: %s", breadcrumb))
	}

	if configLines := configSummaryLines(node.Details); len(configLines) > 0 {
		lines = append(lines, "")
		lines = append(lines, titleStyle.Render("Configuration:"))
		lines = append(lines, configLines...)
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
	if m.downloadInFlight {
		keys = "j/k move (live progress) h/d close q quit"
	}
	msg := strings.TrimSpace(m.statusText)
	modeText := titleStyle.Render(strings.ToUpper(m.mode))
	stateText := mutedStyle.Render(inFlight)
	selectedText := selected
	if selected != "none" {
		selectedText = selectedStyle.Render(selected)
	}
	keysText := mutedStyle.Render(keys)
	if msg != "" {
		return fmt.Sprintf("%s | %s | %s | %s | %s", modeText, stateText, selectedText, keysText, msg)
	}
	return fmt.Sprintf("%s | %s | %s | %s", modeText, stateText, selectedText, keysText)
}

func (m Model) renderDownloadPanel(node state.Node, maxWidth int, maxLines int) string {
	assets := m.selectedNodeAssets()
	header := []string{
		titleStyle.Render("Download"),
		fmt.Sprintf("Node: %s", displayTitle(node)),
		fmt.Sprintf("Target dir: %s", m.defaultDownloadDir),
		mutedStyle.Render("Keys: j/k move, space toggle, a all, c clear, enter download"),
		"",
	}
	for i := range header {
		header[i] = truncateOneLine(header[i], maxWidth)
	}
	if len(assets) == 0 {
		header = append(header, "(no assets available)")
		return strings.Join(header, "\n")
	}
	assetLines := make([]string, 0, len(assets))
	for i, asset := range assets {
		cursor := " "
		if i == m.downloadCursor {
			cursor = ">"
		}
		mark := colorDownloadMarker(m.downloadMarker(asset.URL))
		name := strings.TrimSpace(asset.Name)
		if name == "" {
			name = asset.URL
		}
		line := fmt.Sprintf("%s [%s] %s (%s)", cursor, mark, name, asset.Kind)
		assetLines = append(assetLines, truncateOneLine(line, maxWidth))
	}
	footer := fmt.Sprintf("Selected: %d/%d", len(m.selectedAssetURLs()), len(assets))
	if m.downloadInFlight || (m.downloadDone+m.downloadFailed) > 0 {
		footer = fmt.Sprintf("Progress: %d/%d done, %d failed", m.downloadDone, len(m.downloadQueue), m.downloadFailed)
	}
	footer = truncateOneLine(footer, maxWidth)

	if maxLines <= len(header)+1 {
		lines := append([]string{}, header...)
		lines = append(lines, footer)
		return strings.Join(clipLines(lines, maxLines), "\n")
	}

	listHeight := maxLines - len(header) - 1
	if listHeight < 1 {
		listHeight = 1
	}
	start := m.downloadOffset
	if start < 0 {
		start = 0
	}
	maxStart := len(assetLines) - listHeight
	if maxStart < 0 {
		maxStart = 0
	}
	if start > maxStart {
		start = maxStart
	}
	end := start + listHeight
	if end > len(assetLines) {
		end = len(assetLines)
	}

	lines := append([]string{}, header...)
	lines = append(lines, assetLines[start:end]...)
	lines = append(lines, footer)
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

func (m *Model) adjustDownloadOffset() {
	assets := m.selectedNodeAssets()
	if len(assets) == 0 {
		m.downloadOffset = 0
		return
	}
	listHeight := m.downloadListHeight()
	if listHeight < 1 {
		listHeight = 1
	}
	if m.downloadOffset < 0 {
		m.downloadOffset = 0
	}
	maxOffset := len(assets) - listHeight
	if maxOffset < 0 {
		maxOffset = 0
	}
	if m.downloadOffset > maxOffset {
		m.downloadOffset = maxOffset
	}
	if m.downloadCursor < m.downloadOffset {
		m.downloadOffset = m.downloadCursor
	}
	if m.downloadCursor >= m.downloadOffset+listHeight {
		m.downloadOffset = m.downloadCursor - listHeight + 1
	}
	if m.downloadOffset < 0 {
		m.downloadOffset = 0
	}
}

func (m Model) downloadNextCmd() tea.Cmd {
	if !m.downloadInFlight || m.downloadCurrent >= len(m.downloadQueue) {
		return nil
	}
	if m.downloadExecutor == nil {
		return nil
	}
	asset := m.downloadQueue[m.downloadCurrent]
	node := m.selectedNode()
	if node == nil {
		return nil
	}

	stSnapshot := m.st
	exec := m.downloadExecutor
	req := DownloadRequest{
		NodeID:    node.ID,
		TargetDir: m.downloadTargetDir,
		Assets:    []state.AssetRef{asset},
	}
	return func() tea.Msg {
		out := exec(stSnapshot, req)
		return downloadFileFinishedMsg{AssetURL: asset.URL, Outcome: out}
	}
}

func (m *Model) handleDownloadFileResult(assetURL string, out DownloadOutcome) {
	if out.Err != nil {
		m.downloadStatus[assetURL] = "error"
		m.downloadErrorByURL[assetURL] = out.Err.Error()
		m.downloadFailed++
	} else {
		m.downloadStatus[assetURL] = "done"
		m.downloadDone++
	}
	m.downloadCurrent++
	if m.downloadCurrent < len(m.downloadQueue) {
		nextURL := m.downloadQueue[m.downloadCurrent].URL
		m.downloadStatus[nextURL] = "active"
		m.focusDownloadAsset(nextURL)
	}
	m.statusText = fmt.Sprintf("downloading %d/%d to %s", m.downloadDone+m.downloadFailed, len(m.downloadQueue), m.downloadTargetDir)
	m.adjustDownloadOffset()
}

func (m Model) finalizeDownloadBatch() Model {
	m.downloadInFlight = false
	duration := time.Since(m.downloadStartedAt).Milliseconds()
	m.defaultDownloadDir = m.downloadTargetDir

	if m.persistChoices != nil {
		selected := m.selectedAssetURLs()
		if node := m.selectedNode(); node != nil {
			if err := m.persistChoices(node.ID, selected, m.downloadTargetDir); err == nil {
				m.recentAssetChoices[node.ID] = selected
			}
		}
	}

	if m.downloadFailed > 0 {
		m.statusText = fmt.Sprintf("download finished: %d ok, %d failed in %dms", m.downloadDone, m.downloadFailed, duration)
	} else {
		m.statusText = fmt.Sprintf("download finished: %d files in %dms", m.downloadDone, duration)
	}
	return m
}

func (m *Model) resetDownloadProgress() {
	m.downloadInFlight = false
	m.downloadQueue = []state.AssetRef{}
	m.downloadTargetDir = ""
	m.downloadCurrent = 0
	m.downloadDone = 0
	m.downloadFailed = 0
	m.downloadStatus = map[string]string{}
	m.downloadErrorByURL = map[string]string{}
	m.downloadStartedAt = time.Time{}
}

func (m Model) downloadMarker(assetURL string) string {
	if status, ok := m.downloadStatus[assetURL]; ok {
		switch status {
		case "active":
			return "…"
		case "done":
			return "✓"
		case "error":
			return "✗"
		case "pending":
			return "·"
		}
	}
	if m.downloadSelection[assetURL] {
		return "x"
	}
	return " "
}

func (m Model) downloadListHeight() int {
	// Download header is 5 lines, footer is 1 line.
	listHeight := m.panelContentHeight() - 6
	if listHeight < 1 {
		return 1
	}
	return listHeight
}

func (m Model) panelContentHeight() int {
	if m.width <= 0 || m.height <= 0 {
		return 1
	}
	availableHeight := maxInt(6, m.height)
	statusStyle := lipgloss.NewStyle().Border(lipgloss.RoundedBorder())
	_, statusFrameH := statusStyle.GetFrameSize()
	statusHeight := statusFrameH + 1 // one-line status content

	topHeight := availableHeight - statusHeight
	if topHeight < 3 {
		topHeight = 3
	}
	panelStyle := lipgloss.NewStyle().Border(lipgloss.RoundedBorder())
	_, panelFrameH := panelStyle.GetFrameSize()
	contentHeight := topHeight - panelFrameH
	if contentHeight < 1 {
		return 1
	}
	return contentHeight
}

func colorStatusTag(s state.Status) string {
	tag := fmt.Sprintf("[%s]", s)
	switch s {
	case state.StatusOK:
		return okStyle.Render(tag)
	case state.StatusStale:
		return staleStyle.Render(tag)
	case state.StatusError:
		return errorStyle.Render(tag)
	case state.StatusNever:
		return neverStyle.Render(tag)
	default:
		return mutedStyle.Render(tag)
	}
}

func colorResultTag(result string, freshness state.Status) string {
	tag := fmt.Sprintf("[%s]", result)
	switch normalizeStatsKey(result) {
	case "passed":
		return passStyle.Render(tag)
	case "failing":
		return failStyle.Render(tag)
	case "not_submitted":
		return noneStyle.Render(tag)
	case "ok":
		return okStyle.Render(tag)
	case "stale":
		return staleStyle.Render(tag)
	case "error":
		return errorStyle.Render(tag)
	case "never":
		return neverStyle.Render(tag)
	case "unknown":
		if freshness == state.StatusError {
			return errorStyle.Render(tag)
		}
		return mutedStyle.Render(tag)
	default:
		if isGradeLike(result) {
			return infoStyle.Render(tag)
		}
		return mutedStyle.Render(tag)
	}
}

func colorFreshnessTag(s state.Status) string {
	switch s {
	case state.StatusStale:
		return staleStyle.Render("(stale)")
	case state.StatusError:
		return errorStyle.Render("(error)")
	case state.StatusNever:
		return neverStyle.Render("(never)")
	default:
		return mutedStyle.Render("")
	}
}

func colorStatusWord(s state.Status) string {
	word := strings.ToUpper(string(s))
	switch s {
	case state.StatusOK:
		return okStyle.Render(word)
	case state.StatusStale:
		return staleStyle.Render(word)
	case state.StatusError:
		return errorStyle.Render(word)
	case state.StatusNever:
		return neverStyle.Render(word)
	default:
		return mutedStyle.Render(word)
	}
}

func colorResultWord(result string) string {
	word := strings.ToUpper(strings.ReplaceAll(result, "_", " "))
	if isGradeLike(result) {
		return infoStyle.Render(strings.TrimSpace(result))
	}
	switch normalizeStatsKey(result) {
	case "passed":
		return passStyle.Render(word)
	case "failing":
		return failStyle.Render(word)
	case "not_submitted":
		return noneStyle.Render(word)
	default:
		return infoStyle.Render(word)
	}
}

func colorDownloadMarker(mark string) string {
	switch mark {
	case "✓":
		return okStyle.Render(mark)
	case "✗":
		return errorStyle.Render(mark)
	case "…":
		return selectedStyle.Render(mark)
	case "·":
		return mutedStyle.Render(mark)
	default:
		return mark
	}
}

func (m *Model) focusDownloadAsset(assetURL string) {
	if assetURL == "" {
		return
	}
	assets := m.selectedNodeAssets()
	for i := range assets {
		if assets[i].URL == assetURL {
			m.downloadCursor = i
			return
		}
	}
}

func readableKind(kind string) string {
	switch strings.TrimSpace(strings.ToLower(kind)) {
	case "catalog":
		return "Catalog"
	case "year":
		return "Academic Year"
	case "course":
		return "Course"
	case "assignment":
		return "Assignment"
	default:
		if kind == "" {
			return "Unknown"
		}
		k := strings.TrimSpace(kind)
		if k == "" {
			return "Unknown"
		}
		return strings.ToUpper(k[:1]) + k[1:]
	}
}

func nodeResultLabel(node state.Node) string {
	if strings.TrimSpace(strings.ToLower(node.Kind)) != "assignment" {
		switch node.Status {
		case state.StatusOK:
			return "ok"
		case state.StatusStale:
			return "stale"
		case state.StatusError:
			return "error"
		case state.StatusNever:
			return "never"
		default:
			return "unknown"
		}
	}

	summary := assignmentStatsSummary(node.Details)
	status := normalizeStatsKey(stringAnyMapGetString(summary, "status"))
	statusText := normalizeStatsKey(stringAnyMapGetString(summary, "status_text"))
	combined := strings.TrimSpace(status + " " + statusText)

	if containsAny(combined, "passed", "pass") {
		return "passed"
	}
	if containsAny(combined, "failed", "failing", "wrong", "error", "timeout", "diff", "runtime") {
		return "failing"
	}
	if grade := strings.TrimSpace(stringAnyMapGetString(summary, "grade")); grade != "" {
		return grade
	}
	if statusPageURL(node.Details) != "" {
		return "not_submitted"
	}
	return "unknown"
}

func containsAny(haystack string, needles ...string) bool {
	for _, n := range needles {
		if strings.Contains(haystack, n) {
			return true
		}
	}
	return false
}

func isGradeLike(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}
	raw = strings.ReplaceAll(raw, ",", ".")
	if _, err := strconv.ParseFloat(raw, 64); err == nil {
		return true
	}
	return false
}

func breadcrumbString(details map[string]any) string {
	raw, ok := details["breadcrumb"]
	if !ok {
		return ""
	}
	switch v := raw.(type) {
	case []string:
		return strings.Join(v, " > ")
	case []any:
		parts := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				parts = append(parts, strings.TrimSpace(s))
			}
		}
		return strings.Join(parts, " > ")
	default:
		return ""
	}
}

func configSummaryLines(details map[string]any) []string {
	raw, ok := details["config"]
	if !ok {
		return nil
	}
	cfg, ok := raw.(map[string]any)
	if !ok {
		return nil
	}
	lines := make([]string, 0, 3)
	if v, ok := cfg["leading_submission"]; ok {
		lines = append(lines, fmt.Sprintf("- Leading submission: %v", v))
	}
	if v, ok := cfg["end_display"]; ok {
		lines = append(lines, fmt.Sprintf("- Due: %v", v))
	} else if v, ok := cfg["end_iso"]; ok {
		lines = append(lines, fmt.Sprintf("- Due: %v", v))
	}
	if v, ok := cfg["sort"]; ok {
		lines = append(lines, fmt.Sprintf("- Sorted: %v", v))
	}
	return lines
}

func statusPageURL(details map[string]any) string {
	raw, ok := details["links"]
	if !ok {
		return ""
	}
	switch links := raw.(type) {
	case map[string]string:
		return strings.TrimSpace(links["status_page"])
	case map[string]any:
		if v, ok := links["status_page"]; ok {
			if s, ok := v.(string); ok {
				return strings.TrimSpace(s)
			}
		}
	}
	return ""
}

func statsSummaryLines(details map[string]any) []string {
	raw, ok := details["stats"]
	if !ok {
		return nil
	}
	stats, ok := raw.(map[string]any)
	if !ok {
		return nil
	}

	lines := make([]string, 0, 8)
	if page := stringAnyMapGetString(stats, "status_page"); page != "" {
		lines = append(lines, fmt.Sprintf("- Page: %s", page))
	}

	if summary := assignmentStatsSummary(details); summary != nil {
		if status := stringAnyMapGetString(summary, "status"); status != "" {
			lines = append(lines, fmt.Sprintf("- Status: %s", status))
		} else if statusText := stringAnyMapGetString(summary, "status_text"); statusText != "" {
			lines = append(lines, fmt.Sprintf("- Status: %s", statusText))
		}
		if grade := stringAnyMapGetString(summary, "grade"); grade != "" {
			lines = append(lines, fmt.Sprintf("- Grade: %s", grade))
		}
		if group := stringAnyMapGetString(summary, "group"); group != "" {
			lines = append(lines, fmt.Sprintf("- Group: %s", group))
		}
	}

	if counts := anyMapToStringAnyMap(stats["counts"]); counts != nil {
		total := stringAnyMapGetInt(counts, "total")
		passed := stringAnyMapGetInt(counts, "passed")
		if total >= 0 || passed >= 0 {
			parts := make([]string, 0, 2)
			if total >= 0 {
				parts = append(parts, fmt.Sprintf("total=%d", total))
			}
			if passed >= 0 {
				parts = append(parts, fmt.Sprintf("passed=%d", passed))
			}
			lines = append(lines, fmt.Sprintf("- Counts: %s", strings.Join(parts, ", ")))
		}
	}

	if refs := stringAnyMapGetMap(stats, "submission_refs"); refs != nil {
		refKeys := []string{"leading", "best", "latest", "first_pass", "last_pass"}
		for _, key := range refKeys {
			ref, ok := pickSubmissionRef(refs, key)
			if !ok {
				continue
			}
			refMap, ok := ref.(map[string]any)
			if !ok {
				continue
			}
			title := strings.TrimSpace(stringAnyMapGetString(refMap, "title"))
			if title == "" {
				title = strings.TrimSpace(stringAnyMapGetString(refMap, "url"))
			}
			if title == "" {
				continue
			}
			label := strings.ReplaceAll(key, "_", " ")
			lines = append(lines, fmt.Sprintf("- %s: %s", capitalizeFirst(label), title))
		}
	}

	return lines
}

func assignmentStatsSummary(details map[string]any) map[string]any {
	raw, ok := details["stats"]
	if !ok {
		return nil
	}
	stats, ok := raw.(map[string]any)
	if !ok {
		return nil
	}
	return stringAnyMapGetMap(stats, "summary")
}

func stringAnyMapGetMap(m map[string]any, key string) map[string]any {
	if m == nil {
		return nil
	}
	v, ok := mapLookupNormalized(m, key)
	if !ok {
		return nil
	}
	out, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	return out
}

func anyMapToStringAnyMap(v any) map[string]any {
	switch vv := v.(type) {
	case map[string]any:
		return vv
	case map[string]int:
		out := make(map[string]any, len(vv))
		for k, val := range vv {
			out[k] = val
		}
		return out
	case map[string]string:
		out := make(map[string]any, len(vv))
		for k, val := range vv {
			out[k] = val
		}
		return out
	default:
		return nil
	}
}

func statsDebugLines(details map[string]any) []string {
	if !statsDebugEnabled() {
		return nil
	}
	raw, ok := details["stats"]
	if !ok {
		return []string{"- stats key missing"}
	}
	stats, ok := raw.(map[string]any)
	if !ok {
		return []string{fmt.Sprintf("- stats type: %T", raw)}
	}
	lines := []string{
		fmt.Sprintf("- has summary: %t", stringAnyMapGetMap(stats, "summary") != nil),
		fmt.Sprintf("- has counts: %t", anyMapToStringAnyMap(stats["counts"]) != nil),
		fmt.Sprintf("- has refs: %t", stringAnyMapGetMap(stats, "submission_refs") != nil),
	}
	if b, err := json.Marshal(stats); err == nil {
		lines = append(lines, "- raw: "+string(b))
	} else {
		lines = append(lines, fmt.Sprintf("- raw marshal error: %v", err))
	}
	return lines
}

func statsDebugEnabled() bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv("THEMIS_DEBUG_STATS")))
	return v == "1" || v == "true" || v == "yes" || v == "on"
}

func stringAnyMapGetString(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	v, ok := mapLookupNormalized(m, key)
	if !ok {
		return ""
	}
	switch vv := v.(type) {
	case string:
		return strings.TrimSpace(vv)
	default:
		return ""
	}
}

func stringAnyMapGetInt(m map[string]any, key string) int {
	if m == nil {
		return -1
	}
	v, ok := mapLookupNormalized(m, key)
	if !ok {
		return -1
	}
	switch vv := v.(type) {
	case int:
		return vv
	case int64:
		return int(vv)
	case float64:
		return int(vv)
	case float32:
		return int(vv)
	case string:
		i, err := strconv.Atoi(strings.TrimSpace(vv))
		if err == nil {
			return i
		}
	}
	return -1
}

func capitalizeFirst(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	runes := []rune(s)
	first := strings.ToUpper(string(runes[0]))
	if len(runes) == 1 {
		return first
	}
	return first + string(runes[1:])
}

func mapLookupNormalized(m map[string]any, key string) (any, bool) {
	if m == nil {
		return nil, false
	}
	if v, ok := m[key]; ok {
		return v, true
	}
	target := normalizeStatsKey(key)
	for k, v := range m {
		if normalizeStatsKey(k) == target {
			return v, true
		}
	}
	return nil, false
}

func normalizeStatsKey(raw string) string {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" {
		return ""
	}
	parts := strings.Fields(raw)
	if len(parts) == 0 {
		return ""
	}
	out := strings.Join(parts, "_")
	out = strings.ReplaceAll(out, "-", "_")
	out = strings.Trim(out, "_:")
	return out
}

func pickSubmissionRef(refs map[string]any, target string) (any, bool) {
	if refs == nil {
		return nil, false
	}
	if v, ok := mapLookupNormalized(refs, target); ok {
		return v, true
	}
	normTarget := normalizeStatsKey(target)
	aliases := map[string][]string{
		"leading":    {"counts_towards_grade", "submission_that_counts_towards_the_grade"},
		"best":       {"latest_submission_with_the_best_result"},
		"latest":     {"most_recent_submission"},
		"first_pass": {"first_submission_that_passed"},
		"last_pass":  {"last_submission_to_pass_before_the_deadline"},
	}
	for k, v := range refs {
		nk := normalizeStatsKey(k)
		if strings.Contains(nk, normTarget) {
			return v, true
		}
		for _, alias := range aliases[normTarget] {
			if strings.Contains(nk, alias) {
				return v, true
			}
		}
	}
	return nil, false
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

func clipLines(lines []string, maxLines int) []string {
	if maxLines <= 0 {
		return []string{}
	}
	if len(lines) <= maxLines {
		return lines
	}
	return lines[:maxLines]
}

func truncateOneLine(content string, maxWidth int) string {
	line := strings.Split(content, "\n")[0]
	if maxWidth <= 0 {
		return ""
	}
	if lipgloss.Width(line) <= maxWidth {
		return line
	}
	if maxWidth == 1 {
		return "…"
	}
	limit := maxWidth - 1
	var b strings.Builder
	currentWidth := 0
	for _, r := range line {
		rw := lipgloss.Width(string(r))
		if currentWidth+rw > limit {
			break
		}
		b.WriteRune(r)
		currentWidth += rw
	}
	return b.String() + "…"
}
