package store

import (
	"testing"

	"workline/internal/domain"
)

func TestToggleStampsAndClearsClosedAt(t *testing.T) {
	s := newTestStore(t)
	a := mustActivity(t, s, "P", "2026-01-01", "2026-12-31")
	st, _ := s.AddTask(ctx, a.ID, "x", date(t, "2026-02-01"))

	closed, err := s.ToggleTask(ctx, st.ID, domain.FixedClock{D: date(t, "2026-02-10"), Hour: 17, Minute: 40})
	if err != nil || closed.ClosedOn != date(t, "2026-02-10") || closed.ClosedAt != "2026-02-10 17:40" {
		t.Fatalf("close: %+v %v", closed, err)
	}
	if subs, _ := s.ListTasks(ctx, a.ID); subs[1].ClosedAt != "2026-02-10 17:40" {
		t.Errorf("stored: %+v", subs[1])
	}
	// editing text or creation date leaves the close time alone
	if err := s.EditJournal(ctx, st.ID, date(t, "2026-02-02"), domain.KindTask, "renamed"); err != nil {
		t.Fatal(err)
	}
	if subs, _ := s.ListTasks(ctx, a.ID); subs[1].ClosedAt != "2026-02-10 17:40" || subs[1].Text != "renamed" {
		t.Errorf("after edit: %+v", subs[1])
	}
	reopened, err := s.ToggleTask(ctx, st.ID, fixed(date(t, "2026-02-11")))
	if err != nil || !reopened.ClosedOn.IsZero() || reopened.ClosedAt != "" {
		t.Errorf("reopen: %+v %v", reopened, err)
	}
	again, _ := s.ToggleTask(ctx, st.ID, domain.FixedClock{D: date(t, "2026-02-12"), Hour: 8, Minute: 5})
	if again.ClosedAt != "2026-02-12 08:05" {
		t.Errorf("closed again: %+v", again)
	}
}

func TestClosedAtChecks(t *testing.T) {
	s := newTestStore(t)
	a := mustActivity(t, s, "P", "2026-01-01", "2026-12-31")
	st, _ := s.AddTask(ctx, a.ID, "x", date(t, "2026-02-01"))
	for name, q := range map[string]string{
		"open task":       `UPDATE journal_entries SET closed_at = '2026-02-10 09:00' WHERE id = ?`,
		"impossible time": `UPDATE journal_entries SET closed_on = '2026-02-10', closed_at = '2026-02-10 25:00' WHERE id = ?`,
		"other day":       `UPDATE journal_entries SET closed_on = '2026-02-10', closed_at = '2026-02-11 09:00' WHERE id = ?`,
		"seconds":         `UPDATE journal_entries SET closed_on = '2026-02-10', closed_at = '2026-02-10 09:00:00' WHERE id = ?`,
	} {
		if _, err := s.db.ExecContext(ctx, q, st.ID); err == nil {
			t.Errorf("%s: database accepted it", name)
		}
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE journal_entries SET closed_on = '2026-02-10', closed_at = '2026-02-10 09:00' WHERE id = ?`, st.ID); err != nil {
		t.Errorf("valid close time rejected: %v", err)
	}
}
