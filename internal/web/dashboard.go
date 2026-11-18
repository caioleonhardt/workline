package web

import (
	"net/http"
	"strconv"

	"workline/internal/domain"
)

type activityRow struct {
	ID                     int64
	Name, Status, Priority string
	Start, Due             string
	Open, Total, Progress  int
	Href                   string
	Selected               bool
}

type projectView struct {
	Name       string
	Activities []activityRow
}

type dashboardView struct {
	Title, Next string
	NewHref     string // header link that opens the inline form
	CancelHref  string // same page without the form
	ShowForm    bool
	Today       string
	AllProjects []domain.Project // choices in the form
	Projects    []projectView
	Panel       *panelView // nil unless sel names an existing activity
}

func (s *Server) dashboard(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	today := s.clock.Today()
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
	subsOf := map[int64][]domain.Subtask{}
	for _, st := range subs {
		subsOf[st.ActivityID] = append(subsOf[st.ActivityID], st)
	}
	sel, _ := strconv.ParseInt(r.URL.Query().Get("sel"), 10, 64)
	rows := map[int64][]activityRow{}
	var panel *panelView
	for _, a := range acts {
		if a.ID == sel {
			journal, err := s.store.ListJournal(ctx, a.ID)
			if err != nil {
				s.fail(w, r, err)
				return
			}
			ack, err := s.store.LatestScopeAck(ctx, a.ID)
			if err != nil {
				s.fail(w, r, err)
				return
			}
			panel = buildPanel(a, projects, subsOf[a.ID], journal, ack, today, r.URL.RequestURI())
		}
		rows[a.ProjectID] = append(rows[a.ProjectID], activityRow{
			ID: a.ID, Name: a.Name, Status: string(a.Status), Priority: string(a.Priority),
			Start: a.Start.String(), Due: a.Due.String(),
			Open: domain.OpenAt(subsOf[a.ID], today), Total: len(subsOf[a.ID]),
			Progress: domain.Progress(a.Status, subsOf[a.ID]),
			Selected: a.ID == sel,
			Href:     withParam(r.URL.RequestURI(), "sel", strconv.FormatInt(a.ID, 10)),
		})
	}
	v := dashboardView{
		Title: "Workline", Next: r.URL.RequestURI(),
		NewHref:    withParam(r.URL.RequestURI(), "new", "1"),
		CancelHref: withParam(r.URL.RequestURI(), "new", ""),
		ShowForm:   r.URL.Query().Get("new") == "1",
		Today:      today.String(), AllProjects: projects, Panel: panel,
	}
	for _, p := range projects {
		v.Projects = append(v.Projects, projectView{Name: p.Name, Activities: rows[p.ID]})
	}
	s.render(w, http.StatusOK, "dashboard.html", v)
}
