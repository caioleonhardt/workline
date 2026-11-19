# Changelog

All notable changes to Workline. Versions follow [Semantic Versioning](https://semver.org/);
each release is tagged `vX.Y.Z` in git.

## [Unreleased]

## [0.7.0] — 2026-11-19

### Added
- Keys `t`, `d`, `u`, `b` and `m` focus the journal bar with task, decision,
  update, blocker or meeting chosen. On the journal page, `/` focuses the bar
  instead of opening the browser's quick find.

### Fixed
- Completion popups float beside their dot instead of opening inside the
  timeline's scrolling box, where they were cut off.

## [0.6.0] — 2026-11-19

### Added
- A journal page at `/`, for weekly 1:1s. It shows two weeks (‹ › to move):
  - tiles for tasks opened, tasks closed, tasks open now and high-priority tasks open;
  - one burndown of all tasks, with dots for completed tasks;
  - open tasks by priority, with a checkbox to close each;
  - the journal entries of those weeks.
- On the journal page, the bar has an Activity select (Unassigned by default).
- Journal · Timeline · Projects links in the top bar.

### Changed
- The timeline moved to `/timeline`. Old `/?…` links redirect there.

## [0.5.0] — 2026-11-19

Upgrading from 0.4.0: run `workline migrate` once before `serve`.

### Added
- Tasks have a priority (high, medium, low or none). The journal bar has a
  priority select for tasks, and task Edit forms can change it.
- Prefixes in the journal bar: `/t` task, `/t1`–`/t3` task with high, medium
  or low priority, `/d` decision, `/u` update, `/b` blocker, `/m` meeting.

### Changed
- The panel lists open tasks first, by priority, with a priority chip.

## [0.4.0] — 2026-11-19

Upgrading from 0.3.0: run `workline migrate` once before `serve`.

### Added
- Log to the journal without choosing an activity. With nothing selected, the
  bar logs to **Unassigned**; a × next to a selected activity switches back to
  Unassigned.
- File an Unassigned entry from its Edit form in the feed by picking an
  activity.
- Unassigned entries show in the feed (unless a project or activity narrows
  it), count in "Open tasks now", and have their own section in the export.

### Changed
- The journal bar is always visible.

## [0.3.0] — 2026-11-19

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

[Unreleased]: ../../compare/v0.7.0...HEAD
[0.7.0]: ../../compare/v0.6.0...v0.7.0
[0.6.0]: ../../compare/v0.5.0...v0.6.0
[0.5.0]: ../../compare/v0.4.0...v0.5.0
[0.4.0]: ../../compare/v0.3.0...v0.4.0
[0.3.0]: ../../compare/v0.2.0...v0.3.0
[0.2.0]: ../../compare/v0.1.0...v0.2.0
[0.1.0]: ../../releases/tag/v0.1.0
