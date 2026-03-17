package discovery

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"

	"themis-cli/internal/state"
)

type RefreshResult struct {
	TargetURL    string   `json:"target_url"`
	Depth        int      `json:"depth"`
	FetchedNodes int      `json:"fetched_nodes"`
	UpdatedNodes int      `json:"updated_nodes"`
	RemovedEdges int      `json:"removed_edges"`
	Errors       []string `json:"errors"`
}

type pageChild struct {
	Title     string
	URL       string
	NavAPIURL string
}

type pageSnapshot struct {
	Title       string
	Kind        string
	Canonical   string
	NavAPIURL   string
	Children    []pageChild
	Details     map[string]any
	ContentHash string
}

func (s *Service) RefreshCatalog(client *http.Client, st *state.State, depth int) (RefreshResult, error) {
	if st == nil {
		return RefreshResult{}, fmt.Errorf("state is nil")
	}
	target := strings.TrimSpace(st.CatalogRootURL)
	if target == "" {
		target = strings.TrimRight(s.BaseURL, "/") + "/course"
	}
	return s.RefreshNode(client, st, target, depth)
}

func (s *Service) RefreshNode(client *http.Client, st *state.State, targetURL string, depth int) (RefreshResult, error) {
	if st == nil {
		return RefreshResult{}, fmt.Errorf("state is nil")
	}
	if client == nil {
		return RefreshResult{}, fmt.Errorf("http client is nil")
	}
	if depth < 0 {
		return RefreshResult{}, fmt.Errorf("depth must be >= 0")
	}

	normalizedTarget, err := s.normalizeURL(targetURL)
	if err != nil {
		return RefreshResult{}, err
	}
	canonicalTarget, err := state.CanonicalizeURL(normalizedTarget)
	if err != nil {
		return RefreshResult{}, fmt.Errorf("canonicalize target URL: %w", err)
	}

	now := time.Now().UTC()
	result := RefreshResult{
		TargetURL: canonicalTarget,
		Depth:     depth,
		Errors:    []string{},
	}
	updatedNodeIDs := map[string]struct{}{}
	visited := map[string]struct{}{}

	var walk func(canonicalURL string, remainingDepth int, parentID string)
	walk = func(canonicalURL string, remainingDepth int, parentID string) {
		if _, ok := visited[canonicalURL]; ok {
			if parentID != "" {
				childID := state.NodeIDFromCanonicalURL(canonicalURL)
				if edgeAdded(st, parentID, childID, now) {
					updatedNodeIDs[parentID] = struct{}{}
					updatedNodeIDs[childID] = struct{}{}
				}
			}
			return
		}
		visited[canonicalURL] = struct{}{}

		snap, fetchErr := s.fetchPageSnapshot(client, canonicalURL)
		if fetchErr != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", canonicalURL, fetchErr))
			errNodeID := markNodeFetchError(st, canonicalURL, fetchErr.Error(), now)
			updatedNodeIDs[errNodeID] = struct{}{}
			return
		}
		result.FetchedNodes++

		nodeID := state.NodeIDFromCanonicalURL(snap.Canonical)
		current, exists := st.Nodes[nodeID]

		patch := state.Node{
			ID:               nodeID,
			Kind:             snap.Kind,
			Title:            snap.Title,
			CanonicalURL:     snap.Canonical,
			NavAPIURL:        snap.NavAPIURL,
			ParentIDs:        append([]string{}, current.ParentIDs...),
			ChildIDs:         append([]string{}, current.ChildIDs...),
			ChildrenHydrated: exists && current.ChildrenHydrated,
			DepthHint:        computeDepthHint(snap.Canonical),
			Status:           state.StatusOK,
			LastFetchedAt:    ptrTime(now),
			LastSuccessAt:    ptrTime(now),
			LastError:        "",
			ContentHash:      "sha256:" + snap.ContentHash,
			Details:          snap.Details,
			Assets:           existingAssetsOrEmpty(current.Assets),
			CreatedAt:        current.CreatedAt,
			UpdatedAt:        current.UpdatedAt,
		}

		_, changed, upsertErr := state.UpsertNode(st, patch, now)
		if upsertErr != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", canonicalURL, upsertErr))
			return
		}
		if changed {
			updatedNodeIDs[nodeID] = struct{}{}
		}

		if parentID != "" {
			if edgeAdded(st, parentID, nodeID, now) {
				updatedNodeIDs[parentID] = struct{}{}
				updatedNodeIDs[nodeID] = struct{}{}
			}
		}

		children := make([]string, 0, len(snap.Children))
		for _, child := range snap.Children {
			childCanonical, cErr := state.CanonicalizeURL(child.URL)
			if cErr != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("%s child URL %q: %v", canonicalURL, child.URL, cErr))
				continue
			}
			childID := state.NodeIDFromCanonicalURL(childCanonical)
			children = append(children, childID)

			childNode, ok := st.Nodes[childID]
			if !ok {
				childNode = state.Node{
					ID:           childID,
					CanonicalURL: childCanonical,
					Status:       state.StatusNever,
					Assets:       []state.AssetRef{},
					CreatedAt:    now,
					UpdatedAt:    now,
				}
			}
			if strings.TrimSpace(child.Title) != "" {
				childNode.Title = child.Title
			}
			if strings.TrimSpace(child.NavAPIURL) != "" {
				childNode.NavAPIURL = child.NavAPIURL
			}
			if childNode.Kind == "" {
				childNode.Kind = inferKindFromURL(childCanonical)
			}
			if childNode.DepthHint == 0 {
				childNode.DepthHint = computeDepthHint(childCanonical)
			}
			if !containsString(childNode.ParentIDs, nodeID) {
				childNode.ParentIDs = append(childNode.ParentIDs, nodeID)
			}
			_, cChanged, cErr := state.UpsertNode(st, childNode, now)
			if cErr != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("upsert child %s: %v", childCanonical, cErr))
				continue
			}
			if cChanged {
				updatedNodeIDs[childID] = struct{}{}
			}
		}

		diff, setErr := state.SetChildren(st, nodeID, children, now)
		if setErr != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("set children for %s: %v", canonicalURL, setErr))
			return
		}
		if len(diff.Added) > 0 || len(diff.Removed) > 0 {
			updatedNodeIDs[nodeID] = struct{}{}
			for _, id := range diff.Added {
				updatedNodeIDs[id] = struct{}{}
			}
			for _, id := range diff.Removed {
				updatedNodeIDs[id] = struct{}{}
			}
		}
		result.RemovedEdges += len(diff.Removed)

		if remainingDepth == 0 {
			return
		}
		for _, child := range snap.Children {
			childCanonical, cErr := state.CanonicalizeURL(child.URL)
			if cErr != nil {
				continue
			}
			walk(childCanonical, remainingDepth-1, nodeID)
		}
	}

	walk(canonicalTarget, depth, "")
	result.UpdatedNodes = len(updatedNodeIDs)

	if st.BaseURL == "" {
		if base, err := state.CanonicalizeURL(strings.TrimRight(s.BaseURL, "/")); err == nil {
			st.BaseURL = strings.TrimRight(base, "/")
		}
	}
	if st.CatalogRootURL == "" {
		st.CatalogRootURL = strings.TrimRight(s.BaseURL, "/") + "/course"
	}

	return result, nil
}

func (s *Service) fetchPageSnapshot(client *http.Client, pageURL string) (pageSnapshot, error) {
	resp, err := client.Get(pageURL)
	if err != nil {
		return pageSnapshot{}, fmt.Errorf("fetch page: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusBadRequest {
		return pageSnapshot{}, fmt.Errorf("fetch page status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return pageSnapshot{}, fmt.Errorf("read page: %w", err)
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
	if err != nil {
		return pageSnapshot{}, fmt.Errorf("parse page: %w", err)
	}

	canonical, err := state.CanonicalizeURL(pageURL)
	if err != nil {
		return pageSnapshot{}, err
	}
	children, err := s.extractChildren(doc, canonical)
	if err != nil {
		return pageSnapshot{}, err
	}

	h := sha256.Sum256(body)

	return pageSnapshot{
		Title:       extractTitle(doc),
		Kind:        inferKindFromURL(canonical),
		Canonical:   canonical,
		NavAPIURL:   s.extractCurrentNavAPIURL(doc, canonical),
		Children:    children,
		Details:     extractDetails(doc, canonical),
		ContentHash: hex.EncodeToString(h[:]),
	}, nil
}

func (s *Service) extractChildren(doc *goquery.Document, currentURL string) ([]pageChild, error) {
	children := make([]pageChild, 0)
	doc.Find("div.subsec.round.shade.ass-children ul.round li").Each(func(_ int, sel *goquery.Selection) {
		anchor := sel.Find("span.ass-link a")
		title := strings.TrimSpace(anchor.Text())
		href, ok := anchor.Attr("href")
		if !ok {
			return
		}
		absolute, err := s.resolveURL(currentURL, href)
		if err != nil {
			return
		}
		navAPI := ""
		if navPath, ok := anchor.Attr("data-navhref"); ok {
			navAPI, _ = s.resolveURL(currentURL, navPath)
		}
		children = append(children, pageChild{Title: title, URL: absolute, NavAPIURL: navAPI})
	})
	return children, nil
}

func (s *Service) extractCurrentNavAPIURL(doc *goquery.Document, canonicalURL string) string {
	parsed, err := url.Parse(canonicalURL)
	if err != nil {
		return ""
	}
	rel := parsed.Path
	if rel == "" {
		rel = "/"
	}
	selector := fmt.Sprintf("#nav a[href='%s']", rel)
	if anchor := doc.Find(selector).First(); anchor.Length() > 0 {
		if nav, ok := anchor.Attr("data-navhref"); ok {
			resolved, err := s.resolveURL(canonicalURL, nav)
			if err == nil {
				return resolved
			}
		}
	}

	if strings.HasPrefix(rel, "/course") {
		suffix := strings.TrimPrefix(rel, "/course")
		if suffix == "" || suffix == "/" {
			return strings.TrimRight(s.BaseURL, "/") + "/api/navigation/"
		}
		return strings.TrimRight(s.BaseURL, "/") + "/api/navigation" + suffix
	}
	return ""
}

func extractTitle(doc *goquery.Document) string {
	if title := strings.TrimSpace(doc.Find(".page-body section.assignment .sec-heading .sec-title a").Last().Text()); title != "" {
		return title
	}
	if titleTag := strings.TrimSpace(doc.Find("head title").First().Text()); titleTag != "" {
		const prefix = "Assignment:"
		const suffix = "- Themis"
		clean := strings.TrimSpace(strings.TrimPrefix(titleTag, prefix))
		clean = strings.TrimSpace(strings.TrimSuffix(clean, suffix))
		if clean != "" {
			return clean
		}
	}
	return "Untitled"
}

func extractDetails(doc *goquery.Document, canonicalURL string) map[string]any {
	out := map[string]any{}

	breadcrumb := make([]string, 0)
	doc.Find(".page-body section.assignment .sec-heading .sec-title a").Each(func(_ int, sel *goquery.Selection) {
		text := strings.TrimSpace(sel.Text())
		if text != "" {
			breadcrumb = append(breadcrumb, text)
		}
	})
	if len(breadcrumb) > 0 {
		out["breadcrumb"] = breadcrumb
	}

	config := map[string]any{}
	doc.Find(".ass-config .cfg-line").Each(func(_ int, sel *goquery.Selection) {
		key := normalizeConfigKey(sel.Find(".cfg-key").First().Text())
		if key == "" {
			return
		}
		valSel := sel.Find(".cfg-val").First()
		val := strings.TrimSpace(valSel.Text())
		if key == "end" {
			if iso := strings.TrimSpace(valSel.Find(".tip-text").First().Text()); iso != "" {
				config["end_iso"] = iso
			}
			if val != "" {
				config["end_display"] = val
			}
			return
		}
		if val != "" {
			config[key] = val
		}
	})
	if len(config) > 0 {
		out["config"] = config
	}

	links := map[string]string{}
	if statusHref, ok := doc.Find("a.iconize.status").First().Attr("href"); ok {
		if abs, err := resolveLinkFromCanonical(canonicalURL, statusHref); err == nil {
			links["status_page"] = abs
		}
	}
	if len(links) > 0 {
		out["links"] = links
	}

	if desc := strings.TrimSpace(doc.Find("p.ass-description").First().Text()); desc != "" {
		out["description"] = desc
	}

	return out
}

func resolveLinkFromCanonical(canonicalBase string, href string) (string, error) {
	base, err := url.Parse(canonicalBase)
	if err != nil {
		return "", err
	}
	rel, err := url.Parse(strings.TrimSpace(href))
	if err != nil {
		return "", err
	}
	return base.ResolveReference(rel).String(), nil
}

func normalizeConfigKey(raw string) string {
	raw = strings.TrimSpace(strings.TrimSuffix(raw, ":"))
	raw = strings.ToLower(raw)
	raw = strings.ReplaceAll(raw, " ", "_")
	if raw == "leading_submission" {
		return raw
	}
	return raw
}

func inferKindFromURL(canonicalURL string) string {
	parsed, err := url.Parse(canonicalURL)
	if err != nil {
		return "assignment"
	}
	p := strings.Trim(path.Clean(parsed.Path), "/")
	if p == "" || p == "course" {
		return "catalog"
	}
	if !strings.HasPrefix(p, "course/") {
		return "assignment"
	}
	tail := strings.TrimPrefix(p, "course/")
	parts := strings.Split(tail, "/")
	switch len(parts) {
	case 1:
		return "year"
	case 2:
		return "course"
	default:
		return "assignment"
	}
}

func computeDepthHint(canonicalURL string) int {
	parsed, err := url.Parse(canonicalURL)
	if err != nil {
		return 0
	}
	p := strings.Trim(path.Clean(parsed.Path), "/")
	if p == "" || p == "course" {
		return 0
	}
	if strings.HasPrefix(p, "course/") {
		tail := strings.TrimPrefix(p, "course/")
		if tail == "" {
			return 0
		}
		return len(strings.Split(tail, "/"))
	}
	return len(strings.Split(p, "/"))
}

func existingAssetsOrEmpty(in []state.AssetRef) []state.AssetRef {
	if in == nil {
		return []state.AssetRef{}
	}
	return in
}

func ptrTime(t time.Time) *time.Time {
	tt := t.UTC()
	return &tt
}

func markNodeFetchError(st *state.State, canonicalURL string, msg string, now time.Time) string {
	nodeID := state.NodeIDFromCanonicalURL(canonicalURL)
	node, ok := st.Nodes[nodeID]
	if !ok {
		node = state.Node{
			ID:           nodeID,
			CanonicalURL: canonicalURL,
			Title:        canonicalURL,
			Kind:         inferKindFromURL(canonicalURL),
			Assets:       []state.AssetRef{},
			CreatedAt:    now,
		}
	}
	node.Status = state.StatusError
	node.LastError = msg
	node.LastFetchedAt = ptrTime(now)
	node.UpdatedAt = now
	if node.ParentIDs == nil {
		node.ParentIDs = []string{}
	}
	if node.ChildIDs == nil {
		node.ChildIDs = []string{}
	}
	_, _, _ = state.UpsertNode(st, node, now)
	return nodeID
}

func edgeAdded(st *state.State, parentID string, childID string, now time.Time) bool {
	parent, ok := st.Nodes[parentID]
	if !ok {
		return false
	}
	if containsString(parent.ChildIDs, childID) {
		return false
	}
	diff, err := state.SetChildren(st, parentID, append(parent.ChildIDs, childID), now)
	if err != nil {
		return false
	}
	return len(diff.Added) > 0
}

func containsString(list []string, target string) bool {
	for _, v := range list {
		if v == target {
			return true
		}
	}
	return false
}
