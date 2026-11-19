package web

import (
	"net/http"
	"strconv"

	"workline/internal/domain"
)

type option struct {
	Value, Label string
	Selected     bool
}

type panelView struct {
	Next                                 string
	ID                                   int64
	ProjectName, Name, RequestedBy       string
	Start, Due                           string
	Today                                string
	Subtasks                             []subtaskRow
	Journal                              []journalRow
	Scope                                scopeView
	Chart                                chartView
	Status                               string
	NewKinds                             []option
	Progress, Closed, Total              int
	Statuses, Priorities, ProjectChoices []option
}

func statusOptions(cur domain.Status) []option {
	var out []option
	for _, s := range []domain.Status{domain.StatusPlanned, domain.StatusActive, domain.StatusBlocked, domain.StatusDone} {
		out = append(out, option{string(s), string(s), s == cur})
	}
	return out
}

func priorityOptions(cur domain.Priority) []option {
	out := []option{{"", "None", cur == domain.PriorityNone}}
	for _, p := range []domain.Priority{domain.PriorityHigh, domain.PriorityMedium, domain.PriorityLow} {
		out = append(out, option{string(p), string(p), p == cur})
	}
	return out
}

func buildPanel(a domain.Activity, projects []domain.Project, subs []domain.Subtask, journal []domain.JournalEntry, ack *domain.ScopeAck, today domain.Date, next string) *panelView {
	p := &panelView{
		Next: next, ID: a.ID, Name: a.Name, RequestedBy: a.RequestedBy,
		Start: a.Start.String(), Due: a.Due.String(), Today: today.String(), Subtasks: subtaskRows(subs),
		Journal: journalRows(journal), Scope: buildScope(a, subs, ack, today),
		Chart: buildChart(a, subs, today), Status: string(a.Status), NewKinds: kindOptions(domain.KindUpdate),
		Progress: domain.Progress(a.Status, subs), Total: len(subs),
		Statuses: statusOptions(a.Status), Priorities: priorityOptions(a.Priority),
	}
	for _, st := range subs {
		if !st.ClosedOn.IsZero() {
			p.Closed++
		}
	}
	for _, pr := range projects {
		p.ProjectChoices = append(p.ProjectChoices, option{strconv.FormatInt(pr.ID, 10), pr.Name, pr.ID == a.ProjectID})
		if pr.ID == a.ProjectID {
			p.ProjectName = pr.Name
		}
	}
	return p
}

// updateActivity edits the activity's fields (form has name) and/or its status
// (form has status). Two small forms post here.
func (s *Server) updateActivity(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := pathID(r)
	if err == nil && r.PostFormValue("name") != "" {
		err = s.saveActivityFields(r, id)
	}
	if err == nil && r.PostForm.Has("status") {
		err = s.store.SetActivityStatus(ctx, id, domain.Status(r.PostFormValue("status")), s.clock.Today())
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.done(w, r, nextURL(r))
}

func (s *Server) saveActivityFields(r *http.Request, id int64) error {
	a, err := s.store.GetActivity(r.Context(), id)
	if err != nil {
		return err
	}
	a.Name = r.PostFormValue("name")
	a.RequestedBy = r.PostFormValue("requested_by")
	a.Priority = domain.Priority(r.PostFormValue("priority"))
	if a.ProjectID, err = strconv.ParseInt(r.PostFormValue("project"), 10, 64); err != nil {
		return domain.Invalid("choose a project")
	}
	if a.Start, err = formDate(r, "start"); err != nil {
		return err
	}
	if a.Due, err = formDate(r, "due"); err != nil {
		return err
	}
	return s.store.UpdateActivity(r.Context(), a)
}

func (s *Server) deleteActivity(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err == nil {
		err = s.store.DeleteActivity(r.Context(), id)
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.done(w, r, withParam(nextURL(r), "sel", ""))
}
