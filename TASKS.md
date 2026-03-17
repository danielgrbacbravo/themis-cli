
1. **Lock contracts and scope**
- Goal: freeze data and URL identity rules before coding.
- Tasks: define canonical URL function, node ID rule, status enums, schema versioning policy, stale policy.
- Deliverable: `docs/architecture/state-contract.md`.
- Example JSON:
```json
{
  "schema_version": 1,
  "id_rules": {
    "node_id": "url:<sha256(canonical_url)>",
    "canonical_url": "absolute, no query, no fragment, trim trailing slash"
  },
  "status_enum": ["never", "ok", "stale", "error"]
}
```
- Done when: team can generate identical node IDs from the same URL across machines.

2. **Create typed state models**
- Goal: create durable structs for global store and node graph.
- Tasks: add `internal/state/model.go` with `State`, `RootRef`, `Node`, `AssetRef`.
- Deliverable: compile-safe model package with JSON tags.
- Example JSON:
```json
{
  "schema_version": 1,
  "base_url": "https://themis.housing.rug.nl",
  "catalog_root_url": "https://themis.housing.rug.nl/course",
  "updated_at": "2026-03-17T10:00:00Z",
  "roots": [],
  "nodes": {}
}
```
- Done when: marshal/unmarshal round-trip test passes.

3. **Implement storage engine**
- Goal: safe read/write of `~/.config/themis/state.json`.
- Tasks: add `Load()`, `SaveAtomic()`, file lock, directory bootstrap, backup-on-write option.
- Deliverable: `internal/state/store_json.go`.
- Example JSON written by engine:
```json
{
  "schema_version": 1,
  "base_url": "https://themis.housing.rug.nl",
  "catalog_root_url": "https://themis.housing.rug.nl/course",
  "updated_at": "2026-03-17T10:01:12Z",
  "roots": [],
  "nodes": {}
}
```
- Done when: abrupt process interruption never leaves corrupted JSON.

4. **Add project link config**
- Goal: map repo to course root.
- Tasks: create `.themis/project.json` loader/saver, root resolution by cwd.
- Deliverable: `internal/projectlink`.
- Example JSON:
```json
{
  "schema_version": 1,
  "base_url": "https://themis.housing.rug.nl",
  "linked_root_url": "https://themis.housing.rug.nl/course/2025-2026/os",
  "linked_root_node_id": "url:7f0c8b...",
  "last_open_node_id": "url:aa90c1...",
  "preferences": {
    "default_refresh_depth": 1,
    "auto_refresh_on_open": false,
    "show_stale_warning_after_minutes": 120
  },
  "updated_at": "2026-03-17T10:02:00Z"
}
```
- Done when: CLI can resolve active root with no `--root-url` in linked repo.

5. **Build graph upsert/merge layer**
- Goal: update node graph incrementally.
- Tasks: implement `UpsertNode`, `SetChildren`, `DiffChildren`, edge consistency checks.
- Deliverable: `internal/state/graph.go`.
- Example node JSON:
```json
{
  "id": "url:7f0c8b...",
  "kind": "course",
  "title": "Operating Systems",
  "canonical_url": "https://themis.housing.rug.nl/course/2025-2026/os",
  "nav_api_url": "https://themis.housing.rug.nl/api/navigation/2025-2026/os",
  "parent_ids": ["url:year2025..."],
  "child_ids": ["url:lab1...", "url:lab2..."],
  "children_hydrated": true,
  "depth_hint": 3,
  "status": "ok",
  "last_fetched_at": "2026-03-17T10:03:00Z",
  "last_success_at": "2026-03-17T10:03:00Z",
  "last_error": "",
  "content_hash": "sha256:3a9d...",
  "details": {},
  "assets": [],
  "created_at": "2026-03-17T10:03:00Z",
  "updated_at": "2026-03-17T10:03:00Z"
}
```
- Done when: subtree refresh only mutates touched nodes and edges.

6. **Refactor discovery into node-refresh engine**
- Goal: move from root-only crawl to targeted refresh.
- Tasks: add `RefreshNode(url, depth)`, `RefreshCatalog()`, parser adapters for assignment children + metadata.
- Deliverable: `internal/discovery/refresh.go`.
- Example refresh result JSON:
```json
{
  "target_url": "https://themis.housing.rug.nl/course/2025-2026/os/lab3",
  "depth": 1,
  "fetched_nodes": 4,
  "updated_nodes": 3,
  "removed_edges": 1,
  "errors": []
}
```
- Done when: refreshing deep node does not fetch root.

7. **Wire CLI to state-first flow**
- Goal: keep existing commands, add persistence-aware behavior.
- Tasks: add flags `--refresh-url`, `--refresh-depth`, `--full-refresh`, `--from-state-only`, `project link` command.
- Deliverable: updated `cmd/themis/main.go`.
- Example JSON output for discover:
```json
{
  "status": "ok",
  "base_url": "https://themis.housing.rug.nl",
  "mode": "state-first",
  "root_url": "https://themis.housing.rug.nl/course/2025-2026/os",
  "refreshed": true,
  "refresh_scope": "subtree",
  "assignments": [
    {
      "name": "Assignment 1: System Diagnostics",
      "url": "https://themis.housing.rug.nl/course/2025-2026/os/lab1",
      "depth": 1,
      "parent_url": "https://themis.housing.rug.nl/course/2025-2026/os"
    }
  ]
}
```
- Done when: second run is fast and returns from cache without crawling unless requested.

8. **Persist enrichment fields for TUI**
- Goal: save useful metadata for rich display.
- Tasks: parse breadcrumb, details/config fields, stats link, optional assets.
- Deliverable: `details` and `assets` consistently populated when available.
- Example JSON:
```json
{
  "details": {
    "breadcrumb": ["Courses", "2025-2026", "Operating Systems"],
    "config": {
      "leading_submission": "latest",
      "end_iso": "2026-08-31T21:59:59.000Z",
      "end_display": "Mon Aug 31 2026 23:59:59 GMT+0200"
    },
    "links": {
      "status_page": "https://themis.housing.rug.nl/stats/2025-2026/os"
    }
  }
}
```
- Done when: UI can render metadata without refetching page.

9. **Add stale/consistency policy**
- Goal: predictable freshness behavior.
- Tasks: mark stale by TTL, set `error` on fetch fail, keep previous data readable, optional tombstones for removed children.
- Deliverable: policy module with deterministic transitions.
- Example JSON:
```json
{
  "id": "url:lab5...",
  "status": "stale",
  "last_success_at": "2026-03-10T08:00:00Z",
  "last_fetched_at": "2026-03-17T10:06:00Z",
  "last_error": "timeout while fetching node"
}
```
- Done when: stale and error states are visible and non-destructive.

10. **Build TUI foundation (Bubble Tea)**
- Goal: read-only tree browser from state.
- Tasks: create `internal/tui/app`, panes (tree/details/status), event loop, resize handling, keymap.
- Deliverable: `themis tui` showing cached graph instantly.
- Example runtime state JSON (optional debug dump):
```json
{
  "selected_node_id": "url:7f0c8b...",
  "expanded_node_ids": ["url:catalog...", "url:year2025...", "url:7f0c8b..."],
  "filter": "",
  "mode": "browse"
}
```
- Done when: user can navigate full cached hierarchy without network calls.

11. **Add TUI actions (refresh + open project root)**
- Goal: interactive incremental refresh.
- Tasks: bind keys for refresh node, refresh subtree, full refresh, jump to linked project root.
- Deliverable: async commands + progress/status messages in TUI.
- Example message JSON:
```json
{
  "type": "refresh_finished",
  "target_node_id": "url:7f0c8b...",
  "scope": "subtree",
  "updated_nodes": 12,
  "duration_ms": 842,
  "error": ""
}
```
- Done when: refresh actions update only targeted branch and repaint tree.

12. **Add TUI download workflows**
- Goal: choose assets/tests from selected node and download.
- Tasks: asset list panel, multi-select, reuse existing fetch logic, show result summary.
- Deliverable: download action from TUI with persisted recent choices.
- Example download summary JSON:
```json
{
  "type": "download_result",
  "node_id": "url:lab2...",
  "target_dir": "/Users/daniel/code/personal-repos/themis-cli/tests",
  "downloaded": 14,
  "files": [
    {"index": 1, "in_path": ".../1.in", "out_path": ".../1.out"}
  ],
  "error": ""
}
```
- Done when: user can run end-to-end from browse -> select -> download.

13. **Testing gates per phase**
- Goal: enforce correctness at each milestone.
- Tasks: unit tests for URL canonicalization, graph diffs, stale transitions, store atomicity, CLI integration fixtures, TUI model tests.
- Deliverable: CI stages mapped to phases.
- Example JSON test fixture:
```json
{
  "input_url": "https://themis.housing.rug.nl/course/2025-2026/os/",
  "canonical_url": "https://themis.housing.rug.nl/course/2025-2026/os",
  "node_id_prefix": "url:"
}
```
- Done when: all phase gates pass before moving forward.

14. **Release phases**
- Goal: ship safely with clear increments.
- Tasks: Phase A state engine + CLI state-first, Phase B refresh-by-node, Phase C TUI browse, Phase D TUI actions/download.
- Deliverable: changelog + migration note for schema_version.
- Example migration JSON:
```json
{
  "from_schema_version": 1,
  "to_schema_version": 2,
  "changes": ["added nav_api_url", "added assets[].kind"],
  "migrated_at": "2026-03-17T10:20:00Z"
}
```
- Done when: users can adopt gradually without losing previous cache.
