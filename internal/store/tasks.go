package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"workline/internal/domain"
)

const taskColumns = `id, activity_id, text, created_on, closed_on, closed_at`

func scanTask(r scanner) (domain.Task, error) {
	var s domain.Task
	var created string
	var closed, closedAt sql.NullString
	if err := r.Scan(&s.ID, &s.ActivityID, &s.Text, &created, &closed, &closedAt); err != nil {
		return s, err
	}
	s.ClosedAt = closedAt.String
	var err error
	if s.CreatedOn, err = parseDate(created); err != nil {
		return s, err
	}
	s.ClosedOn, err = parseOptDate(closed)
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

func insertTask(ctx context.Context, q queryer, st domain.Task) (int64, error) {
	if err := st.Validate(); err != nil {
		return 0, err
	}
	res, err := q.ExecContext(ctx,
		`INSERT INTO subtasks (activity_id, text, created_on) VALUES (?, ?, ?)`,
		st.ActivityID, st.Text, st.CreatedOn.String())
	if err != nil {
		return 0, mapErr(err)
	}
	return res.LastInsertId()
}

// AddTask adds an open task created on createdOn.
func (s *Store) AddTask(ctx context.Context, activityID int64, text string, createdOn domain.Date) (domain.Task, error) {
	st := domain.Task{ActivityID: activityID, Text: text, CreatedOn: createdOn}
	id, err := insertTask(ctx, s.db, st)
	if err != nil {
		return st, fmt.Errorf("add task to activity %d: %w", activityID, err)
	}
	st.ID = id
	return st, nil
}

// ListTasks returns an activity's tasks in id order.
func (s *Store) ListTasks(ctx context.Context, activityID int64) ([]domain.Task, error) {
	return queryTasks(ctx, s.db,
		`SELECT `+taskColumns+` FROM subtasks WHERE activity_id = ? ORDER BY id`, activityID)
}

// ListAllTasks returns every task ordered by activity, then id.
func (s *Store) ListAllTasks(ctx context.Context) ([]domain.Task, error) {
	return queryTasks(ctx, s.db, `SELECT `+taskColumns+` FROM subtasks ORDER BY activity_id, id`)
}

// ToggleTask closes an open task, recording the clock's date and time,
// or reopens a closed one, clearing both, and returns the result.
func (s *Store) ToggleTask(ctx context.Context, id int64, clock domain.Clock) (domain.Task, error) {
	today := clock.Today()
	if today.IsZero() {
		return domain.Task{}, domain.Invalid("today is required")
	}
	st, err := scanTask(s.db.QueryRowContext(ctx,
		`UPDATE subtasks SET
		   closed_on = CASE WHEN closed_on IS NULL THEN ?1 ELSE NULL END,
		   closed_at = CASE WHEN closed_on IS NULL THEN ?2 ELSE NULL END
		 WHERE id = ?3 RETURNING `+taskColumns, today.String(), domain.Stamp(clock.Now()), id))
	if errors.Is(err, sql.ErrNoRows) {
		return st, fmt.Errorf("task %d: %w", id, ErrNotFound)
	}
	if err != nil {
		return st, fmt.Errorf("toggle task %d: %w", id, mapErr(err))
	}
	return st, nil
}

// EditTask changes a task's text and creation date. The database rejects
// a creation date after the existing closed date.
func (s *Store) EditTask(ctx context.Context, id int64, text string, createdOn domain.Date) error {
	if err := (domain.Task{Text: text, CreatedOn: createdOn}).Validate(); err != nil {
		return err
	}
	res, err := s.db.ExecContext(ctx, `UPDATE subtasks SET text = ?, created_on = ? WHERE id = ?`,
		text, createdOn.String(), id)
	if err != nil {
		return fmt.Errorf("edit task %d: %w", id, mapErr(err))
	}
	return affected(res, "task", id)
}

// DeleteTask removes a task.
func (s *Store) DeleteTask(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM subtasks WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete task %d: %w", id, err)
	}
	return affected(res, "task", id)
}
