package state

import (
	"reflect"
	"testing"
	"time"
)

func TestDiffChildren(t *testing.T) {
	diff := DiffChildren([]string{"c1", "c2", "c2"}, []string{"c2", "c3", "", "c3"})

	if !stringSlicesEqual(diff.Added, []string{"c3"}) {
		t.Fatalf("added mismatch: %#v", diff.Added)
	}
	if !stringSlicesEqual(diff.Removed, []string{"c1"}) {
		t.Fatalf("removed mismatch: %#v", diff.Removed)
	}
	if !stringSlicesEqual(diff.Unchanged, []string{"c2"}) {
		t.Fatalf("unchanged mismatch: %#v", diff.Unchanged)
	}
}

func TestUpsertNode_CreateAndNoOpAndUpdate(t *testing.T) {
	now := time.Date(2026, 3, 17, 10, 0, 0, 0, time.UTC)
	st := NewEmptyState()

	node := Node{
		ID:           "url:root",
		Kind:         "course",
		Title:        "Operating Systems",
		CanonicalURL: "https://themis.housing.rug.nl/course/2025-2026/os",
		ParentIDs:    []string{},
		ChildIDs:     []string{"url:lab1"},
		Status:       StatusOK,
	}

	created, changed, err := UpsertNode(&st, node, now)
	if err != nil {
		t.Fatalf("create upsert failed: %v", err)
	}
	if !created || !changed {
		t.Fatalf("expected created+changed true, got created=%v changed=%v", created, changed)
	}

	created, changed, err = UpsertNode(&st, node, now.Add(1*time.Minute))
	if err != nil {
		t.Fatalf("noop upsert failed: %v", err)
	}
	if created || changed {
		t.Fatalf("expected no-op, got created=%v changed=%v", created, changed)
	}

	updated := node
	updated.Title = "Operating Systems (Updated)"
	created, changed, err = UpsertNode(&st, updated, now.Add(2*time.Minute))
	if err != nil {
		t.Fatalf("update upsert failed: %v", err)
	}
	if created || !changed {
		t.Fatalf("expected update change, got created=%v changed=%v", created, changed)
	}
	if st.Nodes[node.ID].CreatedAt.IsZero() {
		t.Fatalf("expected created_at set")
	}
	if st.Nodes[node.ID].Title != updated.Title {
		t.Fatalf("title not updated")
	}
}

func TestSetChildren_MutatesOnlyTouchedNodesAndEdges(t *testing.T) {
	t0 := time.Date(2026, 3, 17, 10, 0, 0, 0, time.UTC)
	t1 := t0.Add(1 * time.Minute)
	st := NewEmptyState()
	st.Nodes = map[string]Node{
		"url:parent": {
			ID:               "url:parent",
			Title:            "Parent",
			Kind:             "course",
			CanonicalURL:     "https://themis.housing.rug.nl/course/root",
			ChildIDs:         []string{"url:old", "url:keep"},
			ChildrenHydrated: true,
			Status:           StatusOK,
			CreatedAt:        t0,
			UpdatedAt:        t0,
		},
		"url:old": {
			ID:           "url:old",
			Title:        "Old",
			Kind:         "assignment",
			CanonicalURL: "https://themis.housing.rug.nl/course/root/old",
			ParentIDs:    []string{"url:parent"},
			Status:       StatusOK,
			CreatedAt:    t0,
			UpdatedAt:    t0,
		},
		"url:keep": {
			ID:           "url:keep",
			Title:        "Keep",
			Kind:         "assignment",
			CanonicalURL: "https://themis.housing.rug.nl/course/root/keep",
			ParentIDs:    []string{"url:parent"},
			Status:       StatusOK,
			CreatedAt:    t0,
			UpdatedAt:    t0,
		},
		"url:new": {
			ID:           "url:new",
			Title:        "New",
			Kind:         "assignment",
			CanonicalURL: "https://themis.housing.rug.nl/course/root/new",
			ParentIDs:    []string{},
			Status:       StatusOK,
			CreatedAt:    t0,
			UpdatedAt:    t0,
		},
		"url:untouched": {
			ID:           "url:untouched",
			Title:        "Untouched",
			Kind:         "assignment",
			CanonicalURL: "https://themis.housing.rug.nl/course/root/untouched",
			ParentIDs:    []string{},
			Status:       StatusOK,
			CreatedAt:    t0,
			UpdatedAt:    t0,
		},
	}

	beforeUntouched := st.Nodes["url:untouched"]

	diff, err := SetChildren(&st, "url:parent", []string{"url:keep", "url:new"}, t1)
	if err != nil {
		t.Fatalf("set children failed: %v", err)
	}

	if !stringSlicesEqual(diff.Added, []string{"url:new"}) {
		t.Fatalf("added mismatch: %#v", diff.Added)
	}
	if !stringSlicesEqual(diff.Removed, []string{"url:old"}) {
		t.Fatalf("removed mismatch: %#v", diff.Removed)
	}

	if !stringSlicesEqual(st.Nodes["url:parent"].ChildIDs, []string{"url:keep", "url:new"}) {
		t.Fatalf("parent child_ids mismatch: %#v", st.Nodes["url:parent"].ChildIDs)
	}
	if containsString(st.Nodes["url:old"].ParentIDs, "url:parent") {
		t.Fatalf("old child should no longer reference parent")
	}
	if !containsString(st.Nodes["url:new"].ParentIDs, "url:parent") {
		t.Fatalf("new child should reference parent")
	}
	if !reflect.DeepEqual(st.Nodes["url:untouched"], beforeUntouched) {
		t.Fatalf("untouched node changed")
	}

	if err := CheckEdgeConsistency(st); err != nil {
		t.Fatalf("expected consistent edges, got: %v", err)
	}
}

func TestSetChildren_RejectsMissingChild(t *testing.T) {
	st := NewEmptyState()
	st.Nodes = map[string]Node{
		"url:parent": {ID: "url:parent", CanonicalURL: "https://themis.housing.rug.nl/course/root", Status: StatusOK},
	}

	if _, err := SetChildren(&st, "url:parent", []string{"url:missing"}, time.Now().UTC()); err == nil {
		t.Fatalf("expected missing child error")
	}
}

func TestCheckEdgeConsistency_DetectsMismatch(t *testing.T) {
	st := NewEmptyState()
	st.Nodes = map[string]Node{
		"url:parent": {
			ID:           "url:parent",
			CanonicalURL: "https://themis.housing.rug.nl/course/root",
			ChildIDs:     []string{"url:child"},
		},
		"url:child": {
			ID:           "url:child",
			CanonicalURL: "https://themis.housing.rug.nl/course/root/child",
			ParentIDs:    []string{},
		},
	}

	if err := CheckEdgeConsistency(st); err == nil {
		t.Fatalf("expected edge mismatch error")
	}
}
