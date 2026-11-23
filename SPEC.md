# Workline — Specification (v1)

A local-first tracker for a tech lead's activities: everything you are asked to do, not only what lives in Jira. Activities are grouped by **project**, broken into **tasks**, documented in a **journal**, and shown on a yearly timeline as **burndowns** (open tasks over time) with a **scope-creep** signal.

The visual reference is the mockup in `docs/mockup.png` (exported from the design canvas). When this spec and the mockup disagree, this spec wins.

---

## 1. Goals and non-goals

**Goals (v1)**
- Single user, single machine, single binary. Works offline.
- Track projects → activities → tasks, with a dated journal per activity.
- Year and quarter timeline dashboards with filters.
- Burndown and scope-creep computed from dated events, never entered by hand.

**Non-goals (v1)**
- Authentication, multiple users, sharing, sync, hosting.
- Jira or calendar integrations (possible later: import an epic's issues as tasks).
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
| priority | enum, nullable | `high`, `medium`, `low`; null = no priority |
| start_date | date | required |
| due_date | date | required, ≥ start_date |
| created_at | timestamp | |

On creation, status defaults to `planned` if `start_date > today`, else `active`.

### JournalEntry
| field | type | notes |
|---|---|---|
| id | int | monotonically increasing, never reused |
| activity_id | int, nullable | FK. null = Unassigned (§10 item 91); `scope_ack` entries always have one |
| date | date | defaults to today; editable. For a task, the day it was opened |
| kind | enum | `update`, `decision`, `blocker`, `meeting`, `task`, `scope_ack` |
| text | text | required |
| logged_at | datetime, nullable | local time it was logged (§10 item 49) |
| closed_at | datetime, nullable | tasks only. null = open. Closing sets the local time (§10 items 53, 108); reopening sets null |
| priority | enum | tasks only: `high`, `medium`, `low`, or empty for none (§10 item 92) |

`scope_ack` entries are created only by the Acknowledge action (§5.4), never from the journal form.

### Task
A **task** is a journal entry of kind `task` (§10 item 84). In the rules below, a task's `created_on` is its entry's `date` and its `closed_on` is the day of its `closed_at` (null while open).

### ScopeAck
| field | type | notes |
|---|---|---|
| id | int | |
| activity_id | int | FK |
| date | date | |
| baseline_count | int | number of tasks at acknowledgement |
| last_task_id | int | highest task id at acknowledgement (0 if none) |

Only the most recent ScopeAck per activity matters for computation; older ones are history.

Deleting a project or activity cascades. Deletions require confirmation in the UI.

---

## 4. Derived values (pure domain logic)

All functions below live in `internal/domain`, take plain structs and a `today` date, and have no I/O.

### 4.1 Open and scope at a date
- `openAt(a, t)` = tasks with `created_on ≤ t` and (`closed_on` is null or `closed_on > t`).
- `scopeAt(a, t)` = tasks with `created_on ≤ t`.

### 4.2 Progress
- With tasks: `round(closed / total × 100)`.
- Without tasks: `100` if status is `done`, else `0`.

Project progress = closed / total over all tasks of its visible activities.

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
    newer    = tasks with id > ack.last_task_id
else:
    newer    = tasks with created_on > start_date + KickoffGrace
    baseline = total - len(newer)

added    = total - baseline
growth%  = baseline > 0 ? round(added / baseline × 100) : 0
add30    = newer with created_on > today - Window
close30  = all tasks with closed_on > today - Window

growthFlag    = status != done && baseline > 0 && growth% >= GrowthThreshold
outpacingFlag = status != done && add30 > close30
flagged       = growthFlag || outpacingFlag
```

Reason texts (shown in the detail panel):
- growth: `Grew from {baseline} to {total} tasks since kickoff (+{growth}%).`, or `…since the acknowledgement on {date}…` when acknowledged.
- outpacing: `In the last 30 days {add30} added vs {close30} closed: work is arriving faster than it closes.`

### 4.5 Period columns and load
- **Year / Quarters**: 4 columns Q1–Q4. **Year / Months**: 12 columns. **Qn**: 3 month columns plus weekly gridlines labelled with Monday's day of month.
- Column load label:
  - Column fully in the future: `{n} planned` (visible activities overlapping it).
  - Otherwise: `{open} open · {n} act.`, where `open` = Σ `openAt(a, min(columnEnd, today))` over visible activities that had started by then.
- The column containing today is tinted; the column(s) overlapping a selected month are tinted differently.

---

## 5. Features

### 5.0 Journal page (`GET /`)
The home page, for the present rather than the year (§10 item 95): the last two weeks (Monday to Sunday), ‹ › to move a week, tiles, one burndown row for all tasks with completion dots, open tasks by priority, and the journal entries dated in those weeks. No detail panel.

### 5.1 Dashboard (`GET /timeline`)
Header with the app name and period eyebrow (e.g. `2026 · ACTIVITY TIMELINE` or `Q4 2026 · OCT–DEC`) and a **New activity** button.

Controls, all reflected in the **URL query string** so any view is bookmarkable:

| param | values | default |
|---|---|---|
| `year` | e.g. `2026` | current year |
| `period` | `year`, `q1`…`q4` | `year` |
| `scale` | `quarter`, `month` (Year only) | `quarter` |
| `project` | project id or empty | all |
| `month` | `1`–`12` or empty; limited to the quarter's months in Qn | all |
| `status` | repeatable: `planned`, `active`, `blocked`, `done` | `planned`, `active`, `blocked` (done hidden) |
| `creep` | `1` or empty | off |
| `sel` | activity id | none |

- Changing `period` clears `month`.
- Status pills are multi-select toggles. Done activities are hidden unless the `done` pill is on; then they are shown with the others, sorted last.
- Status pills show counts (after the period/project/month filters, before the status filter).
- The scope-creep toggle shows the number of flagged activities in scope.
- "Clear filters" appears when any filter is active.

**Summary tiles** (respect the project filter only): Active, Blocked, Planned, Done this year, Open tasks now, Scope creep flags.

**Timeline**
- Left column (300px): labels. Right: the period track.
- Overlays on the track: gridlines, a shaded band for the selected month, and a **Today** line with a label.
- Grouped by project. **Project row = one line**: chevron, name, `· N activities`, `· P% done`, and `· K scope flags` if any. No burndown on project rows. Clicking toggles collapse (state kept in a cookie or `localStorage`).
- **Activity row**:
  - Left: name, status chip, a scope chip (`Scope +N`) when flagged, `open / total`, then dates and requester.
  - Track: a thin span line from start to due; burndown bins in the status colour; a dashed box from today to due (not for `done`); completion dots (one per day on which tasks were closed; hovering or focusing one lists them); `P%` at the right end.
  - Clicking the row or bar selects the activity (`sel=`) and opens the detail panel.
- **Order.** Activities within a project: not-done before done; then priority `high`, `medium`, `low`, none; then due date; then id. Projects: by the best priority among their shown not-done activities (projects with only done activities last); then name.
- Empty state: "No activities match these filters."
- Legend below the timeline.

### 5.2 Detail panel
Shown beside the timeline (below it on narrow screens) when `sel` is set.
- Project name (eyebrow), activity name, requested by, dates.
- Status select (saves on change), progress %, `x of y tasks done`.
- **Burndown** (weekly): scope layer, open layer, dashed future weeks, today marker. Caption `peak N open · weekly`.
- **Scope**: three stats, then the warning if flagged.
  - The stats are Baseline ("At kickoff", or "Baseline (ack {date})"), Now with growth %, and Last 30 days `+add30 / −close30`.
  - When flagged, a warning shows the reason text and the **Acknowledge and log to journal** button.
  - If acknowledged and not flagged, a confirmation line shows the ack date and baseline.
  - Helper text explains the rule.
- **Tasks**: the activity's `task` entries as a checkbox list (toggle closes or reopens), `opened {date}` / `closed {date}` on the right, edit and delete per task. Tasks are added from the quick-add bar (§10 item 86).
- **Journal**: moved out of the panel to the journal workspace below the timeline (§10 items 73–79).
- Edit activity (name, requested by, dates, project) and delete activity.

### 5.3 Create flows
- **New activity**: project select (with "+ New project…" which reveals a name field), name, requested by, start, due. On success, select the new activity. It also logs a journal entry `Activity created`.
- **New project**: through the activity form above, plus a simple projects page (`/projects`) to rename, archive and delete.

### 5.4 Acknowledge scope creep
`POST /activities/{id}/scope-ack`, in one transaction:
1. Insert a ScopeAck with today, the current task count and the max task id.
2. Insert a JournalEntry, kind `scope_ack`, with text
   `Scope change acknowledged: {baseline} → {total} tasks (+{growth}%); last 30 days +{add30} / −{close30}. New baseline: {total}.`
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
| journal kinds | update `#16181D`, decision `#5B3AB8`, blocker `#C2410C`, meeting `#155E68`, scope_ack `#9D1F5F`, task `#2D6A3E` |
| today | `#C2410C` |
| fonts | system sans; system monospace for numbers and dates |

Accessibility: real `<button>`, `<a>`, `<label>`+`<input>`; touch targets ≥ 44px; text contrast ≥ 4.5:1; `aria-pressed` on toggle buttons; `aria-expanded` on project rows.

---

## 7. HTTP surface (sketch)

Pages return full HTML. When `HX-Request` is set, return the fragment being swapped (timeline, panel, or both via `hx-swap-oob`). Every mutation also works without JS (POST then redirect back to the referring dashboard URL).

```
GET    /                                   journal page (§10 item 95); any query key but `week` redirects to /timeline
GET    /timeline                           dashboard (timeline)
GET    /projects                           projects page
POST   /projects                           create
POST   /projects/{id}                      update (rename/archive)
POST   /projects/{id}/delete
POST   /activities                         create
POST   /activities/{id}                    update fields / status
POST   /activities/{id}/delete
POST   /tasks/{id}/toggle                  close or reopen a task entry
POST   /journal                            add (hidden `activity`; kind `task` adds a task)
POST   /journal/{id}                       edit (tasks too)
POST   /journal/{id}/delete                (tasks too)
POST   /journal/{id}/star                  star or unstar an entry
POST   /activities/{id}/scope-ack
GET    /export/journal.md
GET    /report                             1:1 report page
GET    /report.md                          1:1 report as Markdown
GET    /static/*                           embedded assets
```

---

## 8. Operations

- Flags: `-addr` (default `127.0.0.1:7357`), `-port` (overrides the port in `-addr`, e.g. `-port 8081`), `-db` (default `$XDG_DATA_HOME/workline/workline.db`, falling back to `~/.local/share/workline/workline.db`).
- SQLite pragmas: `foreign_keys=ON`, `journal_mode=WAL`, `busy_timeout=5000`.
- Migrations: numbered `.sql` files embedded in the binary, tracked with `PRAGMA user_version`, applied at startup in a transaction.
- `workline backup <path>` subcommand using `VACUUM INTO`.
- `workline seed` subcommand loading the mockup's sample data (dev only).
- `workline migrate` subcommand upgrading an existing database to the current schema (§10 item 83).
- `workline version` subcommand printing the release and the schema it needs (§10 item 90).

---

## 9. Milestones

1. **Domain + storage.** Schema and migrations; `internal/domain` with §4 fully unit-tested (fixed clock); store layer with tests on a temp DB; seed command.
2. **CRUD.** Handlers for projects, activities, tasks, journal, scope-ack; plain HTML forms with PRG; handler tests with `httptest`.
3. **Dashboard timeline.** Periods, scales, filters in the URL, project grouping and collapse, activity burndown rows, tiles, legend.
4. **Detail panel + htmx.** Panel, burndown chart, scope section, acknowledge; htmx partial swaps so selecting, toggling and logging don't reload the page.
5. **Polish.** Journal Markdown export, backup command, keyboard shortcuts (`n` new activity, `/` focus filter, `Esc` close panel), narrow-screen layout.
6. **Journal workspace.** Journal list below the timeline and a fixed quick-add bar (§10 items 73–76).
7. **Tasks in the journal.** `workline migrate`; tasks become journal entries of kind `task`; multiline entries (§10 items 83–89).
8. **Version.** `workline version` (§10 item 90).
9. **Unassigned entries.** Log without choosing an activity; file entries later (§10 item 91).
10. **Task priority and bar prefixes.** Tasks get a priority; `/t`, `/d`, `/u`… set the kind from the bar (§10 items 92–93).
11. **Journal page.** `/` shows the last two weeks for 1:1s; the timeline moves to `/timeline` (§10 items 94–95).
12. **Vim keys.** Normal-mode keys move a cursor over entries, search the page and act on the focused entry (§10 items 99–100).
13. **Search with filters.** A query syntax with terms, exclusions and qualifiers, run by the server over the whole journal (§10 items 102–103).
14. **1:1 report.** Starred entries and a report of a chosen range, as a page and as Markdown for email (§10 items 110–114; see `docs/product-review.md`).
15. **Neutral wording.** No "1:1" or "tech lead" in the app: the report is "Report", its Markdown title `Update · {from} to {to}`, its download `workline-report-{to}.md`; README and SPEC follow. History (CHANGELOG, migrations, `docs/product-review.md`) stays as written (§10 item 116).
16. **Keys rename.** `gr` goes to the roadmap (the timeline until 20), the report moves to `gR`, and `gt` is freed (§10 item 117).
17. **Tasks page.** `GET /tasks?page=&state=open|closed|all&project=`, paginated, reached by `gt` and a Tasks nav link; `[`/`]` move between pages.
18. **Journal is home.** `/` shows only journal entries of one day; open tasks, tiles and the burndown strip leave home; `?week=` is dropped.
19. **Day navigation.** `/?date=YYYY-MM-DD`, default today; view, add and edit entries on any day; `[`/`]` previous/next day, `t` today (§10 item 118).
20. **Roadmap.** Replaces the timeline and its burndown: Q1–Q4 columns, one swimlane per project, activities as bars from start to due, coloured by status. The burndown code, tiles and the detail chart go with it (§10 item 119).
21. **Project notes.** A `note` kind (`/n`, `n`) and a nullable `project_id` on entries, so an entry belongs to a project, an activity, or nothing (§10 item 120).
22. **Context command.** `:p name|id` and `:a name|id` set the project or activity new entries are linked to; it persists in a cookie and shows in the statusline (§10 item 121).

Each milestone ends with `go vet ./...` and `go test ./...` green.

---

## 10. Open questions

- Should acknowledging silence the signal permanently, rather than re-arming on further growth? (v1: re-arm.)
- Are activities ever moved between projects? (v1: yes, through edit.)
- Should "done" activities set `closed_on` on all their open tasks? (v1: no; progress shows 100% only if there are no tasks.)

### Decisions (Milestone 1, 2026-11-18)
1. **Toolchain.** Module `workline`, Go 1.27 (`/usr/local/go`, go1.27.4).
2. **Burndown bins** start at `start_date`: bin *i* = `[start + i·step, start + (i+1)·step − 1]`; the last bin is clipped to `min(due, today)`; empty if `start > today`.
3. **Global max** for bar heights is taken over the bins actually drawn for the *visible* activities (after filters): weekly in Year view, 2-day in Qn view (superseded by item 48).
4. **Project progress** = closed / total over tasks of visible activities, exactly as §4.2 (no fallback). A project is not meant to exist without at least one task; if total is still 0, progress is 0.
5. **Deletions after an acknowledgement**: keep the §4.4 formula; clamp `added` and `growth%` at ≥ 0.
6. **Latest ack** = highest ScopeAck id, not latest date.
7. **Acknowledge on an unflagged activity** is refused (`ErrNotFlagged`); no journal entry is written.
8. **Column load**: both `n` and `open` count visible activities that overlap the column and had started by `min(columnEnd, today)`.
9. **Weeks** start on Monday; gridlines only on Mondays inside the selected quarter.
10. **Task dates**: `closed_on ≥ created_on` (enforced in Go and by a DB CHECK). `created_on` may precede the activity's `start_date`. A `closed_on` after today is rejected.
11. **`Activity created`** journal entry: kind `update`, dated today. "+ New project…" creates the project inside the same transaction. Project names are unique, case-insensitive.
12. **Seed data** comes from `baseline.json` (mockup baseline, as of 2026-11-18), whose `expected` block is a golden test for §4.
13. **Done activities** do not auto-close tasks (v1).
14. **Rounding**: integer half-up everywhere (e.g. 5/8 → 63, 2/3 → 67).
15. **Detail burndown**: the bin containing today is measured at today and is not "ahead"; only bins starting after today are ahead.
16. **Journal edits** may change date, text and kind among `update`/`decision`/`blocker`/`meeting`; never to or from `scope_ack`.
17. **Closing an activity** (`done`) is a manual decision, e.g. the manager drops it, and is allowed with open tasks. "Done this year" therefore needs the date the activity was closed, which the schema does not store yet (to be added in M2).
18. **Creating an activity requires at least one task** (M2 create form).
19. **Done activities** are hidden by default and shown, last, only when the `done` status pill is on (pills are multi-select). See §5.1.
20. **Priority**: optional `high`/`medium`/`low` on activities (§3). It orders activities within a project and also orders projects (§5.1). Added by migration `0002` in M2.

### Decisions (Milestone 2, 2026-11-18)
21. **`done_on`** (activity): set to today when status becomes `done`, cleared when it leaves `done`, kept on done→done; not user-editable. Existing and seeded done activities get `done_on = due_date`. "Done this year" counts `done_on`.
22. **Initial tasks** entered on the create form get `created_on = min(today, start_date)` so they count as kickoff scope; later tasks default to today.
23. Deleting an activity's last task is allowed.
24. Task edits change text and `created_on` (the entry's `date`) only; `closed_on` is set only by toggle. Any future path that sets it must use `ValidateAt(today)`.
25. **Delete confirmation** without JS: a two-step "Delete…" → "Confirm delete" button (e.g. `<details>`). No inline scripts.
26. **Routes**: no `POST /projects` (projects are created through the activity form). The new-activity form is inline on the dashboard, shown with `GET /?new=1` (htmx toggles it in M4), as in the mockup.
27. Rejected forms show a plain error page with a Back link; no sticky re-render in M2.
28. Future `created_on` (tasks) and journal dates are allowed.
29. Mutating forms carry a hidden `next` (the rendering page's path and query); the handler redirects 303 to it only if it is a same-site relative path, else to `/`.
30. `HX-Request` fragments and their tests start in M4; M2 tests cover full pages.
31. The mockup lacks priority, the first-task field, delete controls and `/projects`; those use the §6 tokens with plain styling. Its web fonts are not used (system fonts, §6).
32. A 500 shows a plain "Something went wrong." page; details go to the server log.
33. Activities of archived projects are hidden everywhere on the dashboard, including via `sel=`; unarchive from `/projects` to see them.
34. Journal lists are newest first by date, then id; future-dated entries sort to the top.

### Decisions (Milestone 3, 2026-11-18)
35. **Done this year** counts `done_on` in the selected `year`.
36. **Open tasks now** excludes done activities (a dropped activity's leftovers are not open work).
37. **`status` param**: absent = planned, active, blocked; present = exactly the listed values; `status=` = none.
38. Pill and creep counts are taken after period/project/month filters, before status/creep filters.
39. **Project collapse** is client-side: `localStorage` + `web/static/app.js`, toggling `aria-expanded`. Without JS rows stay expanded.
40. Without JS, filter controls are small GET forms; Project and Month need an Apply button until M4.
41. **Year navigation**: `‹ year ›` controls in the header.
42. **Clear filters** resets project, month, status and creep; keeps year, period, scale and `sel`.
43. Activities may span at most 3660 days (rejected by validation).
44. A selected activity hidden by filters still shows its panel (only archived projects hide it).
45. **Burndown bins of a done activity** stop at `min(due, today, done_on)`.
46. The panel stays below the timeline until M4.
47. Activity rows are links with `aria-current` for the selected one.
48. Bar heights are scaled to the tallest bin drawn in the current view (so 2-day bins in Qn can never exceed 100%).
49. **Journal time**: each entry also records `logged_at`, the local date and time (to the minute) when it was logged, stamped automatically from the clock and never editable. The `date` stays editable for backfilling. Lists order by `date`, then `logged_at`, then id, newest first. Entries that predate this column (including seed data) have no time and show none.
50. ~~Timeline journal dots stack per day.~~ Superseded by item 52.
51. Filter controls keep `new=1`, so the create form stays open across filter changes.
52. **Timeline dots mark task completions, not journal entries.** One dot per activity per day on which at least one task was closed. Hovering or keyboard-focusing the dot shows a small popup listing that day's closed tasks (text and close time when known). Journal entries no longer appear on the timeline; they stay in the panel. The legend says "Tasks completed".
53. **Task close time**: tasks also record `closed_at` (local `YYYY-MM-DD HH:MM`), stamped from the clock when toggled closed and cleared on reopen; never editable. Older and seeded tasks have none (until item 108). Journal `logged_at` (item 49) stays.
54. All stored times are local wall-clock time; no UTC conversion.
55. Completion-dot popups in the first timeline rows open downward so they are not clipped; others open upward and scroll past 150px.
56. A task keeps only its latest close; reopening and closing again moves its dot (no history).
57. `serve -port N` sets the port; the "listening" log line is written only after the socket is bound.

### Decisions (Milestone 4, 2026-11-18)
58. **htmx 2.0.11**, vendored at `internal/web/static/htmx.min.js` (from the npm registry tarball, integrity `sha512-Thx/WtpeOQqSrqBCw/A1cwGJGg4UrVa3+sW0GmrM3p4gJgO89ecH4qtbnyzDDWFvBTqjnIMCgELTNt636dtamA==`; file sha256 `d6fdc75f204e6bdefa99b69bf1e6d4ac69b8a364f77929f45c13476b4000f717`). Config: `allowEval:false`, `selfRequestsOnly:true`, `historyCacheSize:0`.
59. **One swap region**: the dashboard lives in `#dash` with `hx-boost`; HX requests get the `#dash` fragment, others the full page or 303. `Vary: HX-Request`; history-restore requests get the full page. Mutations answer HX with the fragment plus `HX-Push-Url`.
60. Auto-submitting selects and status; tasks are checkboxes. Apply/Save buttons show only without JS (`html.js`).
61. Accepted: a swap resets the timeline's horizontal scroll and drops unsent panel text.
62. Panel burndown heights are relative to the activity's total tasks (as in the mockup).
63. **HX validation errors** (422) show in a small error bar above the dashboard (`HX-Retarget`/`HX-Reswap`), leaving the dashboard as it was. Without JS, item 27's error page applies.
64. With nothing selected, the panel shows "Select an activity to see its burndown, scope, tasks and journal." 
65. A period shows only activities whose start–due range overlaps it (open activities do not carry past their due date). Projects with no visible activity are hidden.

### Decisions (Milestone 5, 2026-11-19)
66. **Journal export** `GET /export/journal.md?project=&from=&to=`: all non-archived projects (an archived one only when named), entries whose own `date` is in `from..to` inclusive, status/creep filters not applied; projects by name, activities by start then id, entries oldest first (date, `logged_at` with untimed first, id). Downloaded as `workline-journal-YYYY-MM-DD.md`. The header's "Export journal" link carries the dashboard's project and period/month range.
67. **Backup** `workline backup [-db path] <dest>` uses `VACUUM INTO`, refuses an existing destination or missing parent directory, writes mode 0600, and opens the source **without applying migrations** (the source is never changed).
68. **Shortcuts**: `n` new activity, `/` focuses the Project select, `Esc` cancels an open new-activity form, else closes the panel. Ignored in form fields and with Ctrl/Alt/Meta. A hint line shows when JS is on.
69. The error bar's Dismiss is handled in `app.js`; the `/errors/clear` route is removed. 422s are not logged.
70. **Narrow screens**: the label column is sticky; it is ~160px wide on phones. Panel stat boxes wrap; all controls ≥ 44px.
71. Timeline header grows to 88px so the Today chip has its own strip; task rows are one line (one date: closed, else opened; both in the tooltip and in Edit).
72. **Target screen is a laptop.** Narrow-screen support (item 70) is best-effort; phone-specific polish is out of scope. Export filenames use today's date; a backup keeps its source's schema version.

### Decisions (Milestone 6, journal workspace, 2026-11-19)
73. **Journal list moves out of the panel** to the timeline's column, directly below the timeline and the same width as it (the panel stays beside both). With nothing selected it is a newest-first feed of entries from all activities visible under the current filters, each tagged with its project and activity (a link that selects it). With an activity selected it shows only that activity's entries. Edit/delete per entry stay (scope_ack read-only). The panel keeps status, burndown, scope and tasks.
74. **Quick-add bar**: fixed to the bottom of the viewport, always shown (item 91), and displays that activity's name (and project) so it is clear where the entry goes. Fields: kind (default Update; Task adds a task, item 86) and text; Enter logs it with date = today and `logged_at` stamped, Shift+Enter starts a new line (item 89). Backfilling another date is done by editing the entry in the list.
75. The bar lives outside the htmx swap region so typed text survives swaps; its target updates when the selection changes. The page reserves bottom space so the bar never covers content. Without JS it is a normal form posting with `next`.
76. Shortcut `j` focuses the bar's text field (when shown); `Esc` from the field blurs it.
77. The feed lists all entries (any date) of the visible activities; the period/month only choose activities. 30 entries per page with "Show 30 more" (`feed` param, step 30, max 600), also in the narrowed view.
78. The bar targets the selected activity even when filters hide it. Adding entries uses `POST /journal` (hidden `activity`); `POST /activities/{id}/journal` is removed.
79. Quick-add errors show inside the bar; the draft text and kind survive swaps and selection changes (the label shows the current target). Bar visibility and the bottom padding use CSS `:has()`.
80. **Whole-row selection**: clicking anywhere on an activity row (label or track) selects it. The activity name remains the one real link, stretched over the row with CSS; completion-dot buttons stay above it and keep their popups.
81. The empty feed reads "No journal entries for these filters." The export keeps filtering by entry date (period/month range). The quick-add kind keeps the last choice after a successful add.
82. **Zero-argument commands**: `workline serve` defaults to `127.0.0.1:7357` and the default database (`$XDG_DATA_HOME/workline/workline.db`, else `~/.local/share/workline/workline.db`). `workline backup` with no destination writes `<data dir>/backups/workline-YYYY-MM-DD-HHMM.db` (folder created, mode 0700) and prints the path. Flags only; no environment variables.
83. **Explicit migrations.** A new (empty) database gets every migration on open. An existing database behind the build's schema is refused by `serve` and `seed`, which point to `workline migrate [-db path]`. `migrate` does nothing on a current database; otherwise it first writes the default dated backup (item 82), then applies the pending migrations (each in its own transaction), runs `PRAGMA foreign_key_check` and `integrity_check`, and prints `schema N → M` with each table's row count before → after. On failure it names the backup.

### Decisions (Milestone 7, tasks in the journal, 2026-11-19)
84. **Tasks are journal entries.** Activities are long-running and keep receiving work, so a task is a journal entry of kind `task` rather than its own table. The rules of §4 are unchanged; they read the `task` entries. Burndown, progress, scope creep and completion dots mean what they did.
85. **Closing a task** only marks its entry closed (`closed_at`, item 108); no extra entry is logged. In the feed a closed task is struck through and sits at its close (item 109).
86. **Kind is fixed for tasks.** `task` is chosen when logging (quick-add bar, new-activity form); Edit cannot turn an entry into a task or a task into another kind (422). The panel lists the selected activity's tasks with toggle, edit and delete; it no longer has its own add form.
87. **Migration 0005** rebuilds `journal_entries` with `AUTOINCREMENT`, copies existing entries with their ids, then appends the old subtasks in id order (`date = created_on`, no `logged_at`). `scope_acks.last_subtask_id` becomes `last_task_id`, remapped to the new id of the last task at or before the old value, so the tasks newer than an ack stay the same. The `subtasks` table is dropped.
88. **Activity created** is logged before the initial tasks, so newest-first lists show the tasks above it. The seed keeps the baseline's task ids; its other entries take the ids after them.
89. **Multiline entries.** Journal text (tasks too) is a `<textarea>` in the quick-add bar and in both Edit forms. The bar's field starts one line high and grows with its content (CSS `field-sizing`, capped at 10 lines). The server turns CRLF into LF and trims trailing blank space; lists show line breaks (`white-space: pre-wrap`), and the export indents continuation lines under their bullet.
90. **Version.** `workline version` (also `-version`) prints `workline vX.Y.Z (schema N)`, plus ` revision abc1234[, modified]` when the binary carries a VCS stamp. `X.Y.Z` is a constant in `cmd/workline/main.go` that each release bumps together with `CHANGELOG.md`; a test keeps them equal.
91. **Unassigned entries.** A journal entry (task too) may have no activity (migration 0006 makes `activity_id` nullable and keeps the id counter, so ids are still never reused). The quick-add bar is always shown: it logs to the selected activity, else to **Unassigned**; a × beside a selected target drops the selection. In the feed, Unassigned entries carry an "Unassigned" tag and appear only when neither a project filter nor a selection narrows the feed. Only an Unassigned entry's Edit form has an Activity select (Unassigned, then live activities grouped by project); filing is one-way from the feed, so filed entries do not repeat the activity list in every row. An Edit post without an `activity` field keeps the entry's activity. Unassigned open tasks count in "Open tasks now" when no project filter is set, never on the timeline. The export lists them last under "## Unassigned" when it covers all projects. Moving a task changes its activity's scope figures; an acknowledgement's `last_task_id` compares ids, so a task filed after an ack counts as added only if its id is newer.
92. **Task priority.** A task has a priority like an activity (high, medium, low, or none; migration 0007, other kinds never have one). The bar has a priority select next to the kind, used only when logging a task; task Edit forms (panel and feed) have one too. Tasks show a priority chip in the panel and the feed. The panel lists open tasks first, then by priority, then oldest opened, then id (`domain.SortTasks`); closed tasks follow in the same order.
93. **Bar prefixes.** Text starting with `/t`, `/d`, `/u`, `/b` or `/m`, then a space or line break and some text, logs a task, decision, update, blocker or meeting; `/t1`, `/t2`, `/t3` log a high, medium or low priority task. The prefix overrides the bar's kind and priority selects and is removed from the text. Anything else (`/x foo`, `/tea`, `/d1 …`, a lone `/t`) is logged as typed. Parsed by the server, so it works without JS.
94. **Timeline at `/timeline`.** The dashboard moved from `/` to `/timeline`; its links and forms use the new path. `GET /` with any query key other than the journal page's `week` redirects (302, not a cacheable 308) to `/timeline` with the same query, and a mutation whose `next` is such an address answers with the `/timeline?…` one, so bookmarks and pages loaded before the move keep working. The top navigation reads Journal · Timeline · Projects; these links load whole pages.
95. **Journal page** (`GET /`, `?week=` any day of the latest week shown; default this week; any other key on `/` redirects to `/timeline`, item 94). It shows two weeks, Monday to Sunday: the named week and the one before; ‹ › move by a week and "This week" returns. Contents:
    - **Tiles**: tasks opened in the range, tasks closed in the range, open tasks now, high-priority open tasks.
    - **Strip**: one timeline row for all tasks (Unassigned included), one column per day, daily bins of open tasks up to today scaled to the range's peak, a completion dot centred in each day with closes (the timeline's popups, opening upward), today's line.
    - **Open tasks by priority** (`domain.SortTasks`), each with a toggle, its priority chip and its activity (a full-page link to `/timeline?sel=ID`) or "Unassigned".
    - **Feed**: the entries whose log date (item 109) is in the range, newest first, with the feed's Edit forms (Unassigned entries can be filed). A task closed in the range is in the feed at its close, wherever it was opened.
    - Activities of archived projects are left out of all of it (item 33).
    - **Bar**: on this page the bar has an Activity select (Unassigned first) instead of a fixed target; it sits outside the swapped region, so the choice survives swaps.
    The page's root is `#dash` like the timeline's, so the bar, htmx fragments, errors and `done()` work the same; a mutation whose `next` is the journal page answers with its fragment. Links between the two pages load whole pages.
96. **Floating completion popups.** With scripting on, a completion dot's popup is `position: fixed` and placed by `app.js` beside its dot: above it when there is room, else below, kept inside the window, and moved along on scroll. The timeline's scrolling box can no longer clip it. Without scripting the CSS-only popup of item 54 stays.
97. **Journal keys.** Outside a text field, `t`, `d`, `u`, `b` and `m` focus the journal bar with kind task, decision, update, blocker or meeting chosen; `j` focuses it keeping the kind. Typing `/` outside a field would open the browser's quick find, so on pages without the timeline's filter (the journal page) `/` focuses the bar instead. The `/t`… prefixes of item 93 still work inside the bar.
98. **Entry editor.** Edit (feed entries and panel tasks) opens an inline box: the text first, full width, growing with its content (CSS `field-sizing`); then kind, activity, priority and date as needed; then Delete… (with its confirm step) on the left and Cancel and Save on the right. Delete is a separate form, reached by the confirm button's `form` attribute. No editor library: entries stay plain text. With scripting on, opening the box focuses the text at its end, Ctrl/Cmd+Enter saves, and Esc or Cancel discards the changes and closes it; without it, the Edit summary closes the box and Cancel is hidden.

### Decisions (Milestone 12, Vim keys, 2026-11-19)
99. **Vim keys.** The page is Normal mode; the quick-add bar, the entry editor and other fields are Insert mode (keys there are left alone, as are keys with Ctrl/Alt/Meta). Entry lists are panes: on the journal page, open tasks and the feed; on the timeline, the panel's tasks and the feed. Each entry is focusable (`tabindex="-1"`) and the focused one is the cursor.
    - `j`/`k` next/previous entry in the pane (from nothing: the first pane's first entry); `gg`/`G` first/last; `h`/`l` previous/next pane.
    - `i` opens the focused entry's editor; Esc or Cancel returns the cursor to the entry. `x` closes or reopens a task that has a toggle. `dd` opens the editor at the Delete… confirm step with Confirm focused; nothing is deleted without it. `Enter` follows the entry's activity link.
    - `o` focuses the bar keeping its kind (the `/t`… prefixes of item 93 choose another).
    - `/` opens a search line; Enter moves to the first entry after the cursor whose text contains the query (case-insensitive) and marks all matches; `n`/`N` move to the next/previous match, wrapping, re-read from the page so they survive swaps. Esc in Normal mode clears the marks first.
    - `[`/`]` previous/next week on the journal page; `gj`/`gt`/`gp` go to Journal, Timeline, Projects; on the timeline `O` opens New activity and `f` focuses the Project filter. `Esc` otherwise cancels the new-activity form, else closes the panel (item 68).
    - `?` opens a dialog listing the keys; the pages' hint lines say "Press ? for keys".
    This replaces the keys of items 68, 76 and 97 (`n`, `/` as filter or bar, `j`, `t d u b m`). Without scripting nothing changes. No counts, Visual mode, registers or `:` commands.
100. **Server-side search** over the whole journal is left for later; `/` searches only the entries on the page. (Superseded by item 103.)
101. **Statusline.** With scripting on, a one-line statusline sits at the bottom of the window, below the quick-add bar, as in Neovim with a lualine-style mode block:
    - **Mode** on the left: `NORMAL`; `INSERT` while a field (bar, editor, filter) has focus; `COMMAND` while typing a search.
    - **Command line**: `/` types the search in the statusline after a `/`. Afterwards it shows `/term [i/n]` for the match under the cursor, `search hit BOTTOM, continuing at TOP` (or `TOP … BOTTOM` for `N`) when a jump wraps, and `E486: Pattern not found: term` when nothing matches. Esc clearing the matches clears it.
    - **Pending keys** (`g`, `d`) show on the right while they wait, like `showcmd`.
    - **Position** on the far right: the cursor's list and place in it, e.g. `Journal 3/30`.
    Lists are named by their `data-pane` value (Open tasks, Tasks, Journal). Without scripting there is no statusline. It is not a live region, so mode changes are not announced.

### Decisions (Milestone 13, search with filters, 2026-11-19)
102. **Query syntax.** A query is tokens separated by spaces (outside double quotes), all of which must match (AND); there is no OR. Matching ignores case.
    - `word` — the text contains it; `"two words"` — the text contains the phrase.
    - `p:1`/`p:high`, `p:2`/`p:medium`, `p:3`/`p:low` — the priority (only tasks have one).
    - `k:` — the kind: `t`/`task`, `d`/`decision`, `u`/`update`, `b`/`blocker`, `m`/`meeting`, `ack`/`scope_ack`.
    - `proj:` / `act:` — the project / activity name starts with the value; the value may be quoted (`proj:"Big Thing"`).
    - `is:open` / `is:closed` — an open / closed task; `is:unassigned` — an entry with no activity.
    - `!` before any token negates it (`!draft`, `!k:meeting`).
    A `key:` that is not one of these is plain text, so URLs still search. A bad value for a known key is refused with `E475: Invalid argument: p:9`, an unclosed quote with `E114: Missing quote: "two`, an empty query with `E35: No previous regular expression`. Parsing and matching live in `internal/domain` (`ParseQuery`, `Query.Match`).
103. **Search routes and keys.** Entries of archived projects are left out (item 33); Unassigned entries are in.
    - `GET /search?q=&from=` lists the matching entries newest first with the feed's rows and Edit forms, 30 at a time with "Show 30 more" (`feed`, as in item 77). Its root is `#dash`, with the bar and the statusline, so mutations whose `next` is the search page answer with its fragment. "Clear search" goes back to `from` (a local path, else `/`). A refused query shows its message instead of results. It works without scripting.
    - `GET /search/ids?q=` answers `{"ids":[…]}` with every matching entry's id, or `{"error":"E…"}` with 400. `GET /search/words` answers `{"projects":[…],"activities":[…]}` (live names) for completion.
    - In the search line, `Enter` marks the entries on the page whose id the server returned and jumps as before (`[i/n]`, wrap, `E486`), adding how many match in the whole journal when that is more; with none on the page but some elsewhere it reads `E486: Pattern not found on this page: … (N in the journal, Ctrl+Enter lists them)`. With a search active the marks are fetched again after each swap. `Ctrl+Enter` opens `/search` with the query and the current address as `from`; there the statusline reads `/query [filter] N results` and `Esc` in Normal mode, with no marks, follows "Clear search".
    - `Tab` / `Shift+Tab` complete the last token: keys, the values of `p:`, `k:`, `is:`, and project or activity names (quoted when they contain a space), cycling through the candidates, which the statusline lists with the current one highlighted.
104. **Tasks toggle from the feed.** A task in the journal feed (timeline, journal page, search page) has a close/reopen checkbox form at the start of its row, before the date (other rows keep an empty slot of the same width, so dates line up; the checkbox is named by the task's text), with its `next` the page it is on, so a closed task can be reopened without selecting its activity; `x` on it toggles it. Without scripting the checkbox's form shows its Close/Reopen button.
105. **Search history.** In the search line, `Ctrl+P` or `Up` recalls the previous search and `Ctrl+N` or `Down` the next, back to what was being typed, as in Vim's cmdline (no prefix filtering). Searches run with `Enter` or `Ctrl+Enter` are kept in the browser's local storage, newest last, 50 at most, so the history survives the search page's load; without storage it lasts as long as the page. On the search page its query is the last pattern, so `/` then `Enter` marks it.
106. **Click a journal row to edit.** With scripting on, a click on a feed row, outside its links, controls and editor and without selecting text, opens the entry's editor. The Edit summary is then hidden until it has keyboard focus (Tab still reaches it, `i` still opens it); without scripting it shows as before. The feed's task checkbox is 14px and sits on the row's text baseline.
107. **One line per journal row.** A feed row shows the entry's first line, followed by ` …` when non-blank lines follow; a long line is cut with an ellipsis, so every row is one line high. The whole text is in the entry's editor (a click on the row, item 106). The export keeps the full text. This replaces item 89's line breaks in lists for the feed; the panel's and the journal page's task lists are unchanged. All task checkboxes are 14px.

### Decisions (close time, 2026-11-19)
108. **One close field.** `closed_on` and `closed_at` said the same thing, so migration 0008 drops `closed_on`; `closed_at` alone records a close, and `closed_on` in §4 is its day. Closes recorded without a time (older and seeded tasks) get `00:00`, which the completion popups show. The seed file gives closed tasks `closed_at` at `00:00`. The DB checks that only a task has a `closed_at` and that its day is not before the task's `date`.
109. **Log date.** The journal is a log, and closing a task is an event in it. An entry's log date and time are a closed task's `closed_at`, else the entry's `date` and `logged_at`. Every journal list (feed, journal page, search, export) orders by log date, then log time (untimed entries after timed ones when newest first, before them when oldest first), then id; the journal page and the export pick entries by log date. This amends items 34, 49, 66 and 95. A closed task's row leads with its close date and time and ends with `opened {date}`; the export writes `(closed, opened {date})`. A task is still one row: when it is reopened it goes back to its opening date, and a task opened in a week and closed later is listed under the week it was closed in. The Edit form's Opened field still edits `date`.

### Decisions (Milestone 14, 1:1 report, 2026-11-20)
110. **Starred entries.** Any journal entry, `scope_ack` included, can be starred (migration 0009, `starred` 0/1). `POST /journal/{id}/star` toggles it and answers like a task toggle (303 to `next`, or the page fragment with `HX-Request`). Feed rows (timeline, journal page, search) and the report's highlights have a ☆/★ button with `aria-pressed`; in Normal mode `s` toggles the focused entry's star. Editing an entry keeps its star. `is:starred` matches starred entries.
111. **Activity creation day.** `activities.created_on` (migration 0010) is the local day an activity was created, from the clock; `created_at` stays as it was (UTC, from SQLite) and is not used. Existing activities take the date of their "Activity created" entry, else the day of `created_at`; seeded ones take their start date.
112. **Report page** `GET /report?from=&to=&project=&only=starred`. From and To default to today − 6 and today; From after To is refused. `project` narrows it to one project (Unassigned entries are then left out); archived projects are left out (item 33). Entries are picked by log date (item 109). Sections, each left out when empty, items tagged `Project / Activity` or `Unassigned`:
    - **Highlights**: starred entries in the range, oldest first.
    - **Done**: tasks closed in the range, by activity; then activities whose `done_on` is in the range ("Finished").
    - **Progress**: activities active or blocked now, or finished in the range, that started by To (overdue ones included): progress at From − 1 → at To (§4.2 at a date: closed by then / opened by then; without tasks 100 once done), the change, open tasks at To and the due date.
    - **New**: activities created in the range (item 111) with their requester; tasks opened in the range, by activity; scope acknowledgements in the range.
    - **Decisions** and **Blockers**: entries of that kind in the range; Blockers first lists the activities blocked now.
    `only=starred` keeps Highlights alone. The page has the date form (GET, works without scripting), the sections, **Download .md** and, with scripting, **Copy Markdown**. Its root is `#dash`, with the bar and the statusline. `gr` goes to it; the top navigation reads Journal · Timeline · Report · Projects; the journal page links to the report of its range, up to today. Download .md carries the page's resolved range, so it matches what is shown.
113. **Report Markdown** `GET /report.md` with the same parameters, downloaded as `workline-1on1-{to}.md`. It must read well as plain text in an email: a `#` title, `##` sections, `-` bullets with two-space indented sub-bullets, no tables or bold. The title line is `1:1 update · {from} to {to}`, followed by the project name when narrowed.
114. Progress in the report is by task count, like everywhere else; weighting tasks is out of scope.

### Decisions (milestones 15–22, 2026-11-23)
115. **Alpha.** Before 1.0.0, breaking changes are allowed to keep the design clean, provided `workline migrate` brings existing data to the new model (CLAUDE.md, Alpha).
116. **Neutral wording.** The app does not assume who uses it or for whom. `docs/product-review.md` is kept as a dated note; its persona is not the product's.
117. **Go keys.** `gj` journal, `gt` tasks, `gr` roadmap, `gp` projects, `gR` report.
118. **Day keys.** `[`/`]` previous/next day and `t` today, not `Ctrl+I`/`Ctrl+O`: the browser owns `Ctrl+O`, the key layer leaves Ctrl keys alone (item 99), and in Vim they walk the jump list. Future days are allowed.
119. **Roadmap scope.** Bars by project only: no milestone markers, plan-vs-actual or health indicator for now.
120. **Notes.** `note` is a journal kind like the others. An entry has at most one of `activity_id` and `project_id`; an activity implies its project. Project notes show on the project page and in the feed under the project.
121. **Context.** `:` outside a field opens a command line. `:p` matches a project by id or by case-insensitive name prefix, then substring; several matches are listed and nothing is set; `:a` does the same for live activities. `:p` or `:a` alone clears it. The context is a cookie the server reads as the bar's default link, so it works without scripting. A kind prefix (`/t` …) sets only the kind. An unknown or archived target is an error and the context is kept.
122. **Ctrl+[ is Esc.** Everywhere Esc acts (entry editor, search line, quick-add bar, Normal mode, the keys dialog), `Ctrl+[` does the same, as in Vim. It is the one Ctrl key the key layer takes (amends item 99).
