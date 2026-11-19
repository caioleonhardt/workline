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
