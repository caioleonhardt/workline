package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"workline/internal/domain"
)

// taskColumns reads a journal entry of kind task as a domain.Task.
const taskColumns = `id, coalesce(activity_id, 0), coalesce(project_id, 0), text, date, closed_at, priority`

func scanTask(r scanner) (domain.Task, error) {
	var s domain.Task
	var created, priority string
	var closedAt sql.NullString
	if err := r.Scan(&s.ID, &s.ActivityID, &s.ProjectID, &s.Text, &created, &closedAt, &priority); err != nil {
		return s, err
	}
	s.ClosedAt, s.Priority = closedAt.String, domain.Priority(priority)
	var err error
	s.CreatedOn, err = parseDate(created)
	return s, err
}

func queryTasks(ctx context.Context, q queryer, query string, args ...any) ([]domain.Task, error) {
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	defer rows.Close()
	var out []domain.Task
	for rows.Next() {
		s, err := scanTask(rows)
		if err != nil {
			return nil, fmt.Errorf("scan task: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// AddTask logs an open task opened on createdOn, without a logged time.
func (s *Store) AddTask(ctx context.Context, activityID int64, text string, createdOn domain.Date) (domain.Task, error) {
	j, err := s.AddJournal(ctx, domain.JournalEntry{ActivityID: activityID, Date: createdOn, Kind: domain.KindTask, Text: text})
	return j.Task(), err
}

// ListTasks returns an activity's tasks in id order.
func (s *Store) ListTasks(ctx context.Context, activityID int64) ([]domain.Task, error) {
	return queryTasks(ctx, s.db,
		`SELECT `+taskColumns+` FROM journal_entries WHERE kind = 'task' AND activity_id = ? ORDER BY id`, activityID)
}

// ListAllTasks returns every task ordered by activity (Unassigned first), then id.
func (s *Store) ListAllTasks(ctx context.Context) ([]domain.Task, error) {
	return queryTasks(ctx, s.db, `SELECT `+taskColumns+` FROM journal_entries WHERE kind = 'task' ORDER BY activity_id, id`)
}

// ToggleTask closes an open task, recording the clock's date and time,
// or reopens a closed one, clearing it, and returns the result.
func (s *Store) ToggleTask(ctx context.Context, id int64, clock domain.Clock) (domain.Task, error) {
	if clock.Today().IsZero() {
		return domain.Task{}, domain.Invalid("today is required")
	}
	st, err := scanTask(s.db.QueryRowContext(ctx,
		`UPDATE journal_entries SET
		   closed_at = CASE WHEN closed_at IS NULL THEN ?1 ELSE NULL END
		 WHERE id = ?2 AND kind = 'task' RETURNING `+taskColumns, domain.Stamp(clock.Now()), id))
	if errors.Is(err, sql.ErrNoRows) {
		return st, fmt.Errorf("task %d: %w", id, ErrNotFound)
	}
	if err != nil {
		return st, fmt.Errorf("toggle task %d: %w", id, mapErr(err))
	}
	return st, nil
}
