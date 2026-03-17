package state

import (
	"testing"
	"time"
)

func TestApplyFetchSuccess(t *testing.T) {
	now := time.Date(2026, 3, 17, 12, 0, 0, 0, time.UTC)
	n := Node{Status: StatusError, LastError: "timeout"}
	if err := ApplyFetchSuccess(&n, now); err != nil {
		t.Fatalf("apply success failed: %v", err)
	}
	if n.Status != StatusOK {
		t.Fatalf("expected status ok, got %s", n.Status)
	}
	if n.LastFetchedAt == nil || !n.LastFetchedAt.Equal(now) {
		t.Fatalf("unexpected last_fetched_at: %#v", n.LastFetchedAt)
	}
	if n.LastSuccessAt == nil || !n.LastSuccessAt.Equal(now) {
		t.Fatalf("unexpected last_success_at: %#v", n.LastSuccessAt)
	}
	if n.LastError != "" {
		t.Fatalf("expected cleared error, got %q", n.LastError)
	}
}

func TestApplyFetchFailure_PreservesReadableData(t *testing.T) {
	now := time.Date(2026, 3, 17, 12, 0, 0, 0, time.UTC)
	oldSuccess := now.Add(-24 * time.Hour)
	n := Node{
		Status:        StatusOK,
		Title:         "Operating Systems",
		CanonicalURL:  "https://themis.housing.rug.nl/course/2025-2026/os",
		Details:       map[string]any{"description": "cached"},
		Assets:        []AssetRef{{Name: "1.in", URL: "https://themis.housing.rug.nl/file/course/%40tests/1.in"}},
		LastSuccessAt: &oldSuccess,
	}
	if err := ApplyFetchFailure(&n, now, "timeout while fetching node"); err != nil {
		t.Fatalf("apply failure failed: %v", err)
	}
	if n.Status != StatusError {
		t.Fatalf("expected status error, got %s", n.Status)
	}
	if n.LastFetchedAt == nil || !n.LastFetchedAt.Equal(now) {
		t.Fatalf("unexpected last_fetched_at: %#v", n.LastFetchedAt)
	}
	if n.LastSuccessAt == nil || !n.LastSuccessAt.Equal(oldSuccess) {
		t.Fatalf("last_success_at should be preserved")
	}
	if n.Title != "Operating Systems" || n.Details["description"] != "cached" || len(n.Assets) != 1 {
		t.Fatalf("expected cached content to remain readable")
	}
}

func TestApplyStalePolicy_TransitionsOnlyOK(t *testing.T) {
	now := time.Date(2026, 3, 17, 12, 0, 0, 0, time.UTC)
	old := now.Add(-3 * time.Hour)
	recent := now.Add(-15 * time.Minute)

	okNode := Node{Status: StatusOK, LastSuccessAt: &old}
	changed, err := ApplyStalePolicy(&okNode, now, time.Hour)
	if err != nil {
		t.Fatalf("stale policy failed: %v", err)
	}
	if !changed || okNode.Status != StatusStale {
		t.Fatalf("expected ok->stale transition")
	}

	errorNode := Node{Status: StatusError, LastSuccessAt: &old}
	changed, err = ApplyStalePolicy(&errorNode, now, time.Hour)
	if err != nil {
		t.Fatalf("stale policy failed: %v", err)
	}
	if changed || errorNode.Status != StatusError {
		t.Fatalf("error node should remain unchanged")
	}

	freshNode := Node{Status: StatusOK, LastSuccessAt: &recent}
	changed, err = ApplyStalePolicy(&freshNode, now, time.Hour)
	if err != nil {
		t.Fatalf("stale policy failed: %v", err)
	}
	if changed || freshNode.Status != StatusOK {
		t.Fatalf("fresh node should remain ok")
	}
}

func TestApplyStateStalePolicy(t *testing.T) {
	now := time.Date(2026, 3, 17, 12, 0, 0, 0, time.UTC)
	old := now.Add(-2 * time.Hour)

	st := NewEmptyState()
	st.Nodes = map[string]Node{
		"url:a": {ID: "url:a", Status: StatusOK, LastSuccessAt: &old},
		"url:b": {ID: "url:b", Status: StatusNever},
	}

	updated, err := ApplyStateStalePolicy(&st, now, 30*time.Minute)
	if err != nil {
		t.Fatalf("apply state stale policy failed: %v", err)
	}
	if updated != 1 {
		t.Fatalf("expected 1 update, got %d", updated)
	}
	if st.Nodes["url:a"].Status != StatusStale {
		t.Fatalf("expected url:a stale")
	}
	if st.Nodes["url:b"].Status != StatusNever {
		t.Fatalf("expected url:b unchanged")
	}
}

func TestApplyChildRemovalTombstones(t *testing.T) {
	now := time.Date(2026, 3, 17, 12, 0, 0, 0, time.UTC)
	n := Node{Details: map[string]any{}}

	if err := ApplyChildRemovalTombstones(&n, []string{"url:c1", "url:c2", "url:c1"}, now, 2); err != nil {
		t.Fatalf("apply tombstones failed: %v", err)
	}
	raw := n.Details[TombstonesDetailsKey]
	tombs, ok := raw.([]Tombstone)
	if !ok {
		t.Fatalf("expected []Tombstone, got %T", raw)
	}
	if len(tombs) != 2 {
		t.Fatalf("expected bounded tombstones, got %d", len(tombs))
	}
	if tombs[0].ChildID != "url:c1" || tombs[1].ChildID != "url:c2" {
		t.Fatalf("unexpected tombstone order/content: %#v", tombs)
	}
}
