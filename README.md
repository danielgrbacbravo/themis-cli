# Themis-CLI
a CLI/TUI Client for the RUG judging software (aka Themis)

## Build

```sh
go build -o themis ./cmd/themis
```

## Usage

### check
Validate base URL access and authentication.

```sh
./themis check \
  --base-url "https://themis.housing.rug.nl" \
  --cookie-file "$HOME/.config/themis/cookie.txt"
```

### list test cases
Probe tests from a tests URL (or a specific test file URL) and return valid indices (`N.in` and `N.out` must both exist).

```sh
./themis list \
  --base-url "https://themis.housing.rug.nl" \
  --cookie-file "$HOME/.config/themis/cookie.txt" \
  --tests-url "https://themis.housing.rug.nl/file/.../%40tests/1.in" \
  --start 1 --max 50 --max-misses 5
```

### list assignments recursively
Discover assignment/exercise URLs so agents can find targets without manually provided exercise URLs.

```sh
./themis list \
  --base-url "https://themis.housing.rug.nl" \
  --cookie-file "$HOME/.config/themis/cookie.txt" \
  --discover \
  --root-url "https://themis.housing.rug.nl/course/2025-2026/os/" \
  --discover-depth 6
```

### fetch
Download discovered `.in/.out` pairs.

```sh
./themis fetch \
  --base-url "https://themis.housing.rug.nl" \
  --cookie-file "$HOME/.config/themis/cookie.txt" \
  --tests-url "https://themis.housing.rug.nl/file/.../%40tests/1.in"
```

Default output directory is `./tests` for the `fetch` command (resolved to absolute path at runtime).  
If missing, it is created automatically. Override with `--out`.

### project link
Link the current repository to a Themis course root so state-first discovery and TUI can resolve the active root without `--root-url`.

```sh
./themis project link \
  --base-url "https://themis.housing.rug.nl" \
  --root-url "https://themis.housing.rug.nl/course/2025-2026/os" \
  --default-refresh-depth 1
```

This writes `.themis/project.json` in the repo root.

### tui
Open the cached hierarchy browser.

```sh
./themis tui \
  --base-url "https://themis.housing.rug.nl" \
  --cookie-file "$HOME/.config/themis/cookie.txt"
```

`themis tui` behavior:
- Uses cached state immediately (no startup crawl).
- Supports targeted refresh actions from the selected node.
- Download mode supports multi-select and per-file progress:
  - `…` active
  - `✓` completed
  - `✗` failed
  - `·` pending
- Uses terminal-adaptive text colors only (no background fills), so it follows your terminal theme.

Download path rules in TUI:
- `%40tests` assets download under `./tests/...`
- Regular files download under current working directory
- Path-like regular assets (for example `/imgs/1.img`) preserve relative path (`./imgs/1.img`)

## Flags And Env

Common flags on all subcommands:
- `--base-url` or `THEMIS_BASE_URL`
- `--cookie-file` or `THEMIS_COOKIE_FILE` (fallback: `THEMIS_COOKIE_PATH`)
- `--cookie-env` or `THEMIS_COOKIE_ENV` (name of env var that contains cookie string)
- `--json`

`list` flags:
- `--tests-url`
- `--start`
- `--max`
- `--max-misses`
- `--discover`
- `--root-url`
- `--discover-depth`
- `--refresh-url`
- `--refresh-depth`
- `--full-refresh`
- `--from-state-only`

`fetch` flags:
- `--tests-url`
- `--out` (default: `./tests`)
- `--target-dir` (deprecated alias for `--out`)

`project link` flags:
- `--root-url`
- `--default-refresh-depth`
- `--auto-refresh-on-open`
- `--show-stale-warning-after-minutes`

`tui` flags:
- `--root-url`

Authentication cookie resolution order:
1. `--cookie-file`
2. `--cookie-env`
3. default path: `$HOME/.config/themis/cookie.txt`

## JSON Output

With `--json`, each command emits exactly one JSON object on stdout with fields:

- `status` (`"ok"` or `"error"`)
- `base_url`
- `tests`
- `downloaded`
- `files`
- `error` (only on failure)

Additional fields may be present depending on command:
- `authenticated`, `user` (`check`)
- `tests_base_url` (`list`, `fetch`)
- `assignments` (`list --discover`)
- `target_dir` (`fetch`)
- `mode`, `root_url`, `refreshed`, `refresh_scope` (`list --discover`)

Logs and human-readable output are written to stderr/non-JSON mode; JSON mode keeps stdout machine-parseable.
