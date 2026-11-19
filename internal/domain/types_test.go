package domain

import (
	"errors"
	"fmt"
	"testing"
)

func TestValidate(t *testing.T) {
	ok := Activity{Name: "x", Status: StatusActive, Start: d(t, "2026-01-01"), Due: d(t, "2026-01-01")}
	acts := []struct {
		name string
		mut  func(*Activity)
		bad  bool
	}{
		{"ok", func(*Activity) {}, false},
		{"blank name", func(a *Activity) { a.Name = "  " }, true},
		{"bad status", func(a *Activity) { a.Status = "weird" }, true},
		{"no start", func(a *Activity) { a.Start = 0 }, true},
		{"no due", func(a *Activity) { a.Due = 0 }, true},
		{"due before start", func(a *Activity) { a.Due = d(t, "2025-12-31") }, true},
		{"longest span", func(a *Activity) { a.Due = a.Start.AddDays(maxSpanDays) }, false},
		{"span too long", func(a *Activity) { a.Due = a.Start.AddDays(maxSpanDays + 1) }, true},
		{"priority", func(a *Activity) { a.Priority = PriorityHigh }, false},
		{"bad priority", func(a *Activity) { a.Priority = "urgent" }, true},
		{"done with done date", func(a *Activity) { a.Status, a.DoneOn = StatusDone, d(t, "2026-01-01") }, false},
		{"done date on active", func(a *Activity) { a.DoneOn = d(t, "2026-01-01") }, true},
	}
	for _, c := range acts {
		a := ok
		c.mut(&a)
		err := a.Validate()
		if (err != nil) != c.bad || (err != nil && !errors.Is(err, ErrInvalid)) {
			t.Errorf("activity %s: err=%v", c.name, err)
		}
	}

	st := Task{Text: "t", CreatedOn: d(t, "2026-01-02")}
	subs := []struct {
		name string
		mut  func(*Task)
		bad  bool
	}{
		{"ok open", func(*Task) {}, false},
		{"ok closed same day", func(s *Task) { s.ClosedAt = s.CreatedOn.String() + " 00:00" }, false},
		{"blank", func(s *Task) { s.Text = "" }, true},
		{"no created", func(s *Task) { s.CreatedOn = 0 }, true},
		{"closed before created", func(s *Task) { s.ClosedAt = "2026-01-01 23:59" }, true},
		{"closed at", func(s *Task) { s.ClosedAt = "2026-02-03 17:40" }, false},
		{"closed at malformed", func(s *Task) { s.ClosedAt = "2026-02-03" }, true},
		{"closed at impossible", func(s *Task) { s.ClosedAt = "2026-02-30 10:00" }, true},
	}
	for _, c := range subs {
		s := st
		c.mut(&s)
		if err := s.Validate(); (err != nil) != c.bad {
			t.Errorf("task %s: err=%v", c.name, err)
		}
	}

	je := JournalEntry{Date: d(t, "2026-01-02"), Kind: KindUpdate, Text: "t"}
	js := []struct {
		name string
		mut  func(*JournalEntry)
		bad  bool
	}{
		{"ok", func(*JournalEntry) {}, false},
		{"ok scope_ack", func(j *JournalEntry) { j.Kind, j.ActivityID = KindScopeAck, 1 }, false},
		{"unassigned scope_ack", func(j *JournalEntry) { j.Kind = KindScopeAck }, true},
		{"blank", func(j *JournalEntry) { j.Text = " " }, true},
		{"no date", func(j *JournalEntry) { j.Date = 0 }, true},
		{"bad kind", func(j *JournalEntry) { j.Kind = "x" }, true},
		{"logged at", func(j *JournalEntry) { j.LoggedAt = "2026-11-18 09:05" }, false},
		{"logged at without minute", func(j *JournalEntry) { j.LoggedAt = "2026-11-18" }, true},
		{"logged at with seconds", func(j *JournalEntry) { j.LoggedAt = "2026-11-18 09:05:00" }, true},
		{"task", func(j *JournalEntry) { j.Kind = KindTask }, false},
		{"closed task", func(j *JournalEntry) { j.Kind, j.ClosedAt = KindTask, "2026-01-02 10:00" }, false},
		{"task closed before opened", func(j *JournalEntry) { j.Kind, j.ClosedAt = KindTask, "2026-01-01 10:00" }, true},
		{"closed update", func(j *JournalEntry) { j.ClosedAt = "2026-01-02 10:00" }, true},
		{"task priority", func(j *JournalEntry) { j.Kind, j.Priority = KindTask, PriorityHigh }, false},
		{"update priority", func(j *JournalEntry) { j.Priority = PriorityLow }, true},
		{"bad priority", func(j *JournalEntry) { j.Kind, j.Priority = KindTask, "urgent" }, true},
		{"closed time on update", func(j *JournalEntry) { j.ClosedAt = "2026-01-02 10:00" }, true},
		{"logged at impossible", func(j *JournalEntry) { j.LoggedAt = "2026-11-18 25:00" }, true},
	}
	for _, c := range js {
		j := je
		c.mut(&j)
		if err := j.Validate(); (err != nil) != c.bad {
			t.Errorf("journal %s: err=%v", c.name, err)
		}
	}
}

func TestTaskValidateAt(t *testing.T) {
	today := d(t, "2026-11-18")
	for _, c := range []struct {
		name   string
		closed string
		bad    bool
	}{
		{"open", "", false},
		{"closed today", "2026-11-18", false},
		{"closed yesterday", "2026-11-17", false},
		{"closed tomorrow", "2026-11-19", true},
	} {
		s := Task{Text: "t", CreatedOn: d(t, "2026-01-01")}
		if c.closed != "" {
			s.ClosedAt = c.closed + " 00:00"
		}
		err := s.ValidateAt(today)
		if (err != nil) != c.bad || (err != nil && !errors.Is(err, ErrInvalid)) {
			t.Errorf("%s: err=%v", c.name, err)
		}
	}
	if err := (Task{}).ValidateAt(today); err == nil {
		t.Error("base validation must still apply")
	}
}

func TestEnumsAndDefaultStatus(t *testing.T) {
	for _, s := range []Status{StatusPlanned, StatusActive, StatusBlocked, StatusDone} {
		if !s.Valid() {
			t.Error(s)
		}
	}
	for _, k := range []Kind{KindUpdate, KindDecision, KindBlocker, KindMeeting, KindScopeAck} {
		if !k.Valid() {
			t.Error(k)
		}
	}
	if Status("").Valid() || Kind("").Valid() {
		t.Error("empty enum valid")
	}
	today := d(t, "2026-11-18")
	if DefaultStatus(d(t, "2026-11-19"), today) != StatusPlanned ||
		DefaultStatus(today, today) != StatusActive ||
		DefaultStatus(d(t, "2026-01-01"), today) != StatusActive {
		t.Error("DefaultStatus")
	}
}

func TestJournalTime(t *testing.T) {
	if got := (JournalEntry{LoggedAt: "2026-11-18 09:05"}).Time(); got != "09:05" {
		t.Errorf("Time() = %q", got)
	}
	if got := (JournalEntry{}).Time(); got != "" {
		t.Errorf("Time() of an old entry = %q", got)
	}
}

func TestInvalidErrorKeepsItsMessageUnderWrapping(t *testing.T) {
	err := fmt.Errorf("add journal to activity 1: %w", Invalid("journal text is required"))
	var ie *InvalidError
	if !errors.Is(err, ErrInvalid) || !errors.As(err, &ie) || ie.Message != "journal text is required" {
		t.Fatalf("err = %v", err)
	}
	if got := err.Error(); got != "add journal to activity 1: invalid value: journal text is required" {
		t.Errorf("the full chain is kept for logs: %q", got)
	}
	if _, perr := ParseDate("soon"); !errors.As(perr, &ie) || ie.Message != `date "soon" is not YYYY-MM-DD` {
		t.Errorf("ParseDate error: %v", perr)
	}
}

func TestJournalEntryTask(t *testing.T) {
	j := JournalEntry{ID: 7, ActivityID: 2, Date: d(t, "2026-01-02"), Kind: KindTask, Text: "t", ClosedAt: "2026-01-05 09:00"}
	want := Task{ID: 7, ActivityID: 2, Text: "t", CreatedOn: d(t, "2026-01-02"), ClosedAt: "2026-01-05 09:00"}
	if got := j.Task(); got != want {
		t.Errorf("Task() = %+v, want %+v", got, want)
	}
}
