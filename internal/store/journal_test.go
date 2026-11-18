package store

import (
	"errors"
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
	if err != nil || len(list) != 4 {
		t.Fatalf("list: %+v %v", list, err)
	}
	wantOrder := []string{"new-b", "new-a", "old", "Activity created"} // date desc, id desc
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

	if err := s.EditJournal(ctx, old.ID, date(t, "2026-02-02"), domain.KindUpdate, "edited"); err != nil {
		t.Fatal(err)
	}
	if err := s.EditJournal(ctx, old.ID, date(t, "2026-02-02"), domain.KindScopeAck, "x"); !errors.Is(err, domain.ErrInvalid) {
		t.Errorf("edit into scope_ack: %v", err)
	}
	if err := s.EditJournal(ctx, old.ID, date(t, "2026-02-02"), domain.KindUpdate, ""); !errors.Is(err, domain.ErrInvalid) {
		t.Errorf("edit blank: %v", err)
	}
	if err := s.EditJournal(ctx, 999, date(t, "2026-02-02"), domain.KindUpdate, "x"); !errors.Is(err, ErrNotFound) {
		t.Errorf("edit missing: %v", err)
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
	if err := s.EditJournal(ctx, ackID, date(t, "2026-04-02"), domain.KindUpdate, "x"); !errors.Is(err, ErrReadOnly) {
		t.Errorf("edit scope_ack: %v", err)
	}
	if err := s.DeleteJournal(ctx, ackID); !errors.Is(err, ErrReadOnly) {
		t.Errorf("delete scope_ack: %v", err)
	}
}

func TestListJournalMarks(t *testing.T) {
	s := newTestStore(t)
	a := mustActivity(t, s, "P", "2026-01-01", "2026-12-31") // "Activity created" dated 2026-01-01
	b := mustActivity(t, s, "Q", "2026-01-01", "2026-12-31")
	s.AddJournal(ctx, domain.JournalEntry{ActivityID: a.ID, Date: date(t, "2026-03-01"), Kind: domain.KindBlocker, Text: "x"})
	marks, err := s.ListJournalMarks(ctx)
	if err != nil || len(marks) != 3 {
		t.Fatalf("marks: %+v %v", marks, err)
	}
	want := JournalMark{ActivityID: a.ID, Date: date(t, "2026-03-01"), Kind: domain.KindBlocker}
	if marks[2] != want || marks[0].ActivityID != a.ID || marks[1].ActivityID != b.ID {
		t.Errorf("marks: %+v", marks)
	}
}
