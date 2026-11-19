package web

import (
	"net/http"
	"slices"

	"workline/internal/domain"
)

type taskRow struct {
	ID                      int64
	Text, Created, Closed   string // Closed is empty while open
	Priority, PriorityLabel string // empty for none
	Priorities              []option
}

// taskRows lists tasks open first, by priority (domain.SortTasks).
func taskRows(subs []domain.Task) []taskRow {
	sorted := slices.Clone(subs)
	domain.SortTasks(sorted)
	var rows []taskRow
	for _, st := range sorted {
		rows = append(rows, taskRow{st.ID, st.Text, st.CreatedOn.String(), st.ClosedOn.String(),
			string(st.Priority), priorityLabel(st.Priority), priorityOptions(st.Priority)})
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
