package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"workline/internal/domain"
)

const subtaskColumns = `id, activity_id, text, created_on, closed_on, closed_at`

func scanSubtask(r scanner) (domain.Subtask, error) {
	var s domain.Subtask
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

func querySubtasks(ctx context.Context, q queryer, query string, args ...any) ([]domain.Subtask, error) {
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list subtasks: %w", err)
	}
	defer rows.Close()
	var out []domain.Subtask
	for rows.Next() {
		s, err := scanSubtask(rows)
		if err != nil {
			return nil, fmt.Errorf("scan subtask: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func insertSubtask(ctx context.Context, q queryer, st domain.Subtask) (int64, error) {
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

// AddSubtask adds an open subtask created on createdOn.
func (s *Store) AddSubtask(ctx context.Context, activityID int64, text string, createdOn domain.Date) (domain.Subtask, error) {
	st := domain.Subtask{ActivityID: activityID, Text: text, CreatedOn: createdOn}
	id, err := insertSubtask(ctx, s.db, st)
	if err != nil {
		return st, fmt.Errorf("add subtask to activity %d: %w", activityID, err)
	}
	st.ID = id
	return st, nil
}

// ListSubtasks returns an activity's subtasks in id order.
func (s *Store) ListSubtasks(ctx context.Context, activityID int64) ([]domain.Subtask, error) {
	return querySubtasks(ctx, s.db,
		`SELECT `+subtaskColumns+` FROM subtasks WHERE activity_id = ? ORDER BY id`, activityID)
}

// ListAllSubtasks returns every subtask ordered by activity, then id.
func (s *Store) ListAllSubtasks(ctx context.Context) ([]domain.Subtask, error) {
	return querySubtasks(ctx, s.db, `SELECT `+subtaskColumns+` FROM subtasks ORDER BY activity_id, id`)
}

// ToggleSubtask closes an open subtask, recording the clock's date and time,
// or reopens a closed one, clearing both, and returns the result.
func (s *Store) ToggleSubtask(ctx context.Context, id int64, clock domain.Clock) (domain.Subtask, error) {
	today := clock.Today()
	if today.IsZero() {
		return domain.Subtask{}, domain.Invalid("today is required")
	}
	st, err := scanSubtask(s.db.QueryRowContext(ctx,
		`UPDATE subtasks SET
		   closed_on = CASE WHEN closed_on IS NULL THEN ?1 ELSE NULL END,
		   closed_at = CASE WHEN closed_on IS NULL THEN ?2 ELSE NULL END
		 WHERE id = ?3 RETURNING `+subtaskColumns, today.String(), domain.Stamp(clock.Now()), id))
	if errors.Is(err, sql.ErrNoRows) {
		return st, fmt.Errorf("subtask %d: %w", id, ErrNotFound)
	}
	if err != nil {
		return st, fmt.Errorf("toggle subtask %d: %w", id, mapErr(err))
	}
	return st, nil
}

// EditSubtask changes a subtask's text and creation date. The database rejects
// a creation date after the existing closed date.
func (s *Store) EditSubtask(ctx context.Context, id int64, text string, createdOn domain.Date) error {
	if err := (domain.Subtask{Text: text, CreatedOn: createdOn}).Validate(); err != nil {
		return err
	}
	res, err := s.db.ExecContext(ctx, `UPDATE subtasks SET text = ?, created_on = ? WHERE id = ?`,
		text, createdOn.String(), id)
	if err != nil {
		return fmt.Errorf("edit subtask %d: %w", id, mapErr(err))
	}
	return affected(res, "subtask", id)
}

// DeleteSubtask removes a subtask.
func (s *Store) DeleteSubtask(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM subtasks WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete subtask %d: %w", id, err)
	}
	return affected(res, "subtask", id)
}
