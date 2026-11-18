package web

import (
	"net/http"
	"strings"

	"workline/internal/domain"
)

type journalRow struct {
	ID                     int64
	Date, Time, Kind, Text string // Time is empty for entries without a logged time
	ReadOnly               bool   // scope_ack entries cannot be edited
	Kinds                  []option
}

func kindOptions(cur domain.Kind) []option {
	var out []option
	for _, k := range []domain.Kind{domain.KindUpdate, domain.KindDecision, domain.KindBlocker, domain.KindMeeting} {
		out = append(out, option{string(k), strings.ToUpper(string(k)[:1]) + string(k)[1:], k == cur})
	}
	return out
}

func journalRows(entries []domain.JournalEntry) []journalRow {
	var rows []journalRow
	for _, j := range entries {
		rows = append(rows, journalRow{
			ID: j.ID, Date: j.Date.String(), Time: j.Time(), Kind: string(j.Kind), Text: j.Text,
			ReadOnly: j.Kind == domain.KindScopeAck, Kinds: kindOptions(j.Kind),
		})
	}
	return rows
}

func (s *Server) addJournal(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	day := s.clock.Today()
	if err == nil && r.PostFormValue("date") != "" {
		day, err = formDate(r, "date")
	}
	if err == nil {
		_, err = s.store.AddJournal(r.Context(), domain.JournalEntry{
			ActivityID: id, Date: day, Kind: domain.Kind(r.PostFormValue("kind")), Text: r.PostFormValue("text"),
			LoggedAt: domain.Stamp(s.clock.Now()),
		})
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	redirect(w, r, nextURL(r))
}

func (s *Server) editJournal(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	var day domain.Date
	if err == nil {
		day, err = formDate(r, "date")
	}
	if err == nil {
		err = s.store.EditJournal(r.Context(), id, day, domain.Kind(r.PostFormValue("kind")), r.PostFormValue("text"))
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	redirect(w, r, nextURL(r))
}

func (s *Server) deleteJournal(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err == nil {
		err = s.store.DeleteJournal(r.Context(), id)
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	redirect(w, r, nextURL(r))
}
