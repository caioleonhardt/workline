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
| priority | enum, nullable | `high`, `medium`, `low`; null = no priority |
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
| `status` | repeatable: `planned`, `active`, `blocked`, `done` | `planned`, `active`, `blocked` (done hidden) |
| `creep` | `1` or empty | off |
| `sel` | activity id | none |

- Changing `period` clears `month`.
- Status pills are multi-select toggles. Done activities are hidden unless the `done` pill is on; then they are shown with the others, sorted last.
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
  - Track: a thin span line from start to due; burndown bins in the status colour; a dashed box from today to due (not for `done`); completion dots (one per day on which subtasks were closed; hovering or focusing one lists them); `P%` at the right end.
  - Clicking the row or bar selects the activity (`sel=`) and opens the detail panel.
- **Order.** Activities within a project: not-done before done; then priority `high`, `medium`, `low`, none; then due date; then id. Projects: by the best priority among their shown not-done activities (projects with only done activities last); then name.
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

- Flags: `-addr` (default `127.0.0.1:8080`), `-port` (overrides the port in `-addr`, e.g. `-port 8081`), `-db` (default `$XDG_DATA_HOME/workline/workline.db`, falling back to `~/.local/share/workline/workline.db`).
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
6. **Journal workspace.** Journal list below the timeline and a fixed quick-add bar (§10 items 73–76).

Each milestone ends with `go vet ./...` and `go test ./...` green.

---

## 10. Open questions

- Should acknowledging silence the signal permanently, rather than re-arming on further growth? (v1: re-arm.)
- Are activities ever moved between projects? (v1: yes, through edit.)
- Should "done" activities set `closed_on` on all their open subtasks? (v1: no; progress shows 100% only if there are no subtasks.)

### Decisions (Milestone 1, 2026-11-18)
1. **Toolchain.** Module `workline`, Go 1.27 (`/usr/local/go`, go1.27.4).
2. **Burndown bins** start at `start_date`: bin *i* = `[start + i·step, start + (i+1)·step − 1]`; the last bin is clipped to `min(due, today)`; empty if `start > today`.
3. **Global max** for bar heights is taken over the bins actually drawn for the *visible* activities (after filters): weekly in Year view, 2-day in Qn view (superseded by item 48).
4. **Project progress** = closed / total over subtasks of visible activities, exactly as §4.2 (no fallback). A project is not meant to exist without at least one task; if total is still 0, progress is 0.
5. **Deletions after an acknowledgement**: keep the §4.4 formula; clamp `added` and `growth%` at ≥ 0.
6. **Latest ack** = highest ScopeAck id, not latest date.
7. **Acknowledge on an unflagged activity** is refused (`ErrNotFlagged`); no journal entry is written.
8. **Column load**: both `n` and `open` count visible activities that overlap the column and had started by `min(columnEnd, today)`.
9. **Weeks** start on Monday; gridlines only on Mondays inside the selected quarter.
10. **Subtask dates**: `closed_on ≥ created_on` (enforced in Go and by a DB CHECK). `created_on` may precede the activity's `start_date`. A `closed_on` after today is rejected.
11. **`Activity created`** journal entry: kind `update`, dated today. "+ New project…" creates the project inside the same transaction. Project names are unique, case-insensitive.
12. **Seed data** comes from `baseline.json` (mockup baseline, as of 2026-11-18), whose `expected` block is a golden test for §4.
13. **Done activities** do not auto-close subtasks (v1).
14. **Rounding**: integer half-up everywhere (e.g. 5/8 → 63, 2/3 → 67).
15. **Detail burndown**: the bin containing today is measured at today and is not "ahead"; only bins starting after today are ahead.
16. **Journal edits** may change date, text and kind among `update`/`decision`/`blocker`/`meeting`; never to or from `scope_ack`.
17. **Closing an activity** (`done`) is a manual decision, e.g. the manager drops it, and is allowed with open subtasks. "Done this year" therefore needs the date the activity was closed, which the schema does not store yet (to be added in M2).
18. **Creating an activity requires at least one subtask** (M2 create form).
19. **Done activities** are hidden by default and shown, last, only when the `done` status pill is on (pills are multi-select). See §5.1.
20. **Priority**: optional `high`/`medium`/`low` on activities (§3). It orders activities within a project and also orders projects (§5.1). Added by migration `0002` in M2.

### Decisions (Milestone 2, 2026-11-18)
21. **`done_on`** (activity): set to today when status becomes `done`, cleared when it leaves `done`, kept on done→done; not user-editable. Existing and seeded done activities get `done_on = due_date`. "Done this year" counts `done_on`.
22. **Initial subtasks** entered on the create form get `created_on = min(today, start_date)` so they count as kickoff scope; later subtasks default to today.
23. Deleting an activity's last subtask is allowed.
24. Subtask edits change text and `created_on` only; `closed_on` is set only by toggle. Any future path that sets it must use `ValidateAt(today)`.
25. **Delete confirmation** without JS: a two-step "Delete…" → "Confirm delete" button (e.g. `<details>`). No inline scripts.
26. **Routes**: no `POST /projects` (projects are created through the activity form). The new-activity form is inline on the dashboard, shown with `GET /?new=1` (htmx toggles it in M4), as in the mockup.
27. Rejected forms show a plain error page with a Back link; no sticky re-render in M2.
28. Future `created_on` (subtasks) and journal dates are allowed.
29. Mutating forms carry a hidden `next` (the rendering page's path and query); the handler redirects 303 to it only if it is a same-site relative path, else to `/`.
30. `HX-Request` fragments and their tests start in M4; M2 tests cover full pages.
31. The mockup lacks priority, the first-subtask field, delete controls and `/projects`; those use the §6 tokens with plain styling. Its web fonts are not used (system fonts, §6).
32. A 500 shows a plain "Something went wrong." page; details go to the server log.
33. Activities of archived projects are hidden everywhere on the dashboard, including via `sel=`; unarchive from `/projects` to see them.
34. Journal lists are newest first by date, then id; future-dated entries sort to the top.

### Decisions (Milestone 3, 2026-11-18)
35. **Done this year** counts `done_on` in the selected `year`.
36. **Open subtasks now** excludes done activities (a dropped activity's leftovers are not open work).
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
52. **Timeline dots mark subtask completions, not journal entries.** One dot per activity per day on which at least one subtask was closed. Hovering or keyboard-focusing the dot shows a small popup listing that day's closed subtasks (text and close time when known). Journal entries no longer appear on the timeline; they stay in the panel. The legend says "Subtasks completed".
53. **Subtask close time**: subtasks also record `closed_at` (local `YYYY-MM-DD HH:MM`), stamped from the clock when toggled closed and cleared on reopen; never editable. Older and seeded subtasks have none. Journal `logged_at` (item 49) stays.
54. All stored times are local wall-clock time; no UTC conversion.
55. Completion-dot popups in the first timeline rows open downward so they are not clipped; others open upward and scroll past 150px.
56. A subtask keeps only its latest close; reopening and closing again moves its dot (no history).
57. `serve -port N` sets the port; the "listening" log line is written only after the socket is bound.

### Decisions (Milestone 4, 2026-11-18)
58. **htmx 2.0.11**, vendored at `internal/web/static/htmx.min.js` (from the npm registry tarball, integrity `sha512-Thx/WtpeOQqSrqBCw/A1cwGJGg4UrVa3+sW0GmrM3p4gJgO89ecH4qtbnyzDDWFvBTqjnIMCgELTNt636dtamA==`; file sha256 `d6fdc75f204e6bdefa99b69bf1e6d4ac69b8a364f77929f45c13476b4000f717`). Config: `allowEval:false`, `selfRequestsOnly:true`, `historyCacheSize:0`.
59. **One swap region**: the dashboard lives in `#dash` with `hx-boost`; HX requests get the `#dash` fragment, others the full page or 303. `Vary: HX-Request`; history-restore requests get the full page. Mutations answer HX with the fragment plus `HX-Push-Url`.
60. Auto-submitting selects and status; subtasks are checkboxes. Apply/Save buttons show only without JS (`html.js`).
61. Accepted: a swap resets the timeline's horizontal scroll and drops unsent panel text.
62. Panel burndown heights are relative to the activity's total subtasks (as in the mockup).
63. **HX validation errors** (422) show in a small error bar above the dashboard (`HX-Retarget`/`HX-Reswap`), leaving the dashboard as it was. Without JS, item 27's error page applies.
64. With nothing selected, the panel shows "Select an activity to see its burndown, scope, subtasks and journal." 
65. A period shows only activities whose start–due range overlaps it (open activities do not carry past their due date). Projects with no visible activity are hidden.

### Decisions (Milestone 5, 2026-11-19)
66. **Journal export** `GET /export/journal.md?project=&from=&to=`: all non-archived projects (an archived one only when named), entries whose own `date` is in `from..to` inclusive, status/creep filters not applied; projects by name, activities by start then id, entries oldest first (date, `logged_at` with untimed first, id). Downloaded as `workline-journal-YYYY-MM-DD.md`. The header's "Export journal" link carries the dashboard's project and period/month range.
67. **Backup** `workline backup [-db path] <dest>` uses `VACUUM INTO`, refuses an existing destination or missing parent directory, writes mode 0600, and opens the source **without applying migrations** (the source is never changed).
68. **Shortcuts**: `n` new activity, `/` focuses the Project select, `Esc` cancels an open new-activity form, else closes the panel. Ignored in form fields and with Ctrl/Alt/Meta. A hint line shows when JS is on.
69. The error bar's Dismiss is handled in `app.js`; the `/errors/clear` route is removed. 422s are not logged.
70. **Narrow screens**: the label column is sticky; it is ~160px wide on phones. Panel stat boxes wrap; all controls ≥ 44px.
71. Timeline header grows to 88px so the Today chip has its own strip; subtask rows are one line (one date: closed, else opened; both in the tooltip and in Edit).
72. **Target screen is a laptop.** Narrow-screen support (item 70) is best-effort; phone-specific polish is out of scope. Export filenames use today's date; a backup keeps its source's schema version.

### Decisions (Milestone 6, journal workspace, 2026-11-19)
73. **Journal list moves out of the panel** to a full-width section below the timeline. With nothing selected it is a newest-first feed of entries from all activities visible under the current filters, each tagged with its project and activity (a link that selects it). With an activity selected it shows only that activity's entries. Edit/delete per entry stay (scope_ack read-only). The panel keeps status, burndown, scope and subtasks.
74. **Quick-add bar**: fixed to the bottom of the viewport, shown only when an activity is selected, and displays that activity's name (and project) so it is clear where the entry goes. Fields: kind (default Update) and text; Enter logs it with date = today and `logged_at` stamped. Backfilling another date is done by editing the entry in the list.
75. The bar lives outside the htmx swap region so typed text survives swaps; its target updates when the selection changes. The page reserves bottom space so the bar never covers content. Without JS it is a normal form posting with `next`.
76. Shortcut `j` focuses the bar's text field (when shown); `Esc` from the field blurs it.
