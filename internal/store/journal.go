package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"workline/internal/domain"
)

const journalColumns = `id, coalesce(activity_id, 0), date, kind, text, logged_at, closed_at, priority`

// An entry sits in the journal at its log date and time: a closed task at its
// close, anything else at its date and logged time (SPEC §10 item 109).
const (
	logDate = `coalesce(substr(closed_at, 1, 10), date)`
	logTime = `coalesce(closed_at, logged_at)`
	newest  = `ORDER BY ` + logDate + ` DESC, ` + logTime + ` IS NULL, ` + logTime + ` DESC, id DESC`
	oldest  = `ORDER BY ` + logDate + `, ` + logTime + ` IS NOT NULL, ` + logTime + `, id`
)

func insertJournal(ctx context.Context, q queryer, j domain.JournalEntry) (int64, error) {
	if err := j.Validate(); err != nil {
		return 0, err
	}
	res, err := q.ExecContext(ctx,
		`INSERT INTO journal_entries (activity_id, date, kind, text, logged_at, priority) VALUES (?, ?, ?, ?, ?, ?)`,
		activityRef(j.ActivityID), j.Date.String(), string(j.Kind), j.Text, sql.NullString{String: j.LoggedAt, Valid: j.LoggedAt != ""}, string(j.Priority))
	if err != nil {
		return 0, mapErr(err)
	}
	return res.LastInsertId()
}

func userKind(k domain.Kind) error {
	if k == domain.KindScopeAck {
		return domain.Invalid("acknowledgement entries are created only by Acknowledge")
	}
	return nil
}

// activityRef stores an activity id, with 0 (Unassigned) as NULL.
func activityRef(id int64) sql.NullInt64 { return sql.NullInt64{Int64: id, Valid: id != 0} }

// AddJournal adds a user-written entry; the caller stamps LoggedAt. Kind
// scope_ack is refused. ActivityID 0 logs it Unassigned.
func (s *Store) AddJournal(ctx context.Context, j domain.JournalEntry) (domain.JournalEntry, error) {
	if err := userKind(j.Kind); err != nil {
		return j, err
	}
	id, err := insertJournal(ctx, s.db, j)
	if err != nil {
		return j, fmt.Errorf("add journal to activity %d: %w", j.ActivityID, err)
	}
	j.ID = id
	return j, nil
}

func (s *Store) queryJournal(ctx context.Context, query string, args ...any) ([]domain.JournalEntry, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list journal: %w", err)
	}
	defer rows.Close()
	var out []domain.JournalEntry
	for rows.Next() {
		var j domain.JournalEntry
		var date, kind, priority string
		var loggedAt, closedAt sql.NullString
		if err := rows.Scan(&j.ID, &j.ActivityID, &date, &kind, &j.Text, &loggedAt, &closedAt, &priority); err != nil {
			return nil, fmt.Errorf("scan journal: %w", err)
		}
		j.Kind, j.LoggedAt, j.ClosedAt, j.Priority = domain.Kind(kind), loggedAt.String, closedAt.String, domain.Priority(priority)
		if j.Date, err = parseDate(date); err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

// ListJournal returns an activity's entries, newest first by log date.
func (s *Store) ListJournal(ctx context.Context, activityID int64) ([]domain.JournalEntry, error) {
	return s.queryJournal(ctx,
		`SELECT `+journalColumns+` FROM journal_entries
		 WHERE activity_id = ? `+newest, activityID)
}

// ListJournalFeed returns up to limit entries of the given activities (and
// the Unassigned ones when unassigned is set), newest first (log date, then log
// time with untimed entries last, then id), and whether more exist beyond the
// limit.
func (s *Store) ListJournalFeed(ctx context.Context, activityIDs []int64, unassigned bool, limit int) ([]domain.JournalEntry, bool, error) {
	if (len(activityIDs) == 0 && !unassigned) || limit <= 0 {
		return nil, false, nil
	}
	args := make([]any, 0, len(activityIDs)+2)
	args = append(args, unassigned)
	for _, id := range activityIDs {
		args = append(args, id)
	}
	args = append(args, limit+1)
	in := strings.TrimPrefix(strings.Repeat(",?", len(activityIDs)), ",")
	entries, err := s.queryJournal(ctx,
		`SELECT `+journalColumns+` FROM journal_entries
		 WHERE (? AND activity_id IS NULL) OR activity_id IN (`+in+`)
		 `+newest+` LIMIT ?`, args...)
	if err != nil {
		return nil, false, err
	}
	if len(entries) > limit {
		return entries[:limit], true, nil
	}
	return entries, false, nil
}

// ListAllJournal returns every entry, Unassigned included, in the feed's
// order: newest first (log date, then log time with untimed entries last, then id).
func (s *Store) ListAllJournal(ctx context.Context) ([]domain.JournalEntry, error) {
	return s.queryJournal(ctx,
		`SELECT `+journalColumns+` FROM journal_entries
		 `+newest)
}

// ListJournalBetween returns the entries of all activities whose log date lies
// in from..to inclusive (a zero bound is open), oldest first: by log date, then
// log time with untimed entries first, then id.
func (s *Store) ListJournalBetween(ctx context.Context, from, to domain.Date) ([]domain.JournalEntry, error) {
	return s.queryJournal(ctx,
		`SELECT `+journalColumns+` FROM journal_entries
		 WHERE (?1 = '' OR `+logDate+` >= ?1) AND (?2 = '' OR `+logDate+` <= ?2)
		 `+oldest, from.String(), to.String())
}

// EditJournal rewrites the date, kind, text and priority of entry id from j,
// and files it under *activity (0 for Unassigned) unless activity is nil.
// scope_ack entries are read-only, and an entry cannot become a task or stop
// being one. A task's date (the day it was opened) cannot move after the day
// it was closed.
func (s *Store) EditJournal(ctx context.Context, id int64, activity *int64, j domain.JournalEntry) error {
	if err := userKind(j.Kind); err != nil {
		return err
	}
	if err := j.Validate(); err != nil {
		return err
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE journal_entries SET date = ?1, kind = ?2, text = ?3, priority = ?7,
		   activity_id = CASE WHEN ?5 THEN ?6 ELSE activity_id END
		 WHERE id = ?4 AND kind <> 'scope_ack' AND (kind = 'task') = (?2 = 'task')`,
		j.Date.String(), string(j.Kind), j.Text, id, activity != nil, activityRef(deref(activity)), string(j.Priority))
	if err != nil {
		return fmt.Errorf("edit journal %d: %w", id, mapErr(err))
	}
	return s.readOnlyOrMissing(ctx, res, id)
}

func deref(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
}

// DeleteJournal removes a user-written entry. scope_ack entries are read-only.
func (s *Store) DeleteJournal(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM journal_entries WHERE id = ? AND kind <> 'scope_ack'`, id)
	if err != nil {
		return fmt.Errorf("delete journal %d: %w", id, err)
	}
	return s.readOnlyOrMissing(ctx, res, id)
}

// readOnlyOrMissing explains why a guarded write touched no rows.
func (s *Store) readOnlyOrMissing(ctx context.Context, res sql.Result, id int64) error {
	n, err := res.RowsAffected()
	if err != nil || n > 0 {
		return err
	}
	var kind string
	err = s.db.QueryRowContext(ctx, `SELECT kind FROM journal_entries WHERE id = ?`, id).Scan(&kind)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return fmt.Errorf("journal entry %d: %w", id, ErrNotFound)
	case err != nil:
		return err
	case kind == string(domain.KindScopeAck):
		return fmt.Errorf("journal entry %d: %w", id, ErrReadOnly)
	}
	return domain.Invalid("a task stays a task, and other entries cannot become tasks")
}
