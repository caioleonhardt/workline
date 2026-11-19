package store

import (
	"errors"
	"strings"
	"testing"

	"workline/internal/domain"
)

func TestJournal(t *testing.T) {
	s := newTestStore(t)
	a := mustActivity(t, s, "P", "2026-01-01", "2026-12-31") // creation entry dated 2026-01-01
	add := func(day, text string, k domain.Kind) domain.JournalEntry {
		t.Helper()
		j, err := s.AddJournal(ctx, domain.JournalEntry{ActivityID: a.ID, Date: date(t, day), Kind: k, Text: text})
		if err != nil {
			t.Fatal(err)
		}
		return j
	}
	old := add("2026-02-01", "old", domain.KindMeeting)
	add("2026-03-01", "new-a", domain.KindDecision)
	newB := add("2026-03-01", "new-b", domain.KindBlocker)

	list, err := s.ListJournal(ctx, a.ID)
	if err != nil || len(list) != 5 {
		t.Fatalf("list: %+v %v", list, err)
	}
	wantOrder := []string{"new-b", "new-a", "old", "first", "Activity created"} // date desc, id desc
	for i, w := range wantOrder {
		if list[i].Text != w {
			t.Errorf("order[%d] = %q, want %q", i, list[i].Text, w)
		}
	}

	if _, err := s.AddJournal(ctx, domain.JournalEntry{ActivityID: a.ID, Date: date(t, "2026-02-01"), Kind: domain.KindScopeAck, Text: "x"}); !errors.Is(err, domain.ErrInvalid) {
		t.Errorf("scope_ack via AddJournal: %v", err)
	}
	if _, err := s.AddJournal(ctx, domain.JournalEntry{ActivityID: a.ID, Date: date(t, "2026-02-01"), Kind: domain.KindUpdate, Text: " "}); !errors.Is(err, domain.ErrInvalid) {
		t.Errorf("blank: %v", err)
	}
	if _, err := s.AddJournal(ctx, domain.JournalEntry{ActivityID: 999, Date: date(t, "2026-02-01"), Kind: domain.KindUpdate, Text: "x"}); !errors.Is(err, domain.ErrInvalid) {
		t.Errorf("missing activity: %v", err)
	}

	if err := s.EditJournal(ctx, old.ID, nil, domain.JournalEntry{Date: date(t, "2026-02-02"), Kind: domain.KindUpdate, Text: "edited"}); err != nil {
		t.Fatal(err)
	}
	if err := s.EditJournal(ctx, old.ID, nil, domain.JournalEntry{Date: date(t, "2026-02-02"), Kind: domain.KindScopeAck, Text: "x"}); !errors.Is(err, domain.ErrInvalid) {
		t.Errorf("edit into scope_ack: %v", err)
	}
	if err := s.EditJournal(ctx, old.ID, nil, domain.JournalEntry{Date: date(t, "2026-02-02"), Kind: domain.KindUpdate, Text: ""}); !errors.Is(err, domain.ErrInvalid) {
		t.Errorf("edit blank: %v", err)
	}
	if err := s.EditJournal(ctx, 999, nil, domain.JournalEntry{Date: date(t, "2026-02-02"), Kind: domain.KindUpdate, Text: "x"}); !errors.Is(err, ErrNotFound) {
		t.Errorf("edit missing: %v", err)
	}
	// a task keeps its kind, and other entries cannot become tasks
	if err := s.EditJournal(ctx, old.ID, nil, domain.JournalEntry{Date: date(t, "2026-02-02"), Kind: domain.KindTask, Text: "edited"}); !errors.Is(err, domain.ErrInvalid) {
		t.Errorf("update into task: %v", err)
	}
	task, _ := s.AddTask(ctx, a.ID, "a task", date(t, "2026-02-01"))
	if err := s.EditJournal(ctx, task.ID, nil, domain.JournalEntry{Date: date(t, "2026-02-01"), Kind: domain.KindUpdate, Text: "a task"}); !errors.Is(err, domain.ErrInvalid) {
		t.Errorf("task into update: %v", err)
	}
	if err := s.DeleteJournal(ctx, newB.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteJournal(ctx, newB.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("delete twice: %v", err)
	}

	// scope_ack rows are read-only
	var ackID int64
	res, err := s.db.ExecContext(ctx, `INSERT INTO journal_entries (activity_id, date, kind, text) VALUES (?, '2026-04-01', 'scope_ack', 'acked')`, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	ackID, _ = res.LastInsertId()
	if err := s.EditJournal(ctx, ackID, nil, domain.JournalEntry{Date: date(t, "2026-04-02"), Kind: domain.KindUpdate, Text: "x"}); !errors.Is(err, ErrReadOnly) {
		t.Errorf("edit scope_ack: %v", err)
	}
	if err := s.DeleteJournal(ctx, ackID); !errors.Is(err, ErrReadOnly) {
		t.Errorf("delete scope_ack: %v", err)
	}
}

func TestJournalLoggedAt(t *testing.T) {
	s := newTestStore(t)
	a := mustActivity(t, s, "P", "2026-01-01", "2026-12-31")
	list, _ := s.ListJournal(ctx, a.ID)
	if list[0].LoggedAt != "2026-01-01 09:30" {
		t.Errorf("Activity created entry: %q", list[0].LoggedAt)
	}

	add := func(day, at, text string) int64 {
		t.Helper()
		j, err := s.AddJournal(ctx, domain.JournalEntry{ActivityID: a.ID, Date: date(t, day), Kind: domain.KindUpdate, Text: text, LoggedAt: at})
		if err != nil {
			t.Fatal(err)
		}
		return j.ID
	}
	add("2026-03-01", "2026-03-01 09:00", "early")
	add("2026-03-01", "2026-03-01 17:45", "late")
	add("2026-03-01", "2026-03-01 17:45", "same time, later id")
	add("2026-03-01", "", "no time")
	backfilled := add("2026-02-01", "2026-03-05 08:00", "backfilled, logged later") // lists by its date, not by when it was logged
	list, _ = s.ListJournal(ctx, a.ID)
	var got []string
	for _, j := range list {
		got = append(got, j.Text)
	}
	want := []string{"same time, later id", "late", "early", "no time", "backfilled, logged later", "first", "Activity created"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("order:\n got %v\nwant %v", got, want)
	}
	if list[3].LoggedAt != "" || list[3].Time() != "" || list[0].Time() != "17:45" {
		t.Errorf("times: %+v", list)
	}

	// editing never changes when it was logged
	if err := s.EditJournal(ctx, backfilled, nil, domain.JournalEntry{Date: date(t, "2026-02-02"), Kind: domain.KindDecision, Text: "edited"}); err != nil {
		t.Fatal(err)
	}
	list, _ = s.ListJournal(ctx, a.ID)
	for _, j := range list {
		if j.ID == backfilled && (j.LoggedAt != "2026-03-05 08:00" || j.Text != "edited") {
			t.Errorf("after edit: %+v", j)
		}
	}
	if _, err := s.AddJournal(ctx, domain.JournalEntry{ActivityID: a.ID, Date: date(t, "2026-03-01"), Kind: domain.KindUpdate, Text: "x", LoggedAt: "yesterday"}); !errors.Is(err, domain.ErrInvalid) {
		t.Errorf("bad stamp: %v", err)
	}
	// the database checks the format as well
	if _, err := s.db.ExecContext(ctx, `INSERT INTO journal_entries (activity_id, date, kind, text, logged_at) VALUES (?, '2026-03-01', 'update', 'x', '2026-13-45 99:99')`, a.ID); err == nil {
		t.Error("database accepted an impossible logged_at")
	}
}

func TestAckScopeStampsJournalEntry(t *testing.T) {
	s := newTestStore(t)
	a := mustActivity(t, s, "P", "2026-06-01", "2026-12-31")
	s.AddTask(ctx, a.ID, "n1", date(t, "2026-11-10"))
	s.AddTask(ctx, a.ID, "n2", date(t, "2026-11-10"))
	if _, err := s.AckScope(ctx, a.ID, domain.FixedClock{D: date(t, "2026-11-18"), Hour: 16, Minute: 2}); err != nil {
		t.Fatal(err)
	}
	list, _ := s.ListJournal(ctx, a.ID)
	if list[0].Kind != domain.KindScopeAck || list[0].LoggedAt != "2026-11-18 16:02" {
		t.Errorf("ack entry: %+v", list[0])
	}
}

func TestListJournalBetween(t *testing.T) {
	s := newTestStore(t)
	a := mustActivity(t, s, "P", "2026-01-01", "2026-12-31") // "Activity created", 2026-01-01 09:30
	b := mustActivity(t, s, "Q", "2026-01-01", "2026-12-31")
	add := func(id int64, day, at, text string) {
		t.Helper()
		if _, err := s.AddJournal(ctx, domain.JournalEntry{ActivityID: id, Date: date(t, day), Kind: domain.KindUpdate, Text: text, LoggedAt: at}); err != nil {
			t.Fatal(err)
		}
	}
	add(a.ID, "2026-03-01", "2026-03-01 17:00", "late")
	add(b.ID, "2026-03-01", "2026-03-01 08:00", "early")
	add(a.ID, "2026-03-01", "", "untimed")
	add(a.ID, "2026-05-01", "", "may")
	texts := func(from, to string) string {
		t.Helper()
		var f, to2 domain.Date
		if from != "" {
			f = date(t, from)
		}
		if to != "" {
			to2 = date(t, to)
		}
		es, err := s.ListJournalBetween(ctx, f, to2)
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, e := range es {
			out = append(out, e.Text)
		}
		return strings.Join(out, "|")
	}
	if got, want := texts("", ""), "Activity created|first|Activity created|first|untimed|early|late|may"; got != want {
		t.Errorf("all: %q, want %q", got, want)
	}
	if got, want := texts("2026-03-01", "2026-03-01"), "untimed|early|late"; got != want { // bounds are inclusive
		t.Errorf("one day: %q, want %q", got, want)
	}
	if got, want := texts("2026-03-02", ""), "may"; got != want {
		t.Errorf("from only: %q, want %q", got, want)
	}
	if got, want := texts("", "2026-01-01"), "Activity created|first|Activity created|first"; got != want {
		t.Errorf("to only: %q, want %q", got, want)
	}
	if got := texts("2027-01-01", ""); got != "" {
		t.Errorf("empty range: %q", got)
	}
}

func TestListJournalFeed(t *testing.T) {
	s := newTestStore(t)
	a := mustActivity(t, s, "P", "2026-01-01", "2026-12-31") // "Activity created", 2026-01-01 09:30
	b := mustActivity(t, s, "Q", "2026-01-01", "2026-12-31")
	c := mustActivity(t, s, "R", "2026-01-01", "2026-12-31")
	add := func(id int64, day, at, text string) {
		t.Helper()
		if _, err := s.AddJournal(ctx, domain.JournalEntry{ActivityID: id, Date: date(t, day), Kind: domain.KindUpdate, Text: text, LoggedAt: at}); err != nil {
			t.Fatal(err)
		}
	}
	add(a.ID, "2026-03-01", "2026-03-01 08:00", "a early")
	add(b.ID, "2026-03-01", "2026-03-01 17:00", "b late")
	add(a.ID, "2026-03-01", "", "a untimed")
	add(c.ID, "2026-03-02", "2026-03-02 10:00", "c other activity")
	add(b.ID, "2026-02-01", "", "b old")
	texts := func(ids []int64, limit int) (string, bool) {
		t.Helper()
		es, more, err := s.ListJournalFeed(ctx, ids, false, limit)
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, e := range es {
			out = append(out, e.Text)
		}
		return strings.Join(out, "|"), more
	}
	// newest first, untimed last within a date; activity c is not asked for
	got, more := texts([]int64{a.ID, b.ID}, 10)
	if want := "b late|a early|a untimed|b old|first|Activity created|first|Activity created"; got != want || more {
		t.Errorf("feed = %q more=%v, want %q", got, more, want)
	}
	// the limit, and the "more" flag only when something is left
	got, more = texts([]int64{a.ID, b.ID}, 3)
	if got != "b late|a early|a untimed" || !more {
		t.Errorf("limit 3 = %q more=%v", got, more)
	}
	if got, more = texts([]int64{a.ID, b.ID}, 8); more || strings.Count(got, "|") != 7 {
		t.Errorf("limit equal to the count = %q more=%v", got, more)
	}
	// one activity; none; no limit
	if got, _ = texts([]int64{c.ID}, 10); got != "c other activity|first|Activity created" {
		t.Errorf("one activity = %q", got)
	}
	if got, more = texts(nil, 10); got != "" || more {
		t.Errorf("no ids = %q", got)
	}
	if got, more = texts([]int64{a.ID}, 0); got != "" || more {
		t.Errorf("limit 0 = %q", got)
	}
}

// Entries without an activity are Unassigned until filed (SPEC §10 item 91).
func TestUnassignedJournal(t *testing.T) {
	s := newTestStore(t)
	a := mustActivity(t, s, "P", "2026-01-01", "2026-12-31")
	note, err := s.AddJournal(ctx, domain.JournalEntry{Date: date(t, "2026-03-01"), Kind: domain.KindUpdate, Text: "loose note"})
	if err != nil {
		t.Fatal(err)
	}
	task, err := s.AddTask(ctx, 0, "loose task", date(t, "2026-03-01"))
	if err != nil || task.ActivityID != 0 {
		t.Fatalf("task: %+v %v", task, err)
	}
	if _, err := s.AddJournal(ctx, domain.JournalEntry{Date: date(t, "2026-03-01"), Kind: domain.KindScopeAck, Text: "x"}); err == nil {
		t.Error("an unassigned acknowledgement was accepted")
	}

	// the feed shows them only when asked, alone or with activities
	feed := func(ids []int64, unassigned bool) int {
		t.Helper()
		es, _, err := s.ListJournalFeed(ctx, ids, unassigned, 50)
		if err != nil {
			t.Fatal(err)
		}
		return len(es)
	}
	if feed(nil, true) != 2 || feed([]int64{a.ID}, true) != 4 || feed([]int64{a.ID}, false) != 2 || feed(nil, false) != 0 {
		t.Errorf("feeds: %d %d %d", feed(nil, true), feed([]int64{a.ID}, true), feed([]int64{a.ID}, false))
	}
	if all, _ := s.ListAllTasks(ctx); len(all) != 2 || all[0].ID != task.ID {
		t.Errorf("all tasks, Unassigned first: %+v", all)
	}

	// filing: nil keeps the activity, an id moves it, 0 unassigns it again
	file := func(id int64, to *int64, kind domain.Kind) error {
		return s.EditJournal(ctx, id, to, domain.JournalEntry{Date: date(t, "2026-03-01"), Kind: kind, Text: "filed"})
	}
	if err := file(task.ID, nil, domain.KindTask); err != nil {
		t.Fatal(err)
	}
	if ts, _ := s.ListTasks(ctx, a.ID); len(ts) != 1 {
		t.Errorf("nil moved the task: %+v", ts)
	}
	if err := file(task.ID, &a.ID, domain.KindTask); err != nil {
		t.Fatal(err)
	}
	if ts, _ := s.ListTasks(ctx, a.ID); len(ts) != 2 || ts[1].ID != task.ID {
		t.Errorf("task not filed: %+v", ts)
	}
	none, missing := int64(0), int64(999)
	if err := file(task.ID, &none, domain.KindTask); err != nil || feed(nil, true) != 2 {
		t.Errorf("unassign: %v", err)
	}
	if err := file(note.ID, &missing, domain.KindUpdate); !errors.Is(err, domain.ErrInvalid) {
		t.Errorf("filed under a missing activity: %v", err)
	}
}

func TestListAllJournal(t *testing.T) {
	s := newTestStore(t)
	a := mustActivity(t, s, "P", "2026-01-01", "2026-12-31") // "Activity created" and "first", 2026-01-01
	for _, e := range []domain.JournalEntry{
		{ActivityID: a.ID, Date: date(t, "2026-03-01"), Kind: domain.KindUpdate, Text: "untimed"},
		{ActivityID: a.ID, Date: date(t, "2026-03-01"), Kind: domain.KindUpdate, Text: "timed", LoggedAt: "2026-03-01 08:00"},
		{Date: date(t, "2026-02-01"), Kind: domain.KindMeeting, Text: "loose"},
	} {
		if _, err := s.AddJournal(ctx, e); err != nil {
			t.Fatal(err)
		}
	}
	es, err := s.ListAllJournal(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range es {
		out = append(out, e.Text)
	}
	if got, want := strings.Join(out, "|"), "timed|untimed|loose|first|Activity created"; got != want {
		t.Errorf("got %q", got)
	}
}
