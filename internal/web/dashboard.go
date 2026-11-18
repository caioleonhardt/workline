package web

import (
	"context"
	"net/http"

	"workline/internal/domain"
)

type tileView struct {
	Label, Class string
	Value        int
}

func buildTiles(t domain.Tiles) []tileView {
	return []tileView{
		{"Active", "active", t.Active}, {"Blocked", "blocked", t.Blocked}, {"Planned", "planned", t.Planned},
		{"Done this year", "done", t.DoneThisYear}, {"Open subtasks now", "", t.OpenSubtasks},
		{"Scope creep flags", "creep", t.ScopeFlags},
	}
}

type dashboardView struct {
	Title, Next string
	NewHref     string // header link that opens the inline form
	CancelHref  string // same page without the form
	ShowForm    bool
	Today       string
	AllProjects []domain.Project // choices in the form
	Panel       *panelView       // nil unless sel names an existing activity
	Controls    controlsView
	Tiles       []tileView
	Timeline    timelineView
}

func (s *Server) dashboard(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	today := s.clock.Today()
	f := parseFilter(r.URL.Query(), today)
	projects, err := s.store.ListProjects(ctx, false)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	acts, err := s.store.ListActivities(ctx)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	subs, err := s.store.ListAllSubtasks(ctx)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	acks, err := s.store.ListLatestScopeAcks(ctx)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	subsOf := map[int64][]domain.Subtask{}
	for _, st := range subs {
		subsOf[st.ActivityID] = append(subsOf[st.ActivityID], st)
	}
	live := map[int64]bool{} // projects that are not archived
	for _, p := range projects {
		live[p.ID] = true
	}
	var panel *panelView
	var tileItems []domain.ActivityWithSubtasks // the project filter only, not period or status
	creeps := map[int64]domain.Creep{}
	counts := map[domain.Status]int{}
	creepCount := 0
	byProject := map[int64][]domain.Activity{}
	itemOf := map[int64]domain.ActivityWithSubtasks{}
	for _, a := range acts {
		if !live[a.ProjectID] {
			continue
		}
		it := domain.ActivityWithSubtasks{Activity: a, Subtasks: subsOf[a.ID]}
		if ack, ok := acks[a.ID]; ok {
			it.Ack = &ack
		}
		itemOf[a.ID] = it
		if a.ID == f.Sel {
			if panel, err = s.panel(ctx, it, projects, today, r.URL.RequestURI()); err != nil {
				s.fail(w, r, err)
				return
			}
		}
		if f.Project == 0 || a.ProjectID == f.Project {
			tileItems = append(tileItems, it)
		}
		if !f.inScope(a) {
			continue
		}
		creep := domain.ScopeCreep(a, it.Subtasks, it.Ack, today)
		creeps[a.ID] = creep
		counts[a.Status]++
		if creep.Flagged {
			creepCount++
		}
		if f.shows(a.Status, creep.Flagged) {
			byProject[a.ProjectID] = append(byProject[a.ProjectID], a)
		}
	}
	var groups []domain.Group
	for _, p := range projects {
		if as := byProject[p.ID]; len(as) > 0 {
			domain.SortActivities(as)
			groups = append(groups, domain.Group{Project: p, Activities: as})
		}
	}
	domain.SortGroups(groups)

	v := dashboardView{
		Title: "Workline", Next: r.URL.RequestURI(),
		NewHref:    withParam(r.URL.RequestURI(), "new", "1"),
		CancelHref: withParam(r.URL.RequestURI(), "new", ""),
		ShowForm:   f.New,
		Today:      today.String(), AllProjects: projects, Panel: panel,
		Controls: buildControls(f, projects, counts, creepCount),
		Timeline: buildTimeline(f, today, groups, itemOf, creeps),
		Tiles:    buildTiles(domain.ComputeTiles(tileItems, f.Year, today)),
	}
	s.render(w, http.StatusOK, "dashboard.html", v)
}

// panel loads what the detail panel needs for one activity.
func (s *Server) panel(ctx context.Context, it domain.ActivityWithSubtasks, projects []domain.Project, today domain.Date, next string) (*panelView, error) {
	journal, err := s.store.ListJournal(ctx, it.ID)
	if err != nil {
		return nil, err
	}
	return buildPanel(it.Activity, projects, it.Subtasks, journal, it.Ack, today, next), nil
}
