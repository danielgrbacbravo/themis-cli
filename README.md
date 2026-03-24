# Themis-CLI
a CLI/TUI Client for the RUG judging software (aka Themis)

## Build

```sh
go build -o themis ./cmd/themis
```

## Usage

### themis
Open the interactive TUI application.

```sh
./themis \
  --base-url "https://themis.housing.rug.nl" \
  --session-file "$HOME/.config/themis/session.json"
```

If the persisted session is invalid, bare `themis` opens the login TUI first and then continues into the app on success.

### login
Authenticate either interactively or non-interactively.

Interactive login:

```sh
./themis login \
  --base-url "https://themis.housing.rug.nl" \
  --session-file "$HOME/.config/themis/session.json"
```

Non-interactive login:

```sh
./themis login \
  --base-url "https://themis.housing.rug.nl" \
  --session-file "$HOME/.config/themis/session.json" \
  --username "s1234567" \
  --password "secret" \
  --totp "123456"
```

Notes:
- `themis login` with no auth flags opens the login TUI.
- `themis login` with any of `--username`, `--password`, or `--totp` performs a non-interactive login attempt.
- If username/password are already saved in auth settings, `--totp` alone is enough for non-interactive login.
- Non-TUI commands never open the login TUI; they return `not authenticated` or `session expired`.

### auth
View or update persisted auth settings stored alongside the session.

```sh
./themis auth \
  --session-file "$HOME/.config/themis/session.json"
```

Example updates:

```sh
./themis auth --save-username=true --save-password=true
./themis auth --username "s1234567"
./themis auth --clear-password
```

### check
Validate base URL access and authentication.

```sh
./themis check \
  --base-url "https://themis.housing.rug.nl" \
  --session-file "$HOME/.config/themis/session.json"
```

### list test cases
Probe tests from a tests URL (or a specific test file URL) and return valid indices (`N.in` and `N.out` must both exist).

```sh
./themis list \
  --base-url "https://themis.housing.rug.nl" \
  --session-file "$HOME/.config/themis/session.json" \
  --tests-url "https://themis.housing.rug.nl/file/.../%40tests/1.in" \
  --start 1 --max 50 --max-misses 5
```

### list assignments recursively
Discover assignment/exercise URLs so agents can find targets without manually provided exercise URLs.

```sh
./themis list \
  --base-url "https://themis.housing.rug.nl" \
  --session-file "$HOME/.config/themis/session.json" \
  --discover \
  --root-url "https://themis.housing.rug.nl/course/2025-2026/os/" \
  --discover-depth 6
```

### fetch
Download discovered `.in/.out` pairs.

```sh
./themis fetch \
  --base-url "https://themis.housing.rug.nl" \
  --session-file "$HOME/.config/themis/session.json" \
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

TUI behavior:
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
- `--session-file` or `THEMIS_SESSION_FILE`
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

`login` flags:
- `--username`
- `--password`
- `--totp`

`auth` flags:
- `--username`
- `--password`
- `--save-username`
- `--save-password`
- `--clear-username`
- `--clear-password`

`project link` flags:
- `--root-url`
- `--default-refresh-depth`
- `--auto-refresh-on-open`
- `--show-stale-warning-after-minutes`

Bare `themis` flags:
- `--root-url`

Auth/session persistence:
1. Session cookies, saved credentials, and auth preferences live in one session file.
2. Default session file path is `$HOME/.config/themis/session.json`.
3. Commands other than bare `themis` do not auto-launch interactive login.

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
