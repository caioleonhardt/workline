package web

import (
	"net/http"

	"workline/internal/domain"
)

type taskRow struct {
	ID                    int64
	Text, Created, Closed string // Closed is empty while open
}

func taskRows(subs []domain.Task) []taskRow {
	var rows []taskRow
	for _, st := range subs {
		rows = append(rows, taskRow{st.ID, st.Text, st.CreatedOn.String(), st.ClosedOn.String()})
	}
	return rows
}

func (s *Server) addTask(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	created := s.clock.Today()
	if err == nil && r.PostFormValue("created_on") != "" {
		created, err = formDate(r, "created_on")
	}
	if err == nil {
		_, err = s.store.AddTask(r.Context(), id, r.PostFormValue("text"), created)
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.done(w, r, nextURL(r))
}

func (s *Server) toggleTask(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err == nil {
		_, err = s.store.ToggleTask(r.Context(), id, s.clock)
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.done(w, r, nextURL(r))
}

// editTask changes text and creation date only; closed_on is set by toggle
// alone (SPEC §10 item 24).
func (s *Server) editTask(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	var created domain.Date
	if err == nil {
		created, err = formDate(r, "created_on")
	}
	if err == nil {
		err = s.store.EditTask(r.Context(), id, r.PostFormValue("text"), created)
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.done(w, r, nextURL(r))
}

func (s *Server) deleteTask(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err == nil {
		err = s.store.DeleteTask(r.Context(), id)
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.done(w, r, nextURL(r))
}
