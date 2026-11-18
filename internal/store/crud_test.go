package store

import (
	"errors"
	"testing"

	"workline/internal/domain"
)

func TestProjects(t *testing.T) {
	s := newTestStore(t)
	p, err := s.CreateProject(ctx, "  Alpha ")
	if err != nil || p.Name != "Alpha" {
		t.Fatalf("create: %+v %v", p, err)
	}
	if _, err := s.CreateProject(ctx, "ALPHA"); !errors.Is(err, ErrConflict) {
		t.Errorf("case-insensitive duplicate: %v", err)
	}
	if _, err := s.CreateProject(ctx, "  "); !errors.Is(err, domain.ErrInvalid) {
		t.Errorf("blank: %v", err)
	}
	b, _ := s.CreateProject(ctx, "beta")
	if err := s.RenameProject(ctx, b.ID, "alpha"); !errors.Is(err, ErrConflict) {
		t.Errorf("rename to duplicate: %v", err)
	}
	if err := s.RenameProject(ctx, b.ID, "Beta2"); err != nil {
		t.Fatal(err)
	}
	if err := s.RenameProject(ctx, 999, "x"); !errors.Is(err, ErrNotFound) {
		t.Errorf("rename missing: %v", err)
	}
	if err := s.RenameProject(ctx, b.ID, ""); !errors.Is(err, domain.ErrInvalid) {
		t.Errorf("rename blank: %v", err)
	}
	if err := s.SetProjectArchived(ctx, b.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := s.SetProjectArchived(ctx, 999, true); !errors.Is(err, ErrNotFound) {
		t.Errorf("archive missing: %v", err)
	}
	if ps, _ := s.ListProjects(ctx, false); len(ps) != 1 || ps[0].ID != p.ID {
		t.Errorf("active list: %+v", ps)
	}
	if ps, _ := s.ListProjects(ctx, true); len(ps) != 2 || !ps[1].Archived {
		t.Errorf("full list: %+v", ps)
	}
	got, err := s.GetProject(ctx, b.ID)
	if err != nil || got.Name != "Beta2" || !got.Archived {
		t.Errorf("get: %+v %v", got, err)
	}
	if _, err := s.GetProject(ctx, 999); !errors.Is(err, ErrNotFound) {
		t.Errorf("get missing: %v", err)
	}
	if err := s.DeleteProject(ctx, b.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteProject(ctx, b.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("delete twice: %v", err)
	}
}

func TestCreateActivity(t *testing.T) {
	s := newTestStore(t)
	today := date(t, "2026-11-18")
	p, _ := s.CreateProject(ctx, "P")

	a, err := s.CreateActivity(ctx, NewActivity{ProjectID: p.ID, Name: "A", RequestedBy: "Me",
		Start: date(t, "2026-12-01"), Due: date(t, "2026-12-31"), Subtasks: []string{"one"}}, fixed(today))
	if err != nil {
		t.Fatal(err)
	}
	if a.Status != domain.StatusPlanned || a.ID == 0 {
		t.Errorf("future start should be planned: %+v", a)
	}
	got, err := s.GetActivity(ctx, a.ID)
	if err != nil || got != a {
		t.Errorf("round trip: %+v vs %+v (%v)", got, a, err)
	}
	j, _ := s.ListJournal(ctx, a.ID)
	if len(j) != 1 || j[0].Text != "Activity created" || j[0].Kind != domain.KindUpdate || j[0].Date != today {
		t.Errorf("journal: %+v", j)
	}

	cur, _ := s.CreateActivity(ctx, NewActivity{ProjectID: p.ID, Name: "B", Start: today, Due: today, Subtasks: []string{"one"}}, fixed(today))
	if cur.Status != domain.StatusActive {
		t.Errorf("start today should be active: %+v", cur)
	}

	// new project path creates the project in the same transaction
	n, err := s.CreateActivity(ctx, NewActivity{NewProjectName: "Fresh", Name: "C", Start: today, Due: today, Subtasks: []string{"one"}}, fixed(today))
	if err != nil {
		t.Fatal(err)
	}
	if pr, err := s.GetProject(ctx, n.ProjectID); err != nil || pr.Name != "Fresh" {
		t.Errorf("new project: %+v %v", pr, err)
	}

	// invalid input: neither / both project fields, due < start
	for name, in := range map[string]NewActivity{
		"neither":  {Name: "x", Start: today, Due: today, Subtasks: []string{"x"}},
		"both":     {ProjectID: p.ID, NewProjectName: "y", Name: "x", Start: today, Due: today, Subtasks: []string{"x"}},
		"inverted": {NewProjectName: "Ghost", Name: "x", Start: today, Due: today.AddDays(-1), Subtasks: []string{"x"}},
		"blank":    {NewProjectName: "Ghost", Name: " ", Start: today, Due: today, Subtasks: []string{"x"}},
		"dup name": {NewProjectName: "fresh", Name: "x", Start: today, Due: today, Subtasks: []string{"x"}},
		"bad proj": {ProjectID: 999, Name: "x", Start: today, Due: today, Subtasks: []string{"x"}},
	} {
		if _, err := s.CreateActivity(ctx, in, fixed(today)); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
	if ps, _ := s.ListProjects(ctx, true); len(ps) != 2 {
		t.Errorf("failed creations leaked a project: %+v", ps)
	}
	if _, err := s.CreateActivity(ctx, NewActivity{ProjectID: 999, Name: "x", Start: today, Due: today, Subtasks: []string{"x"}}, fixed(today)); !errors.Is(err, domain.ErrInvalid) {
		t.Errorf("FK failure should be ErrInvalid")
	}
}

func TestCreateActivitySubtasks(t *testing.T) {
	s := newTestStore(t)
	today := date(t, "2026-11-18")
	mk := func(start string, subtasks ...string) (domain.Activity, error) {
		return s.CreateActivity(ctx, NewActivity{NewProjectName: start, Name: "A", Start: date(t, start), Due: date(t, "2027-12-31"), Subtasks: subtasks}, fixed(today))
	}
	// started in the past: subtasks dated at start so they count as kickoff scope
	a, err := mk("2026-06-01", "one", "two")
	if err != nil {
		t.Fatal(err)
	}
	subs, _ := s.ListSubtasks(ctx, a.ID)
	if len(subs) != 2 || subs[0].CreatedOn != date(t, "2026-06-01") || subs[1].Text != "two" {
		t.Errorf("past start: %+v", subs)
	}
	act, _ := s.GetActivity(ctx, a.ID)
	if c := domain.ScopeCreep(act, subs, nil, today); c.Flagged || c.Baseline != 2 {
		t.Errorf("new activity must not be flagged: %+v", c)
	}
	// future start: dated today
	b, err := mk("2026-12-01", "x")
	if err != nil {
		t.Fatal(err)
	}
	if subs, _ := s.ListSubtasks(ctx, b.ID); subs[0].CreatedOn != today {
		t.Errorf("future start: %+v", subs)
	}
	// no subtask or a blank one: nothing is written, not even the project
	for _, subtasks := range [][]string{nil, {" "}, {"ok", ""}} {
		if _, err := mk("2026-07-01", subtasks...); !errors.Is(err, domain.ErrInvalid) {
			t.Errorf("subtasks %q: %v", subtasks, err)
		}
	}
	if ps, _ := s.ListProjects(ctx, true); len(ps) != 2 {
		t.Errorf("failed creations leaked rows: %+v", ps)
	}
	if all, _ := s.ListAllSubtasks(ctx); len(all) != 3 {
		t.Errorf("failed creations leaked subtasks: %+v", all)
	}
}

func TestActivityUpdateStatusDeleteList(t *testing.T) {
	s := newTestStore(t)
	a := mustActivity(t, s, "P", "2026-01-01", "2026-02-01")
	q, _ := s.CreateProject(ctx, "Q")
	a.ProjectID, a.Name, a.RequestedBy = q.ID, "renamed", "Boss"
	a.Due = date(t, "2026-03-01")
	if err := s.UpdateActivity(ctx, a); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.GetActivity(ctx, a.ID); got != a {
		t.Errorf("update: %+v", got)
	}
	bad := a
	bad.Due = date(t, "2025-01-01")
	if err := s.UpdateActivity(ctx, bad); !errors.Is(err, domain.ErrInvalid) {
		t.Errorf("update inverted: %v", err)
	}
	missing := a
	missing.ID = 999
	if err := s.UpdateActivity(ctx, missing); !errors.Is(err, ErrNotFound) {
		t.Errorf("update missing: %v", err)
	}
	if err := s.SetActivityStatus(ctx, a.ID, domain.StatusBlocked, date(t, "2026-05-01")); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.GetActivity(ctx, a.ID); got.Status != domain.StatusBlocked {
		t.Errorf("status: %+v", got)
	}
	if err := s.SetActivityStatus(ctx, a.ID, "weird", date(t, "2026-05-01")); !errors.Is(err, domain.ErrInvalid) {
		t.Errorf("bad status: %v", err)
	}
	if err := s.SetActivityStatus(ctx, 999, domain.StatusDone, date(t, "2026-05-01")); !errors.Is(err, ErrNotFound) {
		t.Errorf("status missing: %v", err)
	}
	if _, err := s.GetActivity(ctx, 999); !errors.Is(err, ErrNotFound) {
		t.Errorf("get missing: %v", err)
	}
	if as, err := s.ListActivities(ctx); err != nil || len(as) != 1 {
		t.Errorf("list: %v %v", as, err)
	}
	if err := s.DeleteActivity(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteActivity(ctx, a.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("delete twice: %v", err)
	}
}

func TestPriorityAndDoneOn(t *testing.T) {
	s := newTestStore(t)
	a := mustActivity(t, s, "P", "2026-01-01", "2026-02-01")
	if a.Priority != domain.PriorityNone || !a.DoneOn.IsZero() {
		t.Fatalf("defaults: %+v", a)
	}
	a.Priority = domain.PriorityHigh
	if err := s.UpdateActivity(ctx, a); err != nil {
		t.Fatal(err)
	}
	a.Priority = "urgent"
	if err := s.UpdateActivity(ctx, a); !errors.Is(err, domain.ErrInvalid) {
		t.Errorf("bad priority: %v", err)
	}
	set := func(st domain.Status, today string) domain.Activity {
		t.Helper()
		if err := s.SetActivityStatus(ctx, a.ID, st, date(t, today)); err != nil {
			t.Fatal(err)
		}
		got, _ := s.GetActivity(ctx, a.ID)
		return got
	}
	if got := set(domain.StatusDone, "2026-03-01"); got.DoneOn != date(t, "2026-03-01") || got.Priority != domain.PriorityHigh {
		t.Errorf("done: %+v", got)
	}
	if got := set(domain.StatusDone, "2026-04-01"); got.DoneOn != date(t, "2026-03-01") {
		t.Errorf("done to done must keep the date: %+v", got)
	}
	if got := set(domain.StatusActive, "2026-05-01"); !got.DoneOn.IsZero() {
		t.Errorf("reopened must clear the date: %+v", got)
	}
	if got := set(domain.StatusDone, "2026-06-01"); got.DoneOn != date(t, "2026-06-01") {
		t.Errorf("done again: %+v", got)
	}
}

func TestCascadeDeletes(t *testing.T) {
	s := newTestStore(t)
	count := func(table string) int {
		var n int
		if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM `+table).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	fill := func(name string) domain.Activity {
		a := mustActivity(t, s, name, "2026-01-01", "2026-12-31")
		if _, err := s.AddSubtask(ctx, a.ID, "t", date(t, "2026-01-01")); err != nil {
			t.Fatal(err)
		}
		if _, err := s.db.ExecContext(ctx, `INSERT INTO scope_acks (activity_id, date, baseline_count, last_subtask_id) VALUES (?, '2026-01-02', 1, 1)`, a.ID); err != nil {
			t.Fatal(err)
		}
		return a
	}
	a := fill("P1")
	fill("P2")
	if err := s.DeleteActivity(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	if count("activities") != 1 || count("subtasks") != 2 || count("journal_entries") != 1 || count("scope_acks") != 1 {
		t.Error("activity delete did not cascade exactly")
	}
	if err := s.DeleteProject(ctx, 2); err != nil {
		t.Fatal(err)
	}
	for _, tb := range []string{"activities", "subtasks", "journal_entries", "scope_acks"} {
		if count(tb) != 0 {
			t.Errorf("%s not emptied by project delete", tb)
		}
	}
}

func TestSubtasks(t *testing.T) {
	s := newTestStore(t)
	a := mustActivity(t, s, "P", "2026-01-01", "2026-12-31")
	d1, d2 := date(t, "2026-02-01"), date(t, "2026-02-10")

	st, err := s.AddSubtask(ctx, a.ID, "one", d1)
	if err != nil || st.ID == 0 {
		t.Fatal(st, err)
	}
	if _, err := s.AddSubtask(ctx, a.ID, " ", d1); !errors.Is(err, domain.ErrInvalid) {
		t.Errorf("blank: %v", err)
	}
	if _, err := s.AddSubtask(ctx, 999, "x", d1); !errors.Is(err, domain.ErrInvalid) {
		t.Errorf("missing activity: %v", err)
	}
	// created before activity start is allowed
	if _, err := s.AddSubtask(ctx, a.ID, "early", date(t, "2025-06-01")); err != nil {
		t.Errorf("pre-start subtask: %v", err)
	}

	closed, err := s.ToggleSubtask(ctx, st.ID, d2)
	if err != nil || closed.ClosedOn != d2 || closed.Text != "one" {
		t.Errorf("close: %+v %v", closed, err)
	}
	reopened, err := s.ToggleSubtask(ctx, st.ID, d2)
	if err != nil || !reopened.ClosedOn.IsZero() {
		t.Errorf("reopen: %+v %v", reopened, err)
	}
	if _, err := s.ToggleSubtask(ctx, 999, d2); !errors.Is(err, ErrNotFound) {
		t.Errorf("toggle missing: %v", err)
	}
	if _, err := s.ToggleSubtask(ctx, st.ID, 0); !errors.Is(err, domain.ErrInvalid) {
		t.Errorf("toggle zero today: %v", err)
	}
	// closing before creation violates closed_on >= created_on
	if _, err := s.ToggleSubtask(ctx, st.ID, date(t, "2026-01-01")); !errors.Is(err, domain.ErrInvalid) {
		t.Errorf("close before created: %v", err)
	}

	if _, err := s.ToggleSubtask(ctx, st.ID, d2); err != nil { // closed again
		t.Fatal(err)
	}
	if err := s.EditSubtask(ctx, st.ID, "one!", date(t, "2026-03-01")); !errors.Is(err, domain.ErrInvalid) {
		t.Errorf("created after closed: %v", err)
	}
	if err := s.EditSubtask(ctx, st.ID, "one!", d1.AddDays(1)); err != nil {
		t.Fatal(err)
	}
	if err := s.EditSubtask(ctx, st.ID, "", d1); !errors.Is(err, domain.ErrInvalid) {
		t.Errorf("edit blank: %v", err)
	}
	if err := s.EditSubtask(ctx, 999, "x", d1); !errors.Is(err, ErrNotFound) {
		t.Errorf("edit missing: %v", err)
	}
	subs, err := s.ListSubtasks(ctx, a.ID)
	if err != nil || len(subs) != 3 || subs[1].Text != "one!" || subs[1].ClosedOn != d2 || subs[1].CreatedOn != d1.AddDays(1) {
		t.Errorf("list: %+v %v", subs, err)
	}
	if all, _ := s.ListAllSubtasks(ctx); len(all) != 3 {
		t.Errorf("list all: %+v", all)
	}
	if err := s.DeleteSubtask(ctx, st.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteSubtask(ctx, st.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("delete twice: %v", err)
	}
}

// Deleting the newest subtask must never let its id be handed out again,
// otherwise ScopeAck.LastSubtaskID would misclassify the new subtask.
func TestSubtaskIDsAreNeverReused(t *testing.T) {
	s := newTestStore(t)
	a := mustActivity(t, s, "P", "2026-01-01", "2026-12-31")
	d := date(t, "2026-02-01")
	first, _ := s.AddSubtask(ctx, a.ID, "a", d)
	last, _ := s.AddSubtask(ctx, a.ID, "b", d)
	if err := s.DeleteSubtask(ctx, last.ID); err != nil {
		t.Fatal(err)
	}
	next, err := s.AddSubtask(ctx, a.ID, "c", d)
	if err != nil {
		t.Fatal(err)
	}
	if next.ID <= last.ID || next.ID <= first.ID {
		t.Errorf("id %d reused or not increasing (last was %d)", next.ID, last.ID)
	}
}
