package state

import (
	"fmt"
	"reflect"
	"time"
)

type ChildrenDiff struct {
	Added     []string
	Removed   []string
	Unchanged []string
}

func UpsertNode(st *State, patch Node, now time.Time) (created bool, changed bool, err error) {
	if st == nil {
		return false, false, fmt.Errorf("state is nil")
	}
	if patch.ID == "" {
		return false, false, fmt.Errorf("node id is required")
	}
	if st.Nodes == nil {
		st.Nodes = map[string]Node{}
	}

	patch = normalizeNode(patch)
	existing, ok := st.Nodes[patch.ID]
	if !ok {
		if patch.CreatedAt.IsZero() {
			patch.CreatedAt = now.UTC()
		}
		if patch.UpdatedAt.IsZero() {
			patch.UpdatedAt = now.UTC()
		}
		st.Nodes[patch.ID] = patch
		return true, true, nil
	}

	merged := patch
	merged.CreatedAt = existing.CreatedAt
	if merged.CreatedAt.IsZero() {
		merged.CreatedAt = now.UTC()
	}
	if merged.UpdatedAt.IsZero() {
		merged.UpdatedAt = existing.UpdatedAt
	}

	if !nodeEqualIgnoringUpdatedAt(existing, merged) {
		merged.UpdatedAt = now.UTC()
		st.Nodes[patch.ID] = merged
		return false, true, nil
	}

	return false, false, nil
}

func SetChildren(st *State, parentID string, childIDs []string, now time.Time) (ChildrenDiff, error) {
	if st == nil {
		return ChildrenDiff{}, fmt.Errorf("state is nil")
	}
	if parentID == "" {
		return ChildrenDiff{}, fmt.Errorf("parent id is required")
	}
	parent, ok := st.Nodes[parentID]
	if !ok {
		return ChildrenDiff{}, fmt.Errorf("parent node not found: %s", parentID)
	}

	nextChildIDs := uniqueNonEmptyPreserveOrder(childIDs)
	diff := DiffChildren(parent.ChildIDs, nextChildIDs)

	for _, childID := range diff.Added {
		child, ok := st.Nodes[childID]
		if !ok {
			return ChildrenDiff{}, fmt.Errorf("child node not found: %s", childID)
		}
		parents := uniqueNonEmptyPreserveOrder(append(child.ParentIDs, parentID))
		if !stringSlicesEqual(parents, child.ParentIDs) {
			child.ParentIDs = parents
			child.UpdatedAt = now.UTC()
			st.Nodes[childID] = child
		}
	}

	for _, childID := range diff.Removed {
		child, ok := st.Nodes[childID]
		if !ok {
			continue
		}
		parents := removeString(child.ParentIDs, parentID)
		if !stringSlicesEqual(parents, child.ParentIDs) {
			child.ParentIDs = parents
			child.UpdatedAt = now.UTC()
			st.Nodes[childID] = child
		}
	}

	if !stringSlicesEqual(parent.ChildIDs, nextChildIDs) || !parent.ChildrenHydrated {
		parent.ChildIDs = nextChildIDs
		parent.ChildrenHydrated = true
		parent.UpdatedAt = now.UTC()
		st.Nodes[parentID] = parent
	}

	return diff, nil
}

func DiffChildren(current []string, next []string) ChildrenDiff {
	currentNorm := uniqueNonEmptyPreserveOrder(current)
	nextNorm := uniqueNonEmptyPreserveOrder(next)

	currentSet := make(map[string]struct{}, len(currentNorm))
	nextSet := make(map[string]struct{}, len(nextNorm))
	for _, id := range currentNorm {
		currentSet[id] = struct{}{}
	}
	for _, id := range nextNorm {
		nextSet[id] = struct{}{}
	}

	diff := ChildrenDiff{
		Added:     make([]string, 0),
		Removed:   make([]string, 0),
		Unchanged: make([]string, 0),
	}

	for _, id := range nextNorm {
		if _, ok := currentSet[id]; ok {
			diff.Unchanged = append(diff.Unchanged, id)
			continue
		}
		diff.Added = append(diff.Added, id)
	}
	for _, id := range currentNorm {
		if _, ok := nextSet[id]; !ok {
			diff.Removed = append(diff.Removed, id)
		}
	}

	return diff
}

func CheckEdgeConsistency(st State) error {
	for parentID, parent := range st.Nodes {
		for _, childID := range parent.ChildIDs {
			child, ok := st.Nodes[childID]
			if !ok {
				return fmt.Errorf("node %s references missing child %s", parentID, childID)
			}
			if !containsString(child.ParentIDs, parentID) {
				return fmt.Errorf("missing reverse parent edge: parent=%s child=%s", parentID, childID)
			}
		}
	}

	for childID, child := range st.Nodes {
		for _, parentID := range child.ParentIDs {
			parent, ok := st.Nodes[parentID]
			if !ok {
				return fmt.Errorf("node %s references missing parent %s", childID, parentID)
			}
			if !containsString(parent.ChildIDs, childID) {
				return fmt.Errorf("missing reverse child edge: parent=%s child=%s", parentID, childID)
			}
		}
	}

	return nil
}

func nodeEqualIgnoringUpdatedAt(a Node, b Node) bool {
	a.UpdatedAt = time.Time{}
	b.UpdatedAt = time.Time{}
	return reflect.DeepEqual(normalizeNode(a), normalizeNode(b))
}

func normalizeNode(n Node) Node {
	n.ParentIDs = uniqueNonEmptyPreserveOrder(n.ParentIDs)
	n.ChildIDs = uniqueNonEmptyPreserveOrder(n.ChildIDs)
	if n.Assets == nil {
		n.Assets = []AssetRef{}
	}
	if n.Status == "" {
		n.Status = StatusNever
	}
	return n
}

func uniqueNonEmptyPreserveOrder(ids []string) []string {
	if len(ids) == 0 {
		return []string{}
	}
	seen := make(map[string]struct{}, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func removeString(in []string, target string) []string {
	out := make([]string, 0, len(in))
	for _, v := range in {
		if v != target {
			out = append(out, v)
		}
	}
	return uniqueNonEmptyPreserveOrder(out)
}

func containsString(in []string, target string) bool {
	for _, v := range in {
		if v == target {
			return true
		}
	}
	return false
}

func stringSlicesEqual(a []string, b []string) bool {
	return reflect.DeepEqual(uniqueNonEmptyPreserveOrder(a), uniqueNonEmptyPreserveOrder(b))
}
