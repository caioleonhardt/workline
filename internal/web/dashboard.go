package web

import (
	"context"
	"net/http"
	"net/url"

	"workline/internal/domain"
)

// quickAddView is where the quick-add bar's next entry goes: the selected
// activity, or Unassigned (ID 0).
type quickAddView struct {
	ID            int64
	Name, Project string
	Next          string
	ClearHref     string // the same page without the selection; empty for Unassigned
}

type tileView struct {
	Label, Class string
	Value        int
}

func buildTiles(t domain.Tiles) []tileView {
	return []tileView{
		{"Active", "active", t.Active}, {"Blocked", "blocked", t.Blocked}, {"Planned", "planned", t.Planned},
		{"Done this year", "done", t.DoneThisYear}, {"Open tasks now", "", t.OpenTasks},
		{"Scope creep flags", "creep", t.ScopeFlags},
	}
}

type dashboardView struct {
	Title, Next string
	NewHref     string // header link that opens the inline form
	ExportHref  string
	CancelHref  string // same page without the form
	ShowForm    bool
	Today       string
	AllProjects []domain.Project // choices in the form
	Panel       *panelView       // nil unless sel names an existing activity
	Controls    controlsView
	QuickAdd    quickAddView // the quick-add bar's target
	QuickKinds  []option
	QuickPrios  []option // the bar's task priority
	Journal     journalView
	ClearQuick  bool // a successful quick add also replaces the text field with an empty one
	Tiles       []tileView
	Timeline    timelineView
}

func (s *Server) dashboard(w http.ResponseWriter, r *http.Request) {
	s.renderDashboard(w, r, r.URL.RequestURI())
}

// renderDashboard renders the dashboard for the given path and query.
func (s *Server) renderDashboard(w http.ResponseWriter, r *http.Request, uri string, mods ...func(*dashboardView)) {
	v, err := s.buildDashboard(r.Context(), uri)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	for _, mod := range mods {
		mod(&v)
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
	subs, err := s.store.ListAllTasks(ctx)
	if err != nil {
		return dashboardView{}, err
	}
	acks, err := s.store.ListLatestScopeAcks(ctx)
	if err != nil {
		return dashboardView{}, err
	}
	subsOf := map[int64][]domain.Task{}
	for _, st := range subs {
		subsOf[st.ActivityID] = append(subsOf[st.ActivityID], st)
	}
	live := map[int64]bool{} // projects that are not archived
	for _, p := range projects {
		live[p.ID] = true
	}
	var panel *panelView
	var tileItems []domain.ActivityWithTasks // the project filter only, not period or status
	creeps := map[int64]domain.Creep{}
	counts := map[domain.Status]int{}
	creepCount := 0
	byProject := map[int64][]domain.Activity{}
	liveOf := map[int64][]domain.Activity{} // every live activity, for filing entries
	var visibleIDs []int64
	itemOf := map[int64]domain.ActivityWithTasks{}
	for _, a := range acts {
		if !live[a.ProjectID] {
			continue
		}
		it := domain.ActivityWithTasks{Activity: a, Tasks: subsOf[a.ID]}
		if ack, ok := acks[a.ID]; ok {
			it.Ack = &ack
		}
		itemOf[a.ID] = it
		liveOf[a.ProjectID] = append(liveOf[a.ProjectID], a)
		if a.ID == f.Sel {
			panel = buildPanel(it.Activity, projects, it.Tasks, it.Ack, today, uri)
		}
		if f.Project == 0 || a.ProjectID == f.Project {
			tileItems = append(tileItems, it)
		}
		if !f.inScope(a) {
			continue
		}
		creep := domain.ScopeCreep(a, it.Tasks, it.Ack, today)
		creeps[a.ID] = creep
		counts[a.Status]++
		if creep.Flagged {
			creepCount++
		}
		if f.shows(a.Status, creep.Flagged) {
			byProject[a.ProjectID] = append(byProject[a.ProjectID], a)
			visibleIDs = append(visibleIDs, a.ID)
		}
	}
	var filing []domain.Group
	for _, p := range projects {
		if as := liveOf[p.ID]; len(as) > 0 {
			filing = append(filing, domain.Group{Project: p, Activities: as})
		}
	}
	ids := visibleIDs
	unassigned := f.Project == 0 // Unassigned entries belong to no project
	if panel != nil {
		ids, unassigned = []int64{panel.ID}, false // a selection narrows the journal to its activity
	}
	journal, err := s.buildJournal(ctx, f, uri, ids, unassigned, panel, itemOf, projects, filing)
	if err != nil {
		return dashboardView{}, err
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
		ExportHref: f.ExportURL(),
		CancelHref: withParam(uri, "new", ""),
		ShowForm:   f.New,
		Today:      today.String(), AllProjects: projects, Panel: panel,
		Controls: buildControls(f, projects, counts, creepCount), QuickKinds: kindOptions(quickKinds, domain.KindUpdate), QuickPrios: priorityOptions(domain.PriorityNone), Journal: journal,
		Timeline: buildTimeline(f, today, timelineData{Groups: groups, ItemOf: itemOf, Creeps: creeps}),
		QuickAdd: quickAddView{Name: "Unassigned", Next: uri},
	}
	tiles := domain.ComputeTiles(tileItems, f.Year, today)
	if f.Project == 0 {
		tiles.OpenTasks += domain.OpenAt(subsOf[0], today) // Unassigned tasks
	}
	v.Tiles = buildTiles(tiles)
	if panel != nil {
		v.QuickAdd = quickAddView{ID: panel.ID, Name: panel.Name, Project: panel.ProjectName, Next: uri, ClearHref: panel.CloseHref}
	}
	return v, nil
}
