package store

import (
	"encoding/json"
	"errors"
	"testing"

	"workline/internal/domain"
)

type expected struct {
	TilesRaw map[string]int `json:"tiles"`
	Projects []struct {
		ProjectID  int64  `json:"project_id"`
		Name       string `json:"name"`
		Activities int    `json:"activities"`
		Progress   int    `json:"progress"`
		ScopeFlags int    `json:"scope_flags"`
	} `json:"projects"`
	Activities []struct {
		ActivityID int64  `json:"activity_id"`
		Name       string `json:"name"`
		Total      int    `json:"total"`
		Closed     int    `json:"closed"`
		Open       int    `json:"open"`
		Progress   int    `json:"progress"`
		Scope      struct {
			Baseline      int  `json:"baseline"`
			Added         int  `json:"added"`
			GrowthPct     int  `json:"growth_pct"`
			Add30         int  `json:"add30"`
			Close30       int  `json:"close30"`
			GrowthFlag    bool `json:"growth_flag"`
			OutpacingFlag bool `json:"outpacing_flag"`
			Flagged       bool `json:"flagged"`
		} `json:"scope"`
	} `json:"activities"`
}

// TestSeedMatchesGolden seeds the baseline and recomputes every §4 value the
// baseline's `expected` block lists, from stored data and domain functions only.
func TestSeedMatchesGolden(t *testing.T) {
	var file struct {
		AsOf     string   `json:"as_of"`
		Expected expected `json:"expected"`
	}
	if err := json.Unmarshal(baselineJSON, &file); err != nil {
		t.Fatal(err)
	}
	asOf := date(t, file.AsOf)
	exp := file.Expected

	s := newTestStore(t)
	if err := s.Seed(ctx); err != nil {
		t.Fatal(err)
	}
	projects, err := s.ListProjects(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	acts, err := s.ListActivities(ctx)
	if err != nil {
		t.Fatal(err)
	}
	allSubs, err := s.ListAllSubtasks(ctx)
	if err != nil {
		t.Fatal(err)
	}
	subsOf := map[int64][]domain.Subtask{}
	for _, st := range allSubs {
		subsOf[st.ActivityID] = append(subsOf[st.ActivityID], st)
	}

	creep := map[int64]domain.Creep{}
	flagsOf := map[int64]int{} // by project
	tiles := map[string]int{}
	for _, a := range acts {
		ack, err := s.LatestScopeAck(ctx, a.ID)
		if err != nil {
			t.Fatal(err)
		}
		c := domain.ScopeCreep(a, subsOf[a.ID], ack, asOf)
		creep[a.ID] = c
		if c.Flagged {
			flagsOf[a.ProjectID]++
			tiles["scope_flags"]++
		}
		tiles[string(a.Status)]++
		tiles["open_subtasks"] += domain.OpenAt(subsOf[a.ID], asOf)
	}
	for k, want := range exp.TilesRaw {
		if tiles[k] != want {
			t.Errorf("tile %s = %d, want %d", k, tiles[k], want)
		}
	}
	if len(exp.TilesRaw) != 6 {
		t.Errorf("expected 6 tiles, golden has %d", len(exp.TilesRaw))
	}

	if len(exp.Projects) != len(projects) {
		t.Fatalf("projects: %d stored, %d expected", len(projects), len(exp.Projects))
	}
	byID := map[int64]domain.Project{}
	for _, p := range projects {
		byID[p.ID] = p
	}
	for _, e := range exp.Projects {
		p, ok := byID[e.ProjectID]
		if !ok || p.Name != e.Name {
			t.Errorf("project %d: stored %+v, want %q", e.ProjectID, p, e.Name)
			continue
		}
		n := 0
		var subs []domain.Subtask
		for _, a := range acts {
			if a.ProjectID == p.ID {
				n++
				subs = append(subs, subsOf[a.ID]...)
			}
		}
		if n != e.Activities || domain.ProjectProgress(subs) != e.Progress || flagsOf[p.ID] != e.ScopeFlags {
			t.Errorf("project %d: activities=%d progress=%d flags=%d, want %d/%d/%d",
				p.ID, n, domain.ProjectProgress(subs), flagsOf[p.ID], e.Activities, e.Progress, e.ScopeFlags)
		}
	}

	if len(exp.Activities) != len(acts) {
		t.Fatalf("activities: %d stored, %d expected", len(acts), len(exp.Activities))
	}
	actByID := map[int64]domain.Activity{}
	for _, a := range acts {
		actByID[a.ID] = a
	}
	for _, e := range exp.Activities {
		a, ok := actByID[e.ActivityID]
		if !ok || a.Name != e.Name {
			t.Errorf("activity %d: stored %+v, want %q", e.ActivityID, a, e.Name)
			continue
		}
		subs := subsOf[a.ID]
		closed := 0
		for _, st := range subs {
			if !st.ClosedOn.IsZero() && !st.ClosedOn.After(asOf) {
				closed++
			}
		}
		c := creep[a.ID]
		type vals struct {
			Total, Closed, Open, Progress           int
			Baseline, Added, Growth, Add30, Close30 int
			GrowthFlag, OutpacingFlag, Flagged      bool
		}
		got := vals{len(subs), closed, domain.OpenAt(subs, asOf), domain.Progress(a.Status, subs),
			c.Baseline, c.Added, c.GrowthPct, c.Add30, c.Close30, c.GrowthFlag, c.OutpacingFlag, c.Flagged}
		want := vals{e.Total, e.Closed, e.Open, e.Progress,
			e.Scope.Baseline, e.Scope.Added, e.Scope.GrowthPct, e.Scope.Add30, e.Scope.Close30,
			e.Scope.GrowthFlag, e.Scope.OutpacingFlag, e.Scope.Flagged}
		if got != want {
			t.Errorf("activity %d (%s):\n got  %+v\n want %+v", a.ID, a.Name, got, want)
		}
	}
}

func TestSeedPreservesBaselineRows(t *testing.T) {
	b, err := loadBaseline()
	if err != nil {
		t.Fatal(err)
	}
	s := newTestStore(t)
	if err := s.Seed(ctx); err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{
		"projects":        len(b.Data.Projects),
		"activities":      len(b.Data.Activities),
		"subtasks":        len(b.Data.Subtasks),
		"journal_entries": len(b.Data.JournalEntries),
		"scope_acks":      len(b.Data.ScopeAcks),
	}
	for table, want := range counts {
		var n int
		if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM `+table).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != want {
			t.Errorf("%s: %d rows, want %d", table, n, want)
		}
	}
	// ids preserved, and activity 9's subtasks predate its start (allowed)
	a9, err := s.GetActivity(ctx, 9)
	if err != nil {
		t.Fatal(err)
	}
	subs, _ := s.ListSubtasks(ctx, 9)
	early := 0
	for _, st := range subs {
		if st.CreatedOn.Before(a9.Start) {
			early++
		}
	}
	if early == 0 {
		t.Error("expected activity 9 subtasks created before its start_date")
	}
	// new subtask ids continue after the seeded maximum
	st, err := s.AddSubtask(ctx, 1, "after seed", date(t, "2026-11-18"))
	if err != nil || st.ID != int64(len(b.Data.Subtasks))+1 {
		t.Errorf("next subtask id = %d (%v), want %d", st.ID, err, len(b.Data.Subtasks)+1)
	}
}

func TestSeedRefusesNonEmptyDatabase(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.CreateProject(ctx, "existing"); err != nil {
		t.Fatal(err)
	}
	if err := s.Seed(ctx); !errors.Is(err, ErrNotEmpty) {
		t.Errorf("got %v", err)
	}
	if ps, _ := s.ListProjects(ctx, true); len(ps) != 1 {
		t.Errorf("seed touched a non-empty DB: %+v", ps)
	}

	s2 := newTestStore(t)
	if err := s2.Seed(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s2.Seed(ctx); !errors.Is(err, ErrNotEmpty) {
		t.Errorf("second seed: %v", err)
	}
}
