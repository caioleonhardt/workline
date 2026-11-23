package web

import (
	"net/http"
	"strconv"
	"strings"

	"workline/internal/domain"
)

type journalRow struct {
	ID                          int64
	Date, Kind, KindLabel, Text string // Date is the entry's own (a task's opening day)
	Logged                      string // where it sits in the log: log date, and log time when known
	Summary                     string // the first line, with "…" when more follow (SPEC §10 item 107)
	Closed                      string // a closed task's close date
	Priority, PriorityLabel     string // a task's priority; empty for none
	IsTask                      bool   // a task keeps its kind, so its Edit form has no kind select
	ReadOnly                    bool   // scope_ack entries cannot be edited
	Starred                     bool   // a highlight of the report
	Kinds                       []option
	Priorities                  []option // a task's Edit form choices
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

func priorityLabel(p domain.Priority) string {
	if p == domain.PriorityNone {
		return ""
	}
	return capitalize(string(p))
}

func journalRows(entries []domain.JournalEntry) []journalRow {
	var rows []journalRow
	for _, j := range entries {
		row := journalRow{
			ID: j.ID, Date: j.Date.String(), Logged: strings.TrimSpace(j.LogDate().String() + " " + j.LogTime()), Kind: string(j.Kind), KindLabel: kindLabel(j.Kind), Text: j.Text,
			Closed: j.ClosedOn().String(), Priority: string(j.Priority), PriorityLabel: priorityLabel(j.Priority),
			IsTask: j.Kind == domain.KindTask, ReadOnly: j.Kind == domain.KindScopeAck, Starred: j.Starred, Kinds: kindOptions(noteKinds, j.Kind),
		}
		if first, rest, more := strings.Cut(j.Text, "\n"); more && strings.TrimSpace(rest) != "" {
			row.Summary = first + " …"
		} else {
			row.Summary = first
		}
		if row.IsTask {
			row.Priorities = priorityOptions(j.Priority)
		}
		rows = append(rows, row)
	}
	return rows
}

// prefixKinds are the bar's prefixes: "/t text" logs a task, and so on
// (SPEC §10 item 93).
var prefixKinds = map[byte]domain.Kind{
	't': domain.KindTask, 'd': domain.KindDecision, 'u': domain.KindUpdate, 'b': domain.KindBlocker, 'm': domain.KindMeeting,
}

// prefixPriorities follow /t: "/t1 text" is a high-priority task.
var prefixPriorities = map[byte]domain.Priority{'1': domain.PriorityHigh, '2': domain.PriorityMedium, '3': domain.PriorityLow}

// parsePrefix reads a leading "/t", "/t1".."/t3", "/d", "/u", "/b" or "/m"
// followed by a space or line break and some text. ok is false, and the text
// is to be logged as typed, for anything else.
func parsePrefix(text string) (kind domain.Kind, p domain.Priority, rest string, ok bool) {
	if len(text) < 3 || text[0] != '/' {
		return "", "", text, false
	}
	kind, ok = prefixKinds[text[1]]
	if !ok {
		return "", "", text, false
	}
	n := 2
	if kind == domain.KindTask {
		if pr, has := prefixPriorities[text[2]]; has {
			p, n = pr, 3
		}
	}
	if n >= len(text) || (text[n] != ' ' && text[n] != '\n') || strings.TrimSpace(text[n:]) == "" {
		return "", "", text, false
	}
	return kind, p, strings.TrimSpace(text[n:]), true
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
		j := domain.JournalEntry{ActivityID: id, Date: day, Text: formText(r), LoggedAt: domain.Stamp(s.clock.Now())}
		j.Kind, j.Priority = domain.Kind(r.PostFormValue("kind")), formPriority(r)
		if kind, p, rest, ok := parsePrefix(j.Text); ok {
			j.Kind, j.Priority, j.Text = kind, p, rest
		}
		if j.Kind != domain.KindTask {
			j.Priority = domain.PriorityNone // the bar's priority applies to tasks only
		}
		_, err = s.store.AddJournal(r.Context(), j)
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.done(w, r, nextURL(r), func(b *quickBar) { b.ClearQuick = fromQuickAdd(r) })
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
		err = s.store.EditJournal(r.Context(), id, activity, domain.JournalEntry{
			Date: day, Kind: domain.Kind(r.PostFormValue("kind")), Text: formText(r), Priority: formPriority(r),
		})
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

// starJournal stars or unstars an entry (SPEC §10 item 110).
func (s *Server) starJournal(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err == nil {
		err = s.store.ToggleStar(r.Context(), id)
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.done(w, r, nextURL(r))
}

func formPriority(r *http.Request) domain.Priority {
	return domain.Priority(r.PostFormValue("priority"))
}

// formText returns the posted multiline text with browser line endings
// (CRLF) turned into "\n" and trailing blank space removed.
func formText(r *http.Request) string {
	return strings.TrimRight(strings.ReplaceAll(r.PostFormValue("text"), "\r\n", "\n"), " \t\r\n")
}
