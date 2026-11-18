package web

import (
	"net/http"

	"workline/internal/domain"
)

type subtaskRow struct {
	ID                    int64
	Text, Created, Closed string // Closed is empty while open
}

func subtaskRows(subs []domain.Subtask) []subtaskRow {
	var rows []subtaskRow
	for _, st := range subs {
		rows = append(rows, subtaskRow{st.ID, st.Text, st.CreatedOn.String(), st.ClosedOn.String()})
	}
	return rows
}

func (s *Server) addSubtask(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	created := s.clock.Today()
	if err == nil && r.PostFormValue("created_on") != "" {
		created, err = formDate(r, "created_on")
	}
	if err == nil {
		_, err = s.store.AddSubtask(r.Context(), id, r.PostFormValue("text"), created)
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.done(w, r, nextURL(r))
}

func (s *Server) toggleSubtask(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err == nil {
		_, err = s.store.ToggleSubtask(r.Context(), id, s.clock)
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.done(w, r, nextURL(r))
}

// editSubtask changes text and creation date only; closed_on is set by toggle
// alone (SPEC §10 item 24).
func (s *Server) editSubtask(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	var created domain.Date
	if err == nil {
		created, err = formDate(r, "created_on")
	}
	if err == nil {
		err = s.store.EditSubtask(r.Context(), id, r.PostFormValue("text"), created)
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.done(w, r, nextURL(r))
}

func (s *Server) deleteSubtask(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err == nil {
		err = s.store.DeleteSubtask(r.Context(), id)
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.done(w, r, nextURL(r))
}
