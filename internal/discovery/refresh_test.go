package discovery

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"testing"

	"themis-cli/internal/state"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func testClientFromMap(t *testing.T, pages map[string]string, hits map[string]int) *http.Client {
	t.Helper()
	return &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			url := req.URL.String()
			hits[url]++
			body, ok := pages[url]
			if !ok {
				return &http.Response{
					StatusCode: http.StatusNotFound,
					Body:       io.NopCloser(strings.NewReader("not found")),
					Header:     make(http.Header),
					Request:    req,
				}, nil
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewBufferString(body)),
				Header:     make(http.Header),
				Request:    req,
			}, nil
		}),
	}
}

func TestRefreshNode_DeepNodeDoesNotFetchRoot(t *testing.T) {
	base := "https://themis.housing.rug.nl"
	deep := base + "/course/2025-2026/os/lab3"
	child := base + "/course/2025-2026/os/lab3/task"
	root := base + "/course"

	pages := map[string]string{
		deep: `<html><body>
		<section class="assignment"><div class="sec-heading"><h3 class="sec-title">/ <a href="/course/">Courses</a> / <a href="/course/2025-2026">2025-2026</a> / <a href="/course/2025-2026/os/lab3">Lab 3</a></h3></div></section>
		<div class="subsec round shade ass-children"><ul class="round">
		<li><span class="ass-link"><a href="/course/2025-2026/os/lab3/task" data-navhref="/api/navigation/2025-2026/os/lab3/task">Task</a></span></li>
		</ul></div>
		</body></html>`,
		child: `<html><body>
		<section class="assignment"><div class="sec-heading"><h3 class="sec-title">/ <a href="/course/">Courses</a> / <a href="/course/2025-2026">2025-2026</a> / <a href="/course/2025-2026/os/lab3/task">Task</a></h3></div></section>
		<div class="subsec round shade ass-children"><ul class="round"></ul></div>
		</body></html>`,
		root: `<html><body><div class="subsec round shade ass-children"><ul class="round"></ul></div></body></html>`,
	}
	hits := map[string]int{}
	client := testClientFromMap(t, pages, hits)
	service := NewService(base)
	st := state.NewEmptyState()

	result, err := service.RefreshNode(client, &st, deep, 1)
	if err != nil {
		t.Fatalf("refresh failed: %v", err)
	}
	if result.FetchedNodes != 2 {
		t.Fatalf("unexpected fetched nodes: %d", result.FetchedNodes)
	}
	if hits[root] != 0 {
		t.Fatalf("expected root not to be fetched, hits=%d", hits[root])
	}
	if len(result.Errors) != 0 {
		t.Fatalf("unexpected errors: %#v", result.Errors)
	}
}

func TestRefreshNode_ReplacesChildrenAndTracksRemovedEdges(t *testing.T) {
	base := "https://themis.housing.rug.nl"
	course := base + "/course/2025-2026/os"
	oldChild := base + "/course/2025-2026/os/lab1"
	newChild := base + "/course/2025-2026/os/lab2"

	service := NewService(base)
	st := state.NewEmptyState()

	pages1 := map[string]string{
		course: `<html><body>
		<section class="assignment"><div class="sec-heading"><h3 class="sec-title">/ <a href="/course/">Courses</a> / <a href="/course/2025-2026">2025-2026</a> / <a href="/course/2025-2026/os">Operating Systems</a></h3></div></section>
		<div class="subsec round shade ass-children"><ul class="round"><li><span class="ass-link"><a href="/course/2025-2026/os/lab1" data-navhref="/api/navigation/2025-2026/os/lab1">Lab 1</a></span></li></ul></div>
		</body></html>`,
		oldChild: `<html><body><section class="assignment"><div class="sec-heading"><h3 class="sec-title">/ <a href="/course/2025-2026/os/lab1">Lab 1</a></h3></div></section><div class="subsec round shade ass-children"><ul class="round"></ul></div></body></html>`,
	}
	hits1 := map[string]int{}
	_, err := service.RefreshNode(testClientFromMap(t, pages1, hits1), &st, course, 1)
	if err != nil {
		t.Fatalf("first refresh failed: %v", err)
	}

	pages2 := map[string]string{
		course: `<html><body>
		<section class="assignment"><div class="sec-heading"><h3 class="sec-title">/ <a href="/course/">Courses</a> / <a href="/course/2025-2026">2025-2026</a> / <a href="/course/2025-2026/os">Operating Systems</a></h3></div></section>
		<div class="subsec round shade ass-children"><ul class="round"><li><span class="ass-link"><a href="/course/2025-2026/os/lab2" data-navhref="/api/navigation/2025-2026/os/lab2">Lab 2</a></span></li></ul></div>
		</body></html>`,
		newChild: `<html><body><section class="assignment"><div class="sec-heading"><h3 class="sec-title">/ <a href="/course/2025-2026/os/lab2">Lab 2</a></h3></div></section><div class="subsec round shade ass-children"><ul class="round"></ul></div></body></html>`,
	}
	hits2 := map[string]int{}
	result, err := service.RefreshNode(testClientFromMap(t, pages2, hits2), &st, course, 1)
	if err != nil {
		t.Fatalf("second refresh failed: %v", err)
	}
	if result.RemovedEdges != 1 {
		t.Fatalf("expected removed edge count 1, got %d", result.RemovedEdges)
	}

	courseID, _, _ := state.NodeIDFromURL(course)
	oldID, _, _ := state.NodeIDFromURL(oldChild)
	newID, _, _ := state.NodeIDFromURL(newChild)

	if strings.Join(st.Nodes[courseID].ChildIDs, ",") != newID {
		t.Fatalf("course children not replaced: %#v", st.Nodes[courseID].ChildIDs)
	}
	if contains(st.Nodes[oldID].ParentIDs, courseID) {
		t.Fatalf("old child still has parent edge")
	}
	if !contains(st.Nodes[newID].ParentIDs, courseID) {
		t.Fatalf("new child missing parent edge")
	}
}

func TestRefreshNode_ExtractsMetadata(t *testing.T) {
	base := "https://themis.housing.rug.nl"
	course := base + "/course/2025-2026/os"
	pages := map[string]string{
		course: `<html><body>
		<section class="assignment"><div class="sec-heading"><h3 class="sec-title">/ <a href="/course/">Courses</a> / <a href="/course/2025-2026">2025-2026</a> / <a href="/course/2025-2026/os">Operating Systems</a></h3></div></section>
		<div class="subsec round help shade"><a href="/stats/2025-2026/os" class="iconize status">Status</a></div>
		<div class="subsec round shade ass-children"><ul class="round"></ul></div>
		<div class="cfg-container round ass-config">
		<div class="cfg-line"><span class="cfg-key">Leading Submission:</span><span class="cfg-val">latest</span></div>
		<div class="cfg-line"><span class="cfg-key">End:</span><span class="cfg-val"><span class="tip-text">2026-08-31T21:59:59.000Z</span>Mon Aug 31 2026 23:59:59 GMT+0200</span></div>
		</div>
		<p class="ass-description">Course summary</p>
		</body></html>`,
	}

	service := NewService(base)
	hits := map[string]int{}
	client := testClientFromMap(t, pages, hits)
	st := state.NewEmptyState()

	_, err := service.RefreshNode(client, &st, course, 0)
	if err != nil {
		t.Fatalf("refresh failed: %v", err)
	}

	nodeID, _, _ := state.NodeIDFromURL(course)
	node := st.Nodes[nodeID]
	if node.NavAPIURL == "" {
		t.Fatalf("expected nav_api_url")
	}
	if node.Details == nil {
		t.Fatalf("expected details")
	}
	configAny, ok := node.Details["config"]
	if !ok {
		t.Fatalf("expected config details")
	}
	config, ok := configAny.(map[string]any)
	if !ok {
		t.Fatalf("invalid config type: %T", configAny)
	}
	if config["leading_submission"] != "latest" {
		t.Fatalf("unexpected leading_submission: %#v", config["leading_submission"])
	}
	linksAny, ok := node.Details["links"]
	if !ok {
		t.Fatalf("expected links details")
	}
	links := linksAny.(map[string]string)
	if links["status_page"] != "https://themis.housing.rug.nl/stats/2025-2026/os" {
		t.Fatalf("unexpected status_page: %#v", links["status_page"])
	}
}

func contains(list []string, target string) bool {
	for _, v := range list {
		if v == target {
			return true
		}
	}
	return false
}
