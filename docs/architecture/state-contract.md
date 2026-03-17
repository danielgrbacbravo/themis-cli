# State Contract

This document freezes the identity and state rules for persistent Themis graph data.

## Schema

- `schema_version`: integer, starts at `1`.
- `base_url`: canonical Themis base URL.
- `catalog_root_url`: canonical URL for the catalog root (for example `/course`).
- `updated_at`: RFC3339 timestamp in UTC (`time.RFC3339`).
- `roots`: list of tracked roots (`RootRef`).
- `nodes`: map of `node_id -> Node`.

## URL Identity Rules

Canonical URL rules for all persisted node URLs:

1. Input must be absolute (`http` or `https`) and include host.
2. Strip query (`?`) and fragment (`#`).
3. Trim trailing slash from path except root path `/`.
4. Keep URL absolute in normalized string form.

Node ID rule:

- `node_id = "url:" + hex(sha256(canonical_url))`

This guarantees deterministic IDs for the same URL across machines.

## Status Enum

Allowed node status values:

- `never`: never fetched.
- `ok`: last fetch succeeded and data is fresh.
- `stale`: data kept, but requires refresh by TTL policy.
- `error`: last fetch failed; previous successful data may still exist.

## Staleness Policy

Transitions are deterministic and non-destructive:

- Successful refresh: `never|stale|error -> ok` and clear `last_error`.
- Failed refresh with previous data: `ok|stale -> error` and keep existing content.
- Failed refresh with no data: `never -> error`.
- TTL expiry (without fetch): `ok -> stale`.

A stale or error node remains readable from cache.

## Versioning Policy

- Backward-compatible additions (new optional fields): increment minor behavior docs only, keep `schema_version`.
- Breaking shape changes or semantic reinterpretation: increment `schema_version`.
- Migrations must be explicit and idempotent (`from -> to`), with timestamped migration metadata.

## Contract Sample

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
