# Changelog

All notable changes to Workline. Versions follow [Semantic Versioning](https://semver.org/);
each release is tagged `vX.Y.Z` in git.

## [Unreleased]

### Added
- `workline version` prints the release, the database schema it needs and,
  for a build from a git checkout, the revision.

## [0.2.0] — 2026-11-19

Upgrading from 0.1.0: run `workline migrate` once before `serve`. It backs the
database up to `backups/` first.

### Added
- `workline migrate` upgrades an existing database: backup, pending migrations,
  integrity check, and row counts per table before and after.
- Tasks can be logged from the journal bar with kind **Task**.
- Journal entries can span several lines. In the bar, Enter logs and
  Shift+Enter starts a new line. Line breaks show in the feed, the panel and the
  Markdown export.

### Changed
- Subtasks are now called tasks everywhere: UI, routes (`/tasks/{id}/toggle`),
  code, SPEC and seed data.
- Tasks are journal entries of kind `task` (migration 0005). The panel still
  lists the selected activity's tasks; the feed shows them with a Task chip,
  struck through once closed.
- A task stays a task: Edit can't turn an entry into a task or a task into
  another kind.
- "Activity created" is logged before the activity's first tasks.
- `serve` and `seed` refuse a database at an older schema instead of migrating
  it silently. A new database is still set up automatically.

### Removed
- The panel's own add-task form, and the `/activities/{id}/tasks`,
  `/tasks/{id}` and `/tasks/{id}/delete` routes. Tasks are edited and deleted
  through `/journal/{id}`.

## [0.1.0] — 2026-11-19

First release.

### Added
- Projects and activities with requester, priority, start and due dates, and
  status (planned, active, blocked, done).
- Subtasks and a dated journal per activity, all stored in local SQLite.
- Dashboard timeline by year or quarter, grouped by project, with burndown bins,
  completion dots, summary tiles, and filters kept in the URL.
- Detail panel with a weekly burndown chart, subtasks, journal and scope-creep
  section, with acknowledgement.
- htmx partial updates; every form also works without JavaScript.
- Journal feed below the timeline and a fixed quick-add bar.
- Journal Markdown export, `backup` command, and keyboard shortcuts (`n`, `/`,
  `j`, `Esc`).
- `serve` on 127.0.0.1:7357 and `seed` with the baseline data.
- CI running gofmt, vet and tests.

[Unreleased]: ../../compare/v0.2.0...HEAD
[0.2.0]: ../../compare/v0.1.0...v0.2.0
[0.1.0]: ../../releases/tag/v0.1.0
