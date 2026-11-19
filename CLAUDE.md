# CLAUDE.md — Workline

Read `SPEC.md` before any change. It is the source of truth; `docs/mockup.html` is the visual reference.

## Principles
- **Standard library first.** Allowed dependencies: `modernc.org/sqlite` and htmx 2.0.11 (vendored at `internal/web/static/htmx.min.js`, never loaded from a CDN). Ask before adding anything else, including test helpers.
- **Local-first.** Bind to `127.0.0.1` only. No telemetry, no outbound network calls, no web fonts.
- **Small, boring code.** No frameworks, no ORM, no code generation, no global state.

## Layout
```
cmd/workline/        main: flags, subcommands (serve, seed, backup, migrate, version)
internal/domain/     pure logic: burndown, progress, scope creep, periods. No I/O.
internal/store/      database/sql + SQLite, hand-written SQL, migrations
internal/web/        handlers, view models, templates, static (all embedded)
migrations/          0001_init.sql, 0002_….sql (embedded)
docs/                mockup and notes
```
Dependency direction: `web → store → domain` and `web → domain`. `domain` imports only the stdlib.

## Conventions
- Go 1.22+ `http.ServeMux` patterns (`POST /subtasks/{id}/toggle`), `r.PathValue`.
- Errors: wrap with `fmt.Errorf("…: %w", err)`; handlers log with `log/slog` and return a plain error page or fragment.
- Dates: a `domain.Date` type (`YYYY-MM-DD`, civil date, no time zone math in templates). "Today" comes from an injected `Clock` interface; tests use a fixed clock.
- SQL: parameterised queries only; one function per query in `store`; transactions for multi-row writes (e.g. scope-ack).
- Templates: `html/template` only. Build view models in Go. No logic in templates beyond `if`/`range`.
- htmx: every mutating form works without JS (POST → 303 redirect to the current dashboard URL). With `HX-Request`, return the fragment(s) to swap. Keep filters in the URL query string.
- CSS: one hand-written `web/static/app.css` using the tokens in SPEC §6. Vanilla JS only if htmx can't do it, kept in `web/static/app.js`.
- Accessibility: real buttons, links and labels; `aria-pressed`/`aria-expanded` where SPEC says.

## Testing
- `internal/domain`: table-driven tests for every rule in SPEC §4 (open/scope at date, progress, bins and clipping, every scope-creep branch including after an acknowledgement). Aim for full coverage here.
- `internal/store`: tests against a temp-file SQLite DB with migrations applied.
- `internal/web`: `httptest` tests for main flows and for both full-page and `HX-Request` responses.
- No mocks of the DB; use a real temp DB.

## Commands
```
go run ./cmd/workline serve -db ./dev.db
go run ./cmd/workline seed  -db ./dev.db
go test ./...
go vet ./...
gofmt -l .        # must print nothing
```

## Workflow
- Work one milestone at a time (SPEC §9). Start each with a short plan and wait for approval.
- Before saying a task is done: `gofmt`, `go vet ./...`, `go test ./...` all clean.
- Commit each finished step without asking. Keep commits small and single-scope, with imperative messages.
- Note user-visible changes in `CHANGELOG.md` under Unreleased. A release moves them under a new version heading, bumps `version` in `cmd/workline/main.go` to match (a test checks it), and gets an annotated `vX.Y.Z` tag.
- If the spec is ambiguous, ask; record the decision in SPEC §10.
