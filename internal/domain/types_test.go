package domain

import (
	"errors"
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
	}
	for _, c := range acts {
		a := ok
		c.mut(&a)
		err := a.Validate()
		if (err != nil) != c.bad || (err != nil && !errors.Is(err, ErrInvalid)) {
			t.Errorf("activity %s: err=%v", c.name, err)
		}
	}

	st := Subtask{Text: "t", CreatedOn: d(t, "2026-01-02")}
	subs := []struct {
		name string
		mut  func(*Subtask)
		bad  bool
	}{
		{"ok open", func(*Subtask) {}, false},
		{"ok closed same day", func(s *Subtask) { s.ClosedOn = s.CreatedOn }, false},
		{"blank", func(s *Subtask) { s.Text = "" }, true},
		{"no created", func(s *Subtask) { s.CreatedOn = 0 }, true},
		{"closed before created", func(s *Subtask) { s.ClosedOn = d(t, "2026-01-01") }, true},
	}
	for _, c := range subs {
		s := st
		c.mut(&s)
		if err := s.Validate(); (err != nil) != c.bad {
			t.Errorf("subtask %s: err=%v", c.name, err)
		}
	}

	je := JournalEntry{Date: d(t, "2026-01-02"), Kind: KindUpdate, Text: "t"}
	js := []struct {
		name string
		mut  func(*JournalEntry)
		bad  bool
	}{
		{"ok", func(*JournalEntry) {}, false},
		{"ok scope_ack", func(j *JournalEntry) { j.Kind = KindScopeAck }, false},
		{"blank", func(j *JournalEntry) { j.Text = " " }, true},
		{"no date", func(j *JournalEntry) { j.Date = 0 }, true},
		{"bad kind", func(j *JournalEntry) { j.Kind = "x" }, true},
	}
	for _, c := range js {
		j := je
		c.mut(&j)
		if err := j.Validate(); (err != nil) != c.bad {
			t.Errorf("journal %s: err=%v", c.name, err)
		}
	}
}

func TestSubtaskValidateAt(t *testing.T) {
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
		s := Subtask{Text: "t", CreatedOn: d(t, "2026-01-01")}
		if c.closed != "" {
			s.ClosedOn = d(t, c.closed)
		}
		err := s.ValidateAt(today)
		if (err != nil) != c.bad || (err != nil && !errors.Is(err, ErrInvalid)) {
			t.Errorf("%s: err=%v", c.name, err)
		}
	}
	if err := (Subtask{}).ValidateAt(today); err == nil {
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
