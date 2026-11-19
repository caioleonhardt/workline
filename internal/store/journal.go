package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"workline/internal/domain"
)

func insertJournal(ctx context.Context, q queryer, j domain.JournalEntry) (int64, error) {
	if err := j.Validate(); err != nil {
		return 0, err
	}
	res, err := q.ExecContext(ctx,
		`INSERT INTO journal_entries (activity_id, date, kind, text, logged_at) VALUES (?, ?, ?, ?, ?)`,
		j.ActivityID, j.Date.String(), string(j.Kind), j.Text, sql.NullString{String: j.LoggedAt, Valid: j.LoggedAt != ""})
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

// AddJournal adds a user-written entry; the caller stamps LoggedAt. Kind scope_ack is refused.
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
		var date, kind string
		var loggedAt sql.NullString
		if err := rows.Scan(&j.ID, &j.ActivityID, &date, &kind, &j.Text, &loggedAt); err != nil {
			return nil, fmt.Errorf("scan journal: %w", err)
		}
		j.Kind, j.LoggedAt = domain.Kind(kind), loggedAt.String
		if j.Date, err = parseDate(date); err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

// ListJournal returns an activity's entries, newest first.
func (s *Store) ListJournal(ctx context.Context, activityID int64) ([]domain.JournalEntry, error) {
	return s.queryJournal(ctx,
		`SELECT id, activity_id, date, kind, text, logged_at FROM journal_entries
		 WHERE activity_id = ? ORDER BY date DESC, logged_at IS NULL, logged_at DESC, id DESC`, activityID)
}

// ListJournalFeed returns up to limit entries of the given activities, newest
// first (date, then logged time with untimed entries last, then id), and
// whether more exist beyond the limit.
func (s *Store) ListJournalFeed(ctx context.Context, activityIDs []int64, limit int) ([]domain.JournalEntry, bool, error) {
	if len(activityIDs) == 0 || limit <= 0 {
		return nil, false, nil
	}
	args := make([]any, 0, len(activityIDs)+1)
	for _, id := range activityIDs {
		args = append(args, id)
	}
	args = append(args, limit+1)
	entries, err := s.queryJournal(ctx,
		`SELECT id, activity_id, date, kind, text, logged_at FROM journal_entries
		 WHERE activity_id IN (?`+strings.Repeat(",?", len(activityIDs)-1)+`)
		 ORDER BY date DESC, logged_at IS NULL, logged_at DESC, id DESC LIMIT ?`, args...)
	if err != nil {
		return nil, false, err
	}
	if len(entries) > limit {
		return entries[:limit], true, nil
	}
	return entries, false, nil
}

// ListJournalBetween returns the entries of all activities whose date lies in
// from..to inclusive (a zero bound is open), oldest first: by date, then
// logged time with untimed entries first, then id.
func (s *Store) ListJournalBetween(ctx context.Context, from, to domain.Date) ([]domain.JournalEntry, error) {
	return s.queryJournal(ctx,
		`SELECT id, activity_id, date, kind, text, logged_at FROM journal_entries
		 WHERE (?1 = '' OR date >= ?1) AND (?2 = '' OR date <= ?2)
		 ORDER BY date, logged_at IS NOT NULL, logged_at, id`, from.String(), to.String())
}

// EditJournal rewrites date, kind and text. scope_ack entries are read-only.
func (s *Store) EditJournal(ctx context.Context, id int64, date domain.Date, kind domain.Kind, text string) error {
	j := domain.JournalEntry{ActivityID: id, Date: date, Kind: kind, Text: text}
	if err := userKind(kind); err != nil {
		return err
	}
	if err := j.Validate(); err != nil {
		return err
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE journal_entries SET date = ?, kind = ?, text = ? WHERE id = ? AND kind <> 'scope_ack'`,
		date.String(), string(kind), text, id)
	if err != nil {
		return fmt.Errorf("edit journal %d: %w", id, mapErr(err))
	}
	return s.readOnlyOrMissing(ctx, res, id)
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
	var one int
	err = s.db.QueryRowContext(ctx, `SELECT 1 FROM journal_entries WHERE id = ?`, id).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("journal entry %d: %w", id, ErrNotFound)
	}
	if err != nil {
		return err
	}
	return fmt.Errorf("journal entry %d: %w", id, ErrReadOnly)
}
