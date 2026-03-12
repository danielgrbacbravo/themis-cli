# Themis-CLI
a CLI/TUI Client for the RUG judging software (aka Themis)

## Run

```sh
go run ./cmd/themis \
  --course-url "https://themis.housing.rug.nl/course/2023-2024/progfun/" \
  --depth 1
```

## Configuration

- Cookie file default: `$HOME/.config/themis/cookie.txt`
- Cookie format: `session=...; _ga=...; usernameType=student`

You can override runtime config with flags or env vars:

- `--base-url` or `THEMIS_BASE_URL`
- `--cookie-path` or `THEMIS_COOKIE_PATH`
- `--course-url` or `THEMIS_COURSE_URL`
- `--depth`
