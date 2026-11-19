package web

import (
	"context"
	"net/http"
	"net/url"

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
	s.renderDashboard(w, r, r.URL.RequestURI())
}

// renderDashboard renders the dashboard for the given path and query.
func (s *Server) renderDashboard(w http.ResponseWriter, r *http.Request, uri string) {
	v, err := s.buildDashboard(r.Context(), uri)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	w.Header().Set("Vary", "HX-Request")
	name := "dashboard.html"
	if isHX(r) {
		name = "dash-fragment"
	}
	s.render(w, http.StatusOK, name, v)
}

// buildDashboard loads everything the dashboard shows for the given path and query.
func (s *Server) buildDashboard(ctx context.Context, uri string) (dashboardView, error) {
	today := s.clock.Today()
	u, err := url.Parse(uri)
	if err != nil {
		return dashboardView{}, domain.Invalid("bad dashboard address")
	}
	f := parseFilter(u.Query(), today)
	projects, err := s.store.ListProjects(ctx, false)
	if err != nil {
		return dashboardView{}, err
	}
	acts, err := s.store.ListActivities(ctx)
	if err != nil {
		return dashboardView{}, err
	}
	subs, err := s.store.ListAllSubtasks(ctx)
	if err != nil {
		return dashboardView{}, err
	}
	acks, err := s.store.ListLatestScopeAcks(ctx)
	if err != nil {
		return dashboardView{}, err
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
			if panel, err = s.panel(ctx, it, projects, today, uri); err != nil {
				return dashboardView{}, err
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
		Title: "Workline", Next: uri,
		NewHref:    withParam(uri, "new", "1"),
		CancelHref: withParam(uri, "new", ""),
		ShowForm:   f.New,
		Today:      today.String(), AllProjects: projects, Panel: panel,
		Controls: buildControls(f, projects, counts, creepCount),
		Timeline: buildTimeline(f, today, timelineData{Groups: groups, ItemOf: itemOf, Creeps: creeps}),
		Tiles:    buildTiles(domain.ComputeTiles(tileItems, f.Year, today)),
	}
	return v, nil
}

// panel loads what the detail panel needs for one activity.
func (s *Server) panel(ctx context.Context, it domain.ActivityWithSubtasks, projects []domain.Project, today domain.Date, next string) (*panelView, error) {
	journal, err := s.store.ListJournal(ctx, it.ID)
	if err != nil {
		return nil, err
	}
	return buildPanel(it.Activity, projects, it.Subtasks, journal, it.Ack, today, next), nil
}
