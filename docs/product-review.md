# Product review — Workline as a 1:1 and roadmap tool

Written 2026-11-20 from a PO / business analyst point of view, after v0.10.0.

**Persona.** A tech lead who journals tasks and decisions and, at every weekly
1:1, presents what happened since last week and what new was added. They also
present the roadmap and how things progress in Q1–Q4. In the 1:1 they share
the screen; afterwards they email a Markdown summary.

**Verdict.** Workline is strong at *capture* and weak at *presentation*. The
recent milestones (Vim keys, statusline, search history, completion) made
input faster; little has gone into what the manager actually sees.

## What already serves the persona

- Low-friction capture: quick-add bar, `/t /d /b` prefixes, Unassigned inbox,
  tasks in the journal. A journal nobody fills is worthless, so this matters most.
- `requested_by`: who asked for what, including work outside Jira.
- Scope creep with acknowledgement: a ready-made 1:1 talking point.
- Closed tasks sit at their close date (item 109): "what happened" is right.
- The journal page and the Markdown export are the right starting points.

## Gaps, by job

### "What happened since last week"
- The range is two calendar weeks, not "since my last 1:1".
- No narrative: the feed and export are a raw log by project. A manager wants
  Done · Progress (Δ%, not just %) · Decisions · Blockers · New · Needs from you.
- No curation: the 5 entries that matter can't be marked among 60.

### "What new was added"
- New activities (and who asked) are not surfaced; only an "Activity created"
  line in the feed.
- No status history: "moved to blocked on Tuesday", "blocked for 9 days" can't be said.
- Due-date changes are not recorded, so slippage is invisible.
- Cheapest fix: log status and due-date changes as automatic journal entries.

### Roadmap by quarter
- Plan vs actual: keep the original due date (or a quarter-start snapshot) and show slippage.
- Health: on track / at risk / off track from time elapsed vs tasks closed plus
  the scope flag, with a manual override. Burndown bars are too dense for a director.
- Quarter review: committed at start, delivered, slipped, added mid-quarter, dropped.
- Goals / themes: tie activities to quarterly objectives (outcomes, not a list).
- A one-line status headline per activity: the sentence the manager reads.

### Decisions and blockers
- Blockers have no open/resolved state or "waiting on whom".
- Decisions have no structure (context, who decided) and no per-quarter view
  beyond `k:d` search.
- No `ask` kind for "needs from you", the section managers value most.

## Build order

1. **1:1 report** (page + Markdown) with curation by star. — *Milestone 14.*
2. Automatic history entries for status and due-date changes.
3. Resolvable blockers and an `ask` kind.
4. Original due date and a health indicator on the timeline.
5. Quarter review page (needs 2 and 4).
6. Goals / themes and a status headline per activity.

Pause further keyboard / Vim work until 1–3 exist. All of it fits the
local-first, stdlib-only approach.

## Decided (2026-11-20)

- Milestone 14 builds the 1:1 report and the star (curation).
- The report's range is picked by hand (From/To); no "1:1 held" record yet.
- Curation is a stored star on entries, plus a "highlights only" switch.
- Report sections: Highlights, Done, Progress, New, Decisions, Blockers.
- Delivery: the page for screen share, Markdown readable as plain text for email.
