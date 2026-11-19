# Workline

Local-first tracker for a tech lead's work: projects, subtasks, journals, and burndown timelines with scope-creep signals.

Workline tracks everything you are asked to do, not only what lives in Jira. Activities are grouped by project, broken into subtasks, documented in a dated journal, and drawn on a year or quarter timeline as burndowns of open subtasks over time.

It is a single Go binary with an embedded SQLite database. It works offline, listens on `127.0.0.1` only, and makes no outbound network calls.

## Features

- **Timeline dashboard** by year or quarter, grouped by project, with filters for project, month, status and scope creep. Every view is a bookmarkable URL.
- **Burndowns** computed from subtask open/close dates, never entered by hand.
- **Scope-creep signal** when an activity grows 75% past its baseline, or when work arrives faster than it closes over the last 30 days. Acknowledging it resets the baseline and logs the change to the journal.
- **Journal** per activity, exportable as Markdown for 1:1 preparation.
- **Keyboard shortcuts**: `n` new activity, `/` focus filter, `Esc` close panel.

## Install

Requires Go 1.27 or later. No cgo.

```
go install ./cmd/workline
```

## Usage

```
workline serve     # http://127.0.0.1:7357
workline backup    # copy the database to <data dir>/backups/
workline seed      # load sample data into an empty database
workline migrate   # back up, then upgrade an existing database after an update
```

Every command works with no arguments. The database defaults to `$XDG_DATA_HOME/workline/workline.db`, falling back to `~/.local/share/workline/workline.db`.

| flag | commands | meaning |
|---|---|---|
| `-db path` | all | database file |
| `-addr host:port` | `serve` | listen address (default `127.0.0.1:7357`) |
| `-port N` | `serve` | override only the port |

`workline backup [dest]` uses `VACUUM INTO`, refuses to overwrite an existing file, and never modifies the source.

## Development

```
go run ./cmd/workline seed  -db ./dev.db
go run ./cmd/workline serve -db ./dev.db
go test ./...
go vet ./...
gofmt -l .
```

[`SPEC.md`](SPEC.md) is the source of truth for behaviour; [`docs/mockup.html`](docs/mockup.html) is the visual reference.

```
cmd/workline/     subcommands: serve, seed, backup, migrate
internal/domain/  pure logic: burndown, progress, scope creep, periods
internal/store/   SQLite, hand-written SQL, migrations
internal/web/     handlers, templates, static assets (embedded)
migrations/       numbered SQL migrations (embedded)
```

The only dependencies are [`modernc.org/sqlite`](https://pkg.go.dev/modernc.org/sqlite) and a vendored copy of [htmx](https://htmx.org).
