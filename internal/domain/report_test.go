package domain

import (
	"reflect"
	"testing"
)

func TestProgressAt(t *testing.T) {
	tasks := []Task{sub(t, 1, "2026-01-01", "2026-01-10"), sub(t, 2, "2026-01-01", ""), sub(t, 3, "2026-01-20", "2026-01-25")}
	act := Activity{Status: StatusActive}
	for _, c := range []struct {
		name  string
		a     Activity
		tasks []Task
		at    string
		want  int
	}{
		{"before any task", act, tasks, "2025-12-31", 0},
		{"open only", act, tasks, "2026-01-09", 0},
		{"one of two", act, tasks, "2026-01-10", 50},
		{"scope grew", act, tasks, "2026-01-20", 33},
		{"two of three", act, tasks, "2026-01-25", 67},
		{"no tasks, not done", act, nil, "2026-01-25", 0},
		{"no tasks, done later", Activity{Status: StatusDone, DoneOn: d(t, "2026-02-01")}, nil, "2026-01-31", 0},
		{"no tasks, done by then", Activity{Status: StatusDone, DoneOn: d(t, "2026-02-01")}, nil, "2026-02-01", 100},
	} {
		if got := ProgressAt(c.a, c.tasks, d(t, c.at)); got != c.want {
			t.Errorf("%s: %d, want %d", c.name, got, c.want)
		}
	}
}

func TestBuildReport(t *testing.T) {
	from, to := d(t, "2026-11-14"), d(t, "2026-11-20")
	projects := []Project{{ID: 1, Name: "Zeta"}, {ID: 2, Name: "atlas"}}
	act := func(id, project int64, name string, status Status, start, created, done string) Activity {
		a := Activity{ID: id, ProjectID: project, Name: name, Status: status, Start: d(t, start), Due: d(t, "2026-12-31"), CreatedOn: d(t, created)}
		if done != "" {
			a.DoneOn = d(t, done)
		}
		return a
	}
	mig := act(1, 2, "Migration", StatusActive, "2026-10-01", "2026-10-01", "")
	audit := act(2, 1, "Audit", StatusBlocked, "2026-11-15", "2026-11-15", "")
	old := act(3, 2, "Old API", StatusDone, "2026-09-01", "2026-09-01", "2026-11-18")
	gone := act(4, 2, "Long done", StatusDone, "2026-01-01", "2026-01-01", "2026-03-01")
	later := act(5, 1, "Later", StatusActive, "2026-12-01", "2026-11-19", "")
	twin := act(6, 2, "Twin", StatusActive, "2026-10-01", "2026-10-01", "") // starts with Migration, after it by id
	task := func(id, activity int64, opened, closed string) Task {
		s := sub(t, id, opened, closed)
		s.ActivityID, s.Text = activity, "t"
		return s
	}
	tasks := []Task{
		task(10, 1, "2026-10-01", "2026-10-20"), // closed before
		task(11, 1, "2026-10-01", "2026-11-16"), // closed in range
		task(12, 1, "2026-11-15", ""),           // opened in range
		task(13, 1, "2026-10-01", ""),
		task(14, 2, "2026-11-15", "2026-11-17"), // opened and closed in range
		task(15, 0, "2026-11-19", ""),           // Unassigned
		task(16, 1, "2026-11-14", ""),           // opened earlier in range, listed first
		task(17, 1, "2026-11-15", ""),           // same day as 12, after it by id
	}
	entry := func(id, activity int64, kind Kind, day string, starred bool) JournalEntry {
		return JournalEntry{ID: id, ActivityID: activity, Kind: kind, Date: d(t, day), Text: "e", Starred: starred}
	}
	closedEntry := func(s Task) JournalEntry {
		return JournalEntry{ID: s.ID, ActivityID: s.ActivityID, Kind: KindTask, Date: s.CreatedOn, Text: s.Text, ClosedAt: s.ClosedAt}
	}
	dec := entry(20, 1, KindDecision, "2026-11-15", true)
	blk := entry(21, 2, KindBlocker, "2026-11-16", false)
	ack := entry(22, 1, KindScopeAck, "2026-11-16", false)
	note := entry(23, 0, KindUpdate, "2026-11-19", true)
	entries := []JournalEntry{dec, closedEntry(tasks[1]), blk, ack, closedEntry(tasks[4]), note}
	in := ReportInput{From: from, To: to, Projects: projects, Activities: []Activity{twin, audit, mig, old, gone, later}, Tasks: tasks, Entries: entries}

	r := BuildReport(in)
	names := func(as []ReportActivity) []string {
		var out []string
		for _, a := range as {
			out = append(out, a.Project+"/"+a.Activity.Name)
		}
		return out
	}
	ids := func(items []ReportItem) []int64 {
		var out []int64
		for _, it := range items {
			out = append(out, it.Entry.ID)
		}
		return out
	}
	groups := func(gs []ReportTasks) map[string][]int64 {
		out := map[string][]int64{}
		for _, g := range gs {
			for _, e := range g.Tasks {
				out[g.Project+"/"+g.Activity] = append(out[g.Project+"/"+g.Activity], e.ID)
			}
		}
		return out
	}
	check := func(name string, got, want any) {
		t.Helper()
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s = %v, want %v", name, got, want)
		}
	}
	check("highlights", ids(r.Highlights), []int64{20, 23})
	check("highlight tag", [2]string{r.Highlights[0].Project, r.Highlights[0].Activity}, [2]string{"atlas", "Migration"})
	check("unassigned tag", [2]string{r.Highlights[1].Project, r.Highlights[1].Activity}, [2]string{"", ""})
	check("closed", groups(r.Closed), map[string][]int64{"atlas/Migration": {11}, "Zeta/Audit": {14}})
	check("closed order", r.Closed[0].Activity, "Migration") // atlas before Zeta, ignoring case
	check("finished", names(r.Finished), []string{"atlas/Old API"})
	check("created", names(r.Created), []string{"Zeta/Audit", "Zeta/Later"})
	check("opened", groups(r.Opened), map[string][]int64{"atlas/Migration": {16, 12, 17}, "Zeta/Audit": {14}, "/": {15}})
	check("opened: Unassigned last", r.Opened[len(r.Opened)-1].Activity, "")
	check("acks", ids(r.Acks), []int64{22})
	check("decisions", ids(r.Decisions), []int64{20})
	check("blocked now", names(r.BlockedNow), []string{"Zeta/Audit"})
	check("blockers", ids(r.Blockers), []int64{21})

	// Progress: active, blocked, finished in range; not Long done, not Later (starts after To)
	var prog []string
	for _, p := range r.Progress {
		prog = append(prog, p.Activity.Name)
	}
	check("progress rows", prog, []string{"Old API", "Migration", "Twin", "Audit"}) // atlas by start then id, then Zeta
	// Migration: on 11-13, 1 of 3 closed (33); on 11-20, 2 of 6 (33); 4 open
	check("migration", [3]int{r.Progress[1].Before, r.Progress[1].After, r.Progress[1].Open}, [3]int{33, 33, 4})
	// Old API has no tasks: done on 11-18 → 0 to 100
	check("old api", [2]int{r.Progress[0].Before, r.Progress[0].After}, [2]int{0, 100})
	// Audit started in range: 0 → 100
	check("audit", [3]int{r.Progress[3].Before, r.Progress[3].After, r.Progress[3].Open}, [3]int{0, 100, 0})

	in.OnlyStarred = true
	only := BuildReport(in)
	check("only starred", only, Report{From: from, To: to, Highlights: r.Highlights})
}
