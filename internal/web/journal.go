package web

import (
	"net/http"
	"strconv"
	"strings"

	"workline/internal/domain"
)

type journalRow struct {
	ID                                int64
	Date, Time, Kind, KindLabel, Text string // Time is empty for entries without a logged time
	Closed                            string // a closed task's close date
	IsTask                            bool   // a task keeps its kind, so its Edit form has no kind select
	ReadOnly                          bool   // scope_ack entries cannot be edited
	Kinds                             []option
}

// noteKinds are the kinds an entry other than a task can be edited to; the
// quick-add bar also offers task.
var (
	noteKinds  = []domain.Kind{domain.KindUpdate, domain.KindDecision, domain.KindBlocker, domain.KindMeeting}
	quickKinds = []domain.Kind{domain.KindUpdate, domain.KindDecision, domain.KindBlocker, domain.KindMeeting, domain.KindTask}
)

func kindOptions(kinds []domain.Kind, cur domain.Kind) []option {
	var out []option
	for _, k := range kinds {
		out = append(out, option{string(k), kindLabel(k), k == cur})
	}
	return out
}

func kindLabel(k domain.Kind) string {
	if k == domain.KindScopeAck {
		return "Scope acknowledged"
	}
	return strings.ToUpper(string(k)[:1]) + string(k)[1:]
}

func journalRows(entries []domain.JournalEntry) []journalRow {
	var rows []journalRow
	for _, j := range entries {
		rows = append(rows, journalRow{
			ID: j.ID, Date: j.Date.String(), Time: j.Time(), Kind: string(j.Kind), KindLabel: kindLabel(j.Kind), Text: j.Text,
			Closed: j.ClosedOn.String(), IsTask: j.Kind == domain.KindTask,
			ReadOnly: j.Kind == domain.KindScopeAck, Kinds: kindOptions(noteKinds, j.Kind),
		})
	}
	return rows
}

// formActivity reads the form's `activity` field: an activity id, or empty
// for Unassigned.
func formActivity(r *http.Request) (int64, error) {
	v := r.PostFormValue("activity")
	if v == "" {
		return 0, nil
	}
	id, err := strconv.ParseInt(v, 10, 64)
	if err != nil || id <= 0 {
		return 0, domain.Invalid("choose an activity, or none")
	}
	return id, nil
}

// addJournal logs an entry on the activity named by the form's `activity`
// field, or Unassigned when it is empty or missing.
func (s *Server) addJournal(w http.ResponseWriter, r *http.Request) {
	id, err := formActivity(r)
	day := s.clock.Today()
	if err == nil && r.PostFormValue("date") != "" {
		day, err = formDate(r, "date")
	}
	if err == nil {
		_, err = s.store.AddJournal(r.Context(), domain.JournalEntry{
			ActivityID: id, Date: day, Kind: domain.Kind(r.PostFormValue("kind")), Text: formText(r),
			LoggedAt: domain.Stamp(s.clock.Now()),
		})
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.done(w, r, nextURL(r), func(v *dashboardView) { v.ClearQuick = fromQuickAdd(r) })
}

// editJournal rewrites an entry, and files it when the form has an
// `activity` field (empty for Unassigned).
func (s *Server) editJournal(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	var day domain.Date
	if err == nil {
		day, err = formDate(r, "date")
	}
	var activity *int64
	if err == nil && r.PostForm.Has("activity") {
		var a int64
		a, err = formActivity(r)
		activity = &a
	}
	if err == nil {
		err = s.store.EditJournal(r.Context(), id, activity, day, domain.Kind(r.PostFormValue("kind")), formText(r))
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.done(w, r, nextURL(r))
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
	s.done(w, r, nextURL(r))
}

// formText returns the posted multiline text with browser line endings
// (CRLF) turned into "\n" and trailing blank space removed.
func formText(r *http.Request) string {
	return strings.TrimRight(strings.ReplaceAll(r.PostFormValue("text"), "\r\n", "\n"), " \t\r\n")
}
