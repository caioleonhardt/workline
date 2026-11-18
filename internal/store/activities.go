package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"workline/internal/domain"
)

// NewActivity is the input of CreateActivity. Exactly one of ProjectID and
// NewProjectName must be set.
type NewActivity struct {
	ProjectID      int64
	NewProjectName string
	Name           string
	RequestedBy    string
	Priority       domain.Priority
	Start, Due     domain.Date
}

const activityColumns = `id, project_id, name, requested_by, status, priority, start_date, due_date, done_on`

type scanner interface{ Scan(dest ...any) error }

func scanActivity(r scanner) (domain.Activity, error) {
	var a domain.Activity
	var status, start, due string
	var priority, doneOn sql.NullString
	if err := r.Scan(&a.ID, &a.ProjectID, &a.Name, &a.RequestedBy, &status, &priority, &start, &due, &doneOn); err != nil {
		return a, err
	}
	a.Status = domain.Status(status)
	a.Priority = domain.Priority(priority.String)
	var err error
	if a.DoneOn, err = parseOptDate(doneOn); err != nil {
		return a, err
	}
	if a.Start, err = parseDate(start); err != nil {
		return a, err
	}
	if a.Due, err = parseDate(due); err != nil {
		return a, err
	}
	return a, nil
}

// CreateActivity creates the activity (and the project, if NewProjectName is
// set) plus the "Activity created" journal entry, in one transaction.
func (s *Store) CreateActivity(ctx context.Context, in NewActivity, today domain.Date) (domain.Activity, error) {
	if (in.ProjectID == 0) == (in.NewProjectName == "") {
		return domain.Activity{}, fmt.Errorf("%w: set exactly one of project id and new project name", domain.ErrInvalid)
	}
	a := domain.Activity{
		ProjectID: in.ProjectID, Name: in.Name, RequestedBy: in.RequestedBy, Priority: in.Priority,
		Status: domain.DefaultStatus(in.Start, today), Start: in.Start, Due: in.Due,
	}
	if err := a.Validate(); err != nil {
		return a, err
	}
	err := s.inTx(ctx, func(tx *sql.Tx) error {
		if in.NewProjectName != "" {
			p, err := insertProject(ctx, tx, in.NewProjectName)
			if err != nil {
				return err
			}
			a.ProjectID = p.ID
		}
		id, err := insertActivity(ctx, tx, a)
		if err != nil {
			return err
		}
		a.ID = id
		_, err = insertJournal(ctx, tx, domain.JournalEntry{
			ActivityID: id, Date: today, Kind: domain.KindUpdate, Text: "Activity created",
		})
		return err
	})
	if err != nil {
		return domain.Activity{}, fmt.Errorf("create activity %q: %w", in.Name, err)
	}
	return a, nil
}

func insertActivity(ctx context.Context, q queryer, a domain.Activity) (int64, error) {
	res, err := q.ExecContext(ctx,
		`INSERT INTO activities (project_id, name, requested_by, status, priority, start_date, due_date)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		a.ProjectID, a.Name, a.RequestedBy, string(a.Status), nullPriority(a.Priority), a.Start.String(), a.Due.String())
	if err != nil {
		return 0, mapErr(err)
	}
	return res.LastInsertId()
}

// GetActivity returns one activity.
func (s *Store) GetActivity(ctx context.Context, id int64) (domain.Activity, error) {
	return getActivity(ctx, s.db, id)
}

func getActivity(ctx context.Context, q queryer, id int64) (domain.Activity, error) {
	a, err := scanActivity(q.QueryRowContext(ctx, `SELECT `+activityColumns+` FROM activities WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return a, fmt.Errorf("activity %d: %w", id, ErrNotFound)
	}
	if err != nil {
		return a, fmt.Errorf("get activity %d: %w", id, err)
	}
	return a, nil
}

// ListActivities returns all activities ordered by project, start date, id.
func (s *Store) ListActivities(ctx context.Context) ([]domain.Activity, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+activityColumns+` FROM activities ORDER BY project_id, start_date, id`)
	if err != nil {
		return nil, fmt.Errorf("list activities: %w", err)
	}
	defer rows.Close()
	var out []domain.Activity
	for rows.Next() {
		a, err := scanActivity(rows)
		if err != nil {
			return nil, fmt.Errorf("scan activity: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// UpdateActivity rewrites project, name, requester, priority and dates (not status).
func (s *Store) UpdateActivity(ctx context.Context, a domain.Activity) error {
	if err := a.Validate(); err != nil {
		return err
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE activities SET project_id = ?, name = ?, requested_by = ?, priority = ?, start_date = ?, due_date = ? WHERE id = ?`,
		a.ProjectID, a.Name, a.RequestedBy, nullPriority(a.Priority), a.Start.String(), a.Due.String(), a.ID)
	if err != nil {
		return fmt.Errorf("update activity %d: %w", a.ID, mapErr(err))
	}
	return affected(res, "activity", a.ID)
}

// SetActivityStatus changes the status. Entering done records today as the
// done date (kept if already done); leaving done clears it (SPEC §10 item 21).
func (s *Store) SetActivityStatus(ctx context.Context, id int64, status domain.Status, today domain.Date) error {
	if !status.Valid() {
		return fmt.Errorf("%w: unknown status %q", domain.ErrInvalid, status)
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE activities SET status = ?1,
		   done_on = CASE WHEN ?1 = 'done' THEN COALESCE(done_on, ?2) ELSE NULL END
		 WHERE id = ?3`, string(status), today.String(), id)
	if err != nil {
		return fmt.Errorf("set status of activity %d: %w", id, err)
	}
	return affected(res, "activity", id)
}

// DeleteActivity removes an activity and, by cascade, its subtasks, journal and acks.
func (s *Store) DeleteActivity(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM activities WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete activity %d: %w", id, err)
	}
	return affected(res, "activity", id)
}
