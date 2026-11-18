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
}

type projectView struct {
	Name       string
	Activities []activityRow
}

type dashboardView struct {
	Title    string
	Next     string
	Projects []projectView
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
	rows := map[int64][]activityRow{}
	for _, a := range acts {
		rows[a.ProjectID] = append(rows[a.ProjectID], activityRow{
			ID: a.ID, Name: a.Name, Status: string(a.Status), Priority: string(a.Priority),
			Start: a.Start.String(), Due: a.Due.String(),
			Open: domain.OpenAt(subsOf[a.ID], today), Total: len(subsOf[a.ID]),
			Progress: domain.Progress(a.Status, subsOf[a.ID]),
			Href:     withParam(r.URL.RequestURI(), "sel", strconv.FormatInt(a.ID, 10)),
		})
	}
	v := dashboardView{Title: "Workline", Next: r.URL.RequestURI()}
	for _, p := range projects {
		v.Projects = append(v.Projects, projectView{Name: p.Name, Activities: rows[p.ID]})
	}
	s.render(w, http.StatusOK, "dashboard.html", v)
}
