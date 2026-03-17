package state

import (
	"fmt"
	"time"
)

const TombstonesDetailsKey = "removed_child_tombstones"

type Tombstone struct {
	ChildID   string    `json:"child_id"`
	RemovedAt time.Time `json:"removed_at"`
}

// ApplyFetchSuccess transitions a node to OK and clears transient errors.
func ApplyFetchSuccess(node *Node, now time.Time) error {
	if node == nil {
		return fmt.Errorf("node is nil")
	}
	n := now.UTC()
	node.Status = StatusOK
	node.LastFetchedAt = ptrTimeValue(n)
	node.LastSuccessAt = ptrTimeValue(n)
	node.LastError = ""
	node.UpdatedAt = n
	return nil
}

// ApplyFetchFailure transitions node to ERROR while keeping existing readable data intact.
func ApplyFetchFailure(node *Node, now time.Time, errMsg string) error {
	if node == nil {
		return fmt.Errorf("node is nil")
	}
	n := now.UTC()
	node.Status = StatusError
	node.LastFetchedAt = ptrTimeValue(n)
	node.LastError = errMsg
	node.UpdatedAt = n
	return nil
}

// ApplyStalePolicy marks node as stale if its last successful fetch is older than ttl.
// Only OK nodes transition to STALE; ERROR and NEVER remain unchanged.
func ApplyStalePolicy(node *Node, now time.Time, ttl time.Duration) (bool, error) {
	if node == nil {
		return false, fmt.Errorf("node is nil")
	}
	if ttl <= 0 {
		return false, fmt.Errorf("ttl must be > 0")
	}
	if node.Status != StatusOK {
		return false, nil
	}
	if node.LastSuccessAt == nil {
		return false, nil
	}
	if now.UTC().Sub(node.LastSuccessAt.UTC()) < ttl {
		return false, nil
	}
	node.Status = StatusStale
	node.UpdatedAt = now.UTC()
	return true, nil
}

// ApplyStateStalePolicy applies TTL staleness to all nodes and returns update count.
func ApplyStateStalePolicy(st *State, now time.Time, ttl time.Duration) (int, error) {
	if st == nil {
		return 0, fmt.Errorf("state is nil")
	}
	updated := 0
	for id, node := range st.Nodes {
		changed, err := ApplyStalePolicy(&node, now, ttl)
		if err != nil {
			return updated, err
		}
		if changed {
			st.Nodes[id] = node
			updated++
		}
	}
	return updated, nil
}

// ApplyChildRemovalTombstones stores optional tombstones for removed children in node.details.
func ApplyChildRemovalTombstones(node *Node, removedChildIDs []string, now time.Time, maxItems int) error {
	if node == nil {
		return fmt.Errorf("node is nil")
	}
	if maxItems <= 0 {
		return nil
	}
	if len(removedChildIDs) == 0 {
		return nil
	}
	if node.Details == nil {
		node.Details = map[string]any{}
	}

	existing := make([]Tombstone, 0)
	if raw, ok := node.Details[TombstonesDetailsKey]; ok {
		existing = coerceTombstones(raw)
	}

	n := now.UTC()
	for _, childID := range uniqueNonEmptyPreserveOrder(removedChildIDs) {
		existing = append(existing, Tombstone{ChildID: childID, RemovedAt: n})
	}

	if len(existing) > maxItems {
		existing = existing[len(existing)-maxItems:]
	}
	node.Details[TombstonesDetailsKey] = existing
	node.UpdatedAt = n
	return nil
}

func coerceTombstones(raw any) []Tombstone {
	switch v := raw.(type) {
	case []Tombstone:
		return append([]Tombstone(nil), v...)
	case []any:
		out := make([]Tombstone, 0, len(v))
		for _, item := range v {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			childID, _ := m["child_id"].(string)
			removedAtStr, _ := m["removed_at"].(string)
			if childID == "" || removedAtStr == "" {
				continue
			}
			if ts, err := time.Parse(time.RFC3339, removedAtStr); err == nil {
				out = append(out, Tombstone{ChildID: childID, RemovedAt: ts.UTC()})
			}
		}
		return out
	default:
		return []Tombstone{}
	}
}

func ptrTimeValue(t time.Time) *time.Time {
	tt := t.UTC()
	return &tt
}
