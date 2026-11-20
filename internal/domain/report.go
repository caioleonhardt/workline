package domain

import (
	"sort"
	"strings"
)

// ProgressAt is an activity's completion percentage on day t: tasks closed by
// t over tasks opened by t; without tasks by then, 100 once done by t, else 0.
func ProgressAt(a Activity, tasks []Task, t Date) int {
	scope, closed := 0, 0
	for _, s := range tasks {
		if s.CreatedOn.After(t) {
			continue
		}
		scope++
		if c := s.ClosedOn(); !c.IsZero() && !c.After(t) {
			closed++
		}
	}
	if scope == 0 {
		if !a.DoneOn.IsZero() && !a.DoneOn.After(t) {
			return 100
		}
		return 0
	}
	return percent(closed, scope)
}

// ReportInput is what the 1:1 report is built from (SPEC §10 item 112). The
// caller leaves out what the report must not show (archived projects, other
// projects, Unassigned work) from all of Activities, Tasks and Entries.
type ReportInput struct {
	From, To    Date
	Projects    []Project
	Activities  []Activity
	Tasks       []Task         // every task of Activities, and Unassigned ones
	Entries     []JournalEntry // those whose log date is in From..To, oldest first
	OnlyStarred bool
}

// ReportItem is one journal entry with where it belongs; Activity is empty for Unassigned.
type ReportItem struct {
	Project, Activity string
	Entry             JournalEntry
}

// ReportTasks are an activity's tasks in a section, in the journal's order.
type ReportTasks struct {
	Project, Activity string
	Tasks             []JournalEntry
}

// ReportActivity is an activity with its project's name.
type ReportActivity struct {
	Project  string
	Activity Activity
}

// ReportProgress is an activity's progress over the range: on the day before
// From, on To, and its open tasks on To.
type ReportProgress struct {
	ReportActivity
	Before, After, Open int
}

// Report is the 1:1 report: each section in display order, empty when it has nothing.
type Report struct {
	From, To   Date
	Highlights []ReportItem
	Closed     []ReportTasks // Done: tasks closed in the range
	Finished   []ReportActivity
	Progress   []ReportProgress
	Created    []ReportActivity // New: activities created in the range
	Opened     []ReportTasks    // New: tasks opened in the range
	Acks       []ReportItem
	Decisions  []ReportItem
	BlockedNow []ReportActivity
	Blockers   []ReportItem
}

// BuildReport builds the 1:1 report. Activities are listed by project name,
// then start, then id, with Unassigned work last.
func BuildReport(in ReportInput) Report {
	r := Report{From: in.From, To: in.To}
	projectName := map[int64]string{}
	for _, p := range in.Projects {
		projectName[p.ID] = p.Name
	}
	acts := append([]Activity(nil), in.Activities...)
	sort.SliceStable(acts, func(i, j int) bool {
		a, b := acts[i], acts[j]
		if pa, pb := strings.ToLower(projectName[a.ProjectID]), strings.ToLower(projectName[b.ProjectID]); pa != pb {
			return pa < pb
		}
		if a.Start != b.Start {
			return a.Start.Before(b.Start)
		}
		return a.ID < b.ID
	})
	rank := map[int64]int{0: len(acts)} // Unassigned last
	actOf := map[int64]Activity{}
	for i, a := range acts {
		rank[a.ID], actOf[a.ID] = i, a
	}
	item := func(e JournalEntry) ReportItem {
		a := actOf[e.ActivityID]
		return ReportItem{Project: projectName[a.ProjectID], Activity: a.Name, Entry: e}
	}
	inRange := func(d Date) bool { return !d.Before(in.From) && !d.After(in.To) }

	for _, e := range in.Entries {
		if e.Starred {
			r.Highlights = append(r.Highlights, item(e))
		}
	}
	if in.OnlyStarred {
		return r
	}

	closed := map[int64][]JournalEntry{}
	for _, e := range in.Entries {
		switch {
		case e.Kind == KindTask && e.ClosedAt != "":
			closed[e.ActivityID] = append(closed[e.ActivityID], e)
		case e.Kind == KindScopeAck:
			r.Acks = append(r.Acks, item(e))
		case e.Kind == KindDecision:
			r.Decisions = append(r.Decisions, item(e))
		case e.Kind == KindBlocker:
			r.Blockers = append(r.Blockers, item(e))
		}
	}
	opened := map[int64][]JournalEntry{}
	tasksOf := map[int64][]Task{}
	for _, t := range in.Tasks {
		tasksOf[t.ActivityID] = append(tasksOf[t.ActivityID], t)
		if inRange(t.CreatedOn) {
			opened[t.ActivityID] = append(opened[t.ActivityID], JournalEntry{
				ID: t.ID, ActivityID: t.ActivityID, Date: t.CreatedOn, Kind: KindTask, Text: t.Text, ClosedAt: t.ClosedAt, Priority: t.Priority,
			})
		}
	}
	r.Closed = groupTasks(closed, rank, actOf, projectName)
	r.Opened = groupTasks(opened, rank, actOf, projectName)
	for _, ts := range r.Opened {
		sort.SliceStable(ts.Tasks, func(i, j int) bool {
			if ts.Tasks[i].Date != ts.Tasks[j].Date {
				return ts.Tasks[i].Date.Before(ts.Tasks[j].Date)
			}
			return ts.Tasks[i].ID < ts.Tasks[j].ID
		})
	}

	for _, a := range acts {
		ra := ReportActivity{Project: projectName[a.ProjectID], Activity: a}
		finished := a.Status == StatusDone && inRange(a.DoneOn)
		if finished {
			r.Finished = append(r.Finished, ra)
		}
		if inRange(a.CreatedOn) {
			r.Created = append(r.Created, ra)
		}
		if a.Status == StatusBlocked {
			r.BlockedNow = append(r.BlockedNow, ra)
		}
		if (finished || a.Status == StatusActive || a.Status == StatusBlocked) && !a.Start.After(in.To) {
			ts := tasksOf[a.ID]
			r.Progress = append(r.Progress, ReportProgress{ReportActivity: ra,
				Before: ProgressAt(a, ts, in.From.AddDays(-1)), After: ProgressAt(a, ts, in.To), Open: OpenAt(ts, in.To)})
		}
	}
	return r
}

func groupTasks(by map[int64][]JournalEntry, rank map[int64]int, actOf map[int64]Activity, projectName map[int64]string) []ReportTasks {
	ids := make([]int64, 0, len(by))
	for id := range by {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return rank[ids[i]] < rank[ids[j]] })
	var out []ReportTasks
	for _, id := range ids {
		a := actOf[id]
		out = append(out, ReportTasks{Project: projectName[a.ProjectID], Activity: a.Name, Tasks: by[id]})
	}
	return out
}
