package store

import (
	"errors"
	"testing"

	"workline/internal/domain"
)

func journalCount(t *testing.T, s *Store, activityID int64) int {
	t.Helper()
	j, err := s.ListJournal(ctx, activityID)
	if err != nil {
		t.Fatal(err)
	}
	return len(j)
}

func TestAckScope(t *testing.T) {
	s := newTestStore(t)
	today := date(t, "2026-11-18")
	a := mustActivity(t, s, "P", "2026-06-01", "2026-12-31") // kickoff cutoff 2026-06-08
	for i := 0; i < 1; i++ {                                 // plus the subtask made with the activity: baseline 2
		s.AddSubtask(ctx, a.ID, "base", date(t, "2026-06-01"))
	}

	// not flagged yet: refused, nothing written
	before := journalCount(t, s, a.ID)
	if _, err := s.AckScope(ctx, a.ID, today); !errors.Is(err, ErrNotFlagged) {
		t.Fatalf("unflagged: %v", err)
	}
	if journalCount(t, s, a.ID) != before {
		t.Error("refused ack wrote a journal entry")
	}
	if ack, err := s.LatestScopeAck(ctx, a.ID); err != nil || ack != nil {
		t.Errorf("refused ack left an ack: %+v %v", ack, err)
	}

	// grow: 3 newer subtasks in the window => growth 150%, add30 3 > close30 0
	var last domain.Subtask
	for i := 0; i < 3; i++ {
		last, _ = s.AddSubtask(ctx, a.ID, "new", date(t, "2026-11-10"))
	}
	ack, err := s.AckScope(ctx, a.ID, today)
	if err != nil {
		t.Fatal(err)
	}
	if ack.BaselineCount != 5 || ack.LastSubtaskID != last.ID || ack.Date != today || ack.ID == 0 {
		t.Errorf("ack: %+v", ack)
	}
	j, _ := s.ListJournal(ctx, a.ID)
	wantText := "Scope change acknowledged: 2 → 5 subtasks (+150%); last 30 days +3 / −0. New baseline: 5."
	if j[0].Kind != domain.KindScopeAck || j[0].Text != wantText || j[0].Date != today {
		t.Errorf("journal: %+v", j[0])
	}

	// round trip: flag cleared after reload
	got, err := s.LatestScopeAck(ctx, a.ID)
	if err != nil || got == nil || *got != ack {
		t.Fatalf("latest: %+v %v", got, err)
	}
	subs, _ := s.ListSubtasks(ctx, a.ID)
	act, _ := s.GetActivity(ctx, a.ID)
	if c := domain.ScopeCreep(act, subs, got, today); c.Flagged {
		t.Errorf("still flagged after ack: %+v", c)
	}
	if _, err := s.AckScope(ctx, a.ID, today); !errors.Is(err, ErrNotFlagged) {
		t.Errorf("second ack on cleared activity: %v", err)
	}

	// regrow past 75% of the new baseline (5): 4 more => 80%; latest ack wins
	for i := 0; i < 4; i++ {
		last, _ = s.AddSubtask(ctx, a.ID, "more", date(t, "2026-11-12"))
	}
	ack2, err := s.AckScope(ctx, a.ID, today)
	if err != nil {
		t.Fatal(err)
	}
	if ack2.ID <= ack.ID || ack2.BaselineCount != 9 || ack2.LastSubtaskID != last.ID {
		t.Errorf("second ack: %+v", ack2)
	}
	if latest, _ := s.LatestScopeAck(ctx, a.ID); latest == nil || latest.ID != ack2.ID {
		t.Errorf("latest should be the highest id: %+v", latest)
	}
}

func TestAckScopeMissingActivityWritesNothing(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.AckScope(ctx, 999, date(t, "2026-11-18")); !errors.Is(err, ErrNotFound) {
		t.Errorf("got %v", err)
	}
	var n int
	s.db.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM scope_acks) + (SELECT count(*) FROM journal_entries)`).Scan(&n)
	if n != 0 {
		t.Errorf("%d rows written", n)
	}
}

func TestAckScopeRollsBackWhenJournalFails(t *testing.T) {
	s := newTestStore(t)
	a := mustActivity(t, s, "P", "2026-06-01", "2026-12-31")
	s.AddSubtask(ctx, a.ID, "base", date(t, "2026-06-01"))
	s.AddSubtask(ctx, a.ID, "new", date(t, "2026-11-10"))
	// A trigger makes the journal insert fail after the ack insert succeeded.
	if _, err := s.db.ExecContext(ctx, `CREATE TRIGGER no_ack_journal BEFORE INSERT ON journal_entries
		WHEN NEW.kind = 'scope_ack' BEGIN SELECT RAISE(ABORT, 'boom'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AckScope(ctx, a.ID, date(t, "2026-11-18")); err == nil {
		t.Fatal("expected failure")
	}
	if ack, _ := s.LatestScopeAck(ctx, a.ID); ack != nil {
		t.Errorf("ack survived a failed transaction: %+v", ack)
	}
}

func TestListLatestScopeAcks(t *testing.T) {
	s := newTestStore(t)
	today := date(t, "2026-11-18")
	a := mustActivity(t, s, "P", "2026-06-01", "2026-12-31")
	b := mustActivity(t, s, "Q", "2026-06-01", "2026-12-31")
	if got, err := s.ListLatestScopeAcks(ctx); err != nil || len(got) != 0 {
		t.Fatalf("empty: %v %v", got, err)
	}
	for _, id := range []int64{a.ID, b.ID} {
		s.AddSubtask(ctx, id, "new", date(t, "2026-11-10"))
		s.AddSubtask(ctx, id, "new", date(t, "2026-11-10"))
	}
	first, err := s.AckScope(ctx, a.ID, today)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 4; i++ {
		s.AddSubtask(ctx, a.ID, "more", date(t, "2026-11-12"))
	}
	second, err := s.AckScope(ctx, a.ID, today)
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.ListLatestScopeAcks(ctx)
	if err != nil || len(got) != 1 || got[a.ID] != second || got[a.ID].ID <= first.ID {
		t.Errorf("latest acks: %+v %v", got, err)
	}
}
