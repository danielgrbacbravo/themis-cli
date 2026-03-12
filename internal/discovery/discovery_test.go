package discovery

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDiscoverAssignments_Recursive(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/course/root":
			_, _ = w.Write([]byte(`
				<div class="subsec round shade ass-children">
					<ul class="round">
						<li><span class="ass-link"><a href="/course/a1">Assignment 1</a></span></li>
						<li><span class="ass-link"><a href="/course/a2">Assignment 2</a></span></li>
					</ul>
				</div>`))
		case "/course/a1":
			_, _ = w.Write([]byte(`
				<div class="subsec round shade ass-children">
					<ul class="round">
						<li><span class="ass-link"><a href="/course/a1/e1">Exercise 1</a></span></li>
					</ul>
				</div>`))
		default:
			_, _ = w.Write([]byte(`<div class="subsec round shade ass-children"><ul class="round"></ul></div>`))
		}
	}))
	defer server.Close()

	service := NewService(server.URL)
	rootURL, entries, err := service.DiscoverAssignments(server.Client(), "/course/root", 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if rootURL != server.URL+"/course/root" {
		t.Fatalf("unexpected normalized root URL: %s", rootURL)
	}

	if len(entries) != 3 {
		t.Fatalf("unexpected entry count: got=%d want=%d", len(entries), 3)
	}

	if entries[0].Name != "Assignment 1" || entries[0].Depth != 1 {
		t.Fatalf("unexpected first entry: %#v", entries[0])
	}
	if entries[1].Name != "Exercise 1" || entries[1].Depth != 2 {
		t.Fatalf("unexpected second entry: %#v", entries[1])
	}
	if entries[2].Name != "Assignment 2" || entries[2].Depth != 1 {
		t.Fatalf("unexpected third entry: %#v", entries[2])
	}
}

func TestDiscoverAssignments_DepthLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`
			<div class="subsec round shade ass-children">
				<ul class="round">
					<li><span class="ass-link"><a href="/course/a1">Assignment 1</a></span></li>
				</ul>
			</div>`))
	}))
	defer server.Close()

	service := NewService(server.URL)
	_, entries, err := service.DiscoverAssignments(server.Client(), "/course/root", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected 0 entries at depth 0, got %d", len(entries))
	}
}
