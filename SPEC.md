# Workline — Specification (v1)

A local-first tracker for a tech lead's activities: everything you are asked to do, not only what lives in Jira. Activities are grouped by **project**, broken into **subtasks**, documented in a **journal**, and shown on a yearly timeline as **burndowns** (open subtasks over time) with a **scope-creep** signal.

The visual reference is the mockup in `docs/mockup.png` (exported from the design canvas). When this spec and the mockup disagree, this spec wins.

---

## 1. Goals and non-goals

**Goals (v1)**
- Single user, single machine, single binary. Works offline.
- Track projects → activities → subtasks, with a dated journal per activity.
- Year and quarter timeline dashboards with filters.
- Burndown and scope-creep computed from dated events, never entered by hand.

**Non-goals (v1)**
- Authentication, multiple users, sharing, sync, hosting.
- Jira or calendar integrations (possible later: import an epic's issues as subtasks).
- Mobile app. The page must still work at narrow widths.

---

## 2. Stack

- Go (1.22+ for `net/http` routing patterns), standard library first.
- SQLite via `modernc.org/sqlite` (pure Go, no cgo). Only Go dependency.
- `html/template` for server-rendered HTML.
- htmx, **vendored** as one file under `web/static/` and served via `go:embed`. No CDN, no npm, no build step.
- Tiny vanilla JS only where htmx does not fit.
- System font stack (no web fonts, the app must work offline).
- Binary listens on `127.0.0.1` only.

---

## 3. Domain model

All calendar values are **dates** (`YYYY-MM-DD`, user's local time zone) unless stated. "Today" comes from an injectable clock.

### Project
| field | type | notes |
|---|---|---|
| id | int | |
| name | text | required, unique |
| archived | bool | archived projects are hidden from the dashboard |
| created_at | timestamp | |

### Activity
| field | type | notes |
|---|---|---|
| id | int | |
| project_id | int | required, FK |
| name | text | required |
| requested_by | text | free text, e.g. "Manager", "Director", "Self" |
| status | enum | `planned`, `active`, `blocked`, `done` |
| start_date | date | required |
| due_date | date | required, ≥ start_date |
| created_at | timestamp | |

On creation, status defaults to `planned` if `start_date > today`, else `active`.

### Subtask
| field | type | notes |
|---|---|---|
| id | int | monotonically increasing |
| activity_id | int | FK |
| text | text | required |
| created_on | date | defaults to today; editable (for backfilling) |
| closed_on | date, nullable | null = open. Closing sets today; reopening sets null |

### JournalEntry
| field | type | notes |
|---|---|---|
| id | int | |
| activity_id | int | FK |
| date | date | defaults to today; editable |
| kind | enum | `update`, `decision`, `blocker`, `meeting`, `scope_ack` |
| text | text | required |

`scope_ack` entries are created only by the Acknowledge action (§5.4), never from the journal form.

### ScopeAck
| field | type | notes |
|---|---|---|
| id | int | |
| activity_id | int | FK |
| date | date | |
| baseline_count | int | number of subtasks at acknowledgement |
| last_subtask_id | int | highest subtask id at acknowledgement (0 if none) |

Only the most recent ScopeAck per activity matters for computation; older ones are history.

Deleting a project or activity cascades. Deletions require confirmation in the UI.

---

## 4. Derived values (pure domain logic)

All functions below live in `internal/domain`, take plain structs and a `today` date, and have no I/O.

### 4.1 Open and scope at a date
- `openAt(a, t)` = subtasks with `created_on ≤ t` and (`closed_on` is null or `closed_on > t`).
- `scopeAt(a, t)` = subtasks with `created_on ≤ t`.

### 4.2 Progress
- With subtasks: `round(closed / total × 100)`.
- Without subtasks: `100` if status is `done`, else `0`.

Project progress = closed / total over all subtasks of its visible activities.

### 4.3 Burndown series
For an activity, from `start_date` to `min(due_date, today)`, split into bins of size `step`. Each bin reports `openAt(a, binEnd)`.

- Timeline rows: `step` = 7 days in Year view, 2 days in Quarter view.
- Detail panel: always 7 days, over the full `start_date … due_date` span. Bins after today are "ahead" (no value, drawn as a dashed frame). The panel bin also reports `scopeAt` for the scope layer.
- Row bar height is scaled by **one global maximum**: the highest weekly `openAt` reached by any activity. This keeps rows comparable ("taller = more open").
- Bins outside the visible period are dropped; bins crossing its edge are clipped.

### 4.4 Scope creep
Constants: `GrowthThreshold = 75` (%), `Window = 30` days, `KickoffGrace = 7` days.

```
ack      = latest ScopeAck for the activity, if any
if ack:
    baseline = ack.baseline_count
    newer    = subtasks with id > ack.last_subtask_id
else:
    newer    = subtasks with created_on > start_date + KickoffGrace
    baseline = total - len(newer)

added    = total - baseline
growth%  = baseline > 0 ? round(added / baseline × 100) : 0
add30    = newer with created_on > today - Window
close30  = all subtasks with closed_on > today - Window

growthFlag    = status != done && baseline > 0 && growth% >= GrowthThreshold
outpacingFlag = status != done && add30 > close30
flagged       = growthFlag || outpacingFlag
```

Reason texts (shown in the detail panel):
- growth: `Grew from {baseline} to {total} subtasks since kickoff (+{growth}%).`, or `…since the acknowledgement on {date}…` when acknowledged.
- outpacing: `In the last 30 days {add30} added vs {close30} closed: work is arriving faster than it closes.`

### 4.5 Period columns and load
- **Year / Quarters**: 4 columns Q1–Q4. **Year / Months**: 12 columns. **Qn**: 3 month columns plus weekly gridlines labelled with Monday's day of month.
- Column load label:
  - Column fully in the future: `{n} planned` (visible activities overlapping it).
  - Otherwise: `{open} open · {n} act.`, where `open` = Σ `openAt(a, min(columnEnd, today))` over visible activities that had started by then.
- The column containing today is tinted; the column(s) overlapping a selected month are tinted differently.

---

## 5. Features

### 5.1 Dashboard (`GET /`)
Header with the app name and period eyebrow (e.g. `2026 · ACTIVITY TIMELINE` or `Q4 2026 · OCT–DEC`) and a **New activity** button.

Controls, all reflected in the **URL query string** so any view is bookmarkable:

| param | values | default |
|---|---|---|
| `year` | e.g. `2026` | current year |
| `period` | `year`, `q1`…`q4` | `year` |
| `scale` | `quarter`, `month` (Year only) | `quarter` |
| `project` | project id or empty | all |
| `month` | `1`–`12` or empty; limited to the quarter's months in Qn | all |
| `status` | `planned`, `active`, `blocked`, `done` or empty | all |
| `creep` | `1` or empty | off |
| `sel` | activity id | none |

- Changing `period` clears `month`.
- Status pills show counts (after the period/project/month filters, before the status filter).
- The scope-creep toggle shows the number of flagged activities in scope.
- "Clear filters" appears when any filter is active.

**Summary tiles** (respect the project filter only): Active, Blocked, Planned, Done this year, Open subtasks now, Scope creep flags.

**Timeline**
- Left column (300px): labels. Right: the period track.
- Overlays on the track: gridlines, a shaded band for the selected month, and a **Today** line with a label.
- Grouped by project. **Project row = one line**: chevron, name, `· N activities`, `· P% done`, and `· K scope flags` if any. No burndown on project rows. Clicking toggles collapse (state kept in a cookie or `localStorage`).
- **Activity row**:
  - Left: name, status chip, a scope chip (`Scope +N`) when flagged, `open / total`, then dates and requester.
  - Track: a thin span line from start to due; burndown bins in the status colour; a dashed box from today to due (not for `done`); journal dots coloured by kind; `P%` at the right end.
  - Clicking the row or bar selects the activity (`sel=`) and opens the detail panel.
- Empty state: "No activities match these filters."
- Legend below the timeline.

### 5.2 Detail panel
Shown beside the timeline (below it on narrow screens) when `sel` is set.
- Project name (eyebrow), activity name, requested by, dates.
- Status select (saves on change), progress %, `x of y subtasks done`.
- **Burndown** (weekly): scope layer, open layer, dashed future weeks, today marker. Caption `peak N open · weekly`.
- **Scope**: three stats, then the warning if flagged.
  - The stats are Baseline ("At kickoff", or "Baseline (ack {date})"), Now with growth %, and Last 30 days `+add30 / −close30`.
  - When flagged, a warning shows the reason text and the **Acknowledge and log to journal** button.
  - If acknowledged and not flagged, a confirmation line shows the ack date and baseline.
  - Helper text explains the rule.
- **Subtasks**: checkbox list (toggle closes or reopens), `opened {date}` / `closed {date}` on the right, add form, edit and delete per subtask.
- **Journal**: form (kind select + text + Log), list newest first with date, kind chip and text. Edit and delete per entry, except `scope_ack` entries, which are read-only.
- Edit activity (name, requested by, dates, project) and delete activity.

### 5.3 Create flows
- **New activity**: project select (with "+ New project…" which reveals a name field), name, requested by, start, due. On success, select the new activity. It also logs a journal entry `Activity created`.
- **New project**: through the activity form above, plus a simple projects page (`/projects`) to rename, archive and delete.

### 5.4 Acknowledge scope creep
`POST /activities/{id}/scope-ack`, in one transaction:
1. Insert a ScopeAck with today, the current subtask count and the max subtask id.
2. Insert a JournalEntry, kind `scope_ack`, with text
   `Scope change acknowledged: {baseline} → {total} subtasks (+{growth}%); last 30 days +{add30} / −{close30}. New baseline: {total}.`
   (omit the `(+…%)` part when nothing was added).

Effect: the flag clears and, per §4.4, returns only if scope grows again from the new baseline.

### 5.5 Export (milestone 5)
`GET /export/journal.md?project=&from=&to=`: Markdown grouped by project → activity, with journal entries in date order, for 1:1 preparation.

---

## 6. Visual design

Match the mockup. Tokens:

| token | value |
|---|---|
| page background | `#F6F6F3` |
| surface | `#FFFFFF`, border `#E3E3DE`, radius 12–14px |
| text / muted | `#16181D` / `#4A505C` |
| planned | chip `#EEF0F3`/`#454C5A`, bar `#8B94A5` |
| active | chip `#E6EDFF`/`#1D43BF`, bar `#2F5BEA` |
| blocked | chip `#FFEFDD`/`#8F4500`, bar `#E07A12` |
| done | chip `#E2F3EA`/`#16603E`, bar `#1F8A5B` |
| scope creep | chip `#FCE7F0`/`#9D1F5F` |
| journal kinds | update `#16181D`, decision `#5B3AB8`, blocker `#C2410C`, meeting `#155E68`, scope_ack `#9D1F5F` |
| today | `#C2410C` |
| fonts | system sans; system monospace for numbers and dates |

Accessibility: real `<button>`, `<a>`, `<label>`+`<input>`; touch targets ≥ 44px; text contrast ≥ 4.5:1; `aria-pressed` on toggle buttons; `aria-expanded` on project rows.

---

## 7. HTTP surface (sketch)

Pages return full HTML. When `HX-Request` is set, return the fragment being swapped (timeline, panel, or both via `hx-swap-oob`). Every mutation also works without JS (POST then redirect back to the referring dashboard URL).

```
GET    /                                   dashboard
GET    /projects                           projects page
POST   /projects                           create
POST   /projects/{id}                      update (rename/archive)
POST   /projects/{id}/delete
POST   /activities                         create
POST   /activities/{id}                    update fields / status
POST   /activities/{id}/delete
POST   /activities/{id}/subtasks           add
POST   /subtasks/{id}/toggle
POST   /subtasks/{id}                      edit
POST   /subtasks/{id}/delete
POST   /activities/{id}/journal            add
POST   /journal/{id}                       edit
POST   /journal/{id}/delete
POST   /activities/{id}/scope-ack
GET    /export/journal.md
GET    /static/*                           embedded assets
```

---

## 8. Operations

- Flags: `-addr` (default `127.0.0.1:8080`), `-db` (default `$XDG_DATA_HOME/workline/workline.db`, falling back to `~/.local/share/workline/workline.db`).
- SQLite pragmas: `foreign_keys=ON`, `journal_mode=WAL`, `busy_timeout=5000`.
- Migrations: numbered `.sql` files embedded in the binary, tracked with `PRAGMA user_version`, applied at startup in a transaction.
- `workline backup <path>` subcommand using `VACUUM INTO`.
- `workline seed` subcommand loading the mockup's sample data (dev only).

---

## 9. Milestones

1. **Domain + storage.** Schema and migrations; `internal/domain` with §4 fully unit-tested (fixed clock); store layer with tests on a temp DB; seed command.
2. **CRUD.** Handlers for projects, activities, subtasks, journal, scope-ack; plain HTML forms with PRG; handler tests with `httptest`.
3. **Dashboard timeline.** Periods, scales, filters in the URL, project grouping and collapse, activity burndown rows, tiles, legend.
4. **Detail panel + htmx.** Panel, burndown chart, scope section, acknowledge; htmx partial swaps so selecting, toggling and logging don't reload the page.
5. **Polish.** Journal Markdown export, backup command, keyboard shortcuts (`n` new activity, `/` focus filter, `Esc` close panel), narrow-screen layout.

Each milestone ends with `go vet ./...` and `go test ./...` green.

---

## 10. Open questions

- Should acknowledging silence the signal permanently, rather than re-arming on further growth? (v1: re-arm.)
- Are activities ever moved between projects? (v1: yes, through edit.)
- Should "done" activities set `closed_on` on all their open subtasks? (v1: no; progress shows 100% only if there are no subtasks.)

### Decisions (Milestone 1, 2026-11-18)
1. **Toolchain.** Module `workline`, Go 1.27 (`/usr/local/go`, go1.27.4).
2. **Burndown bins** start at `start_date`: bin *i* = `[start + i·step, start + (i+1)·step − 1]`; the last bin is clipped to `min(due, today)`; empty if `start > today`.
3. **Global max** for bar heights is taken over the weekly series of the *visible* activities (after filters), in every view.
4. **Project progress** = closed / total over subtasks of visible activities, exactly as §4.2 (no fallback). A project is not meant to exist without at least one task; if total is still 0, progress is 0.
5. **Deletions after an acknowledgement**: keep the §4.4 formula; clamp `added` and `growth%` at ≥ 0.
6. **Latest ack** = highest ScopeAck id, not latest date.
7. **Acknowledge on an unflagged activity** is refused (`ErrNotFlagged`); no journal entry is written.
8. **Column load**: both `n` and `open` count visible activities that overlap the column and had started by `min(columnEnd, today)`.
9. **Weeks** start on Monday; gridlines only on Mondays inside the selected quarter.
10. **Subtask dates**: `closed_on ≥ created_on` (enforced in Go and by a DB CHECK). `created_on` may precede the activity's `start_date`. Future `closed_on` is not blocked.
11. **`Activity created`** journal entry: kind `update`, dated today. "+ New project…" creates the project inside the same transaction. Project names are unique, case-insensitive.
12. **Seed data** comes from `baseline.json` (mockup baseline, as of 2026-11-18), whose `expected` block is a golden test for §4.
13. **Done activities** do not auto-close subtasks (v1).
14. **Rounding**: integer half-up everywhere (e.g. 5/8 → 63, 2/3 → 67).
