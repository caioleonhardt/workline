package store

import (
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"fmt"

	"workline/internal/domain"
)

//go:embed seed/baseline.json
var baselineJSON []byte

// baseline mirrors the `data` and `as_of` parts of seed/baseline.json.
type baseline struct {
	AsOf string       `json:"as_of"`
	Data baselineData `json:"data"`
}

type baselineData struct {
	Projects []struct {
		ID       int64  `json:"id"`
		Name     string `json:"name"`
		Archived bool   `json:"archived"`
	} `json:"projects"`
	Activities []struct {
		ID          int64  `json:"id"`
		ProjectID   int64  `json:"project_id"`
		Name        string `json:"name"`
		RequestedBy string `json:"requested_by"`
		Status      string `json:"status"`
		StartDate   string `json:"start_date"`
		DueDate     string `json:"due_date"`
	} `json:"activities"`
	Tasks []struct {
		ID         int64  `json:"id"`
		ActivityID int64  `json:"activity_id"`
		Text       string `json:"text"`
		CreatedOn  string `json:"created_on"`
		ClosedAt   string `json:"closed_at"`
	} `json:"tasks"`
	JournalEntries []struct {
		ID         int64  `json:"id"`
		ActivityID int64  `json:"activity_id"`
		Date       string `json:"date"`
		Kind       string `json:"kind"`
		Text       string `json:"text"`
	} `json:"journal_entries"`
	ScopeAcks []struct {
		ID            int64  `json:"id"`
		ActivityID    int64  `json:"activity_id"`
		Date          string `json:"date"`
		BaselineCount int    `json:"baseline_count"`
		LastTaskID    int64  `json:"last_task_id"`
	} `json:"scope_acks"`
}

func loadBaseline() (baseline, error) {
	var b baseline
	if err := json.Unmarshal(baselineJSON, &b); err != nil {
		return b, fmt.Errorf("decode baseline.json: %w", err)
	}
	return b, nil
}

// Seed loads the mockup baseline (project, activity and task ids preserved) into an empty database, in
// one transaction. It returns ErrNotEmpty if any project already exists.
func (s *Store) Seed(ctx context.Context) error {
	b, err := loadBaseline()
	if err != nil {
		return err
	}
	asOf, err := domain.ParseDate(b.AsOf)
	if err != nil {
		return fmt.Errorf("seed: as_of: %w", err)
	}
	err = s.inTx(ctx, func(tx *sql.Tx) error {
		var n int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM projects`).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			return ErrNotEmpty
		}
		return insertBaseline(ctx, tx, b.Data, asOf)
	})
	if err != nil {
		return fmt.Errorf("seed: %w", err)
	}
	return nil
}

func insertBaseline(ctx context.Context, tx *sql.Tx, d baselineData, asOf domain.Date) error {
	for _, p := range d.Projects {
		if err := validProjectName(p.Name); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO projects (id, name, archived) VALUES (?, ?, ?)`,
			p.ID, p.Name, p.Archived); err != nil {
			return fmt.Errorf("project %d: %w", p.ID, mapErr(err))
		}
	}
	for _, a := range d.Activities {
		start, err := domain.ParseDate(a.StartDate)
		if err != nil {
			return err
		}
		due, err := domain.ParseDate(a.DueDate)
		if err != nil {
			return err
		}
		act := domain.Activity{ID: a.ID, ProjectID: a.ProjectID, Name: a.Name, RequestedBy: a.RequestedBy,
			Status: domain.Status(a.Status), Start: start, Due: due}
		if err := act.Validate(); err != nil {
			return fmt.Errorf("activity %d: %w", a.ID, err)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO activities (id, project_id, name, requested_by, status, start_date, due_date, done_on)
			 VALUES (?, ?, ?, ?, ?, ?, ?, CASE WHEN ? = 'done' THEN ? END)`,
			a.ID, a.ProjectID, a.Name, a.RequestedBy, a.Status, a.StartDate, a.DueDate, a.Status, a.DueDate); err != nil {
			return fmt.Errorf("activity %d: %w", a.ID, mapErr(err))
		}
	}
	for _, st := range d.Tasks {
		created, err := domain.ParseDate(st.CreatedOn)
		if err != nil {
			return err
		}
		sub := domain.Task{ID: st.ID, Text: st.Text, CreatedOn: created, ClosedAt: st.ClosedAt}
		if err := sub.ValidateAt(asOf); err != nil {
			return fmt.Errorf("task %d: %w", st.ID, err)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO journal_entries (id, activity_id, date, kind, text, closed_at) VALUES (?, ?, ?, 'task', ?, ?)`,
			st.ID, st.ActivityID, st.CreatedOn, st.Text, sql.NullString{String: st.ClosedAt, Valid: st.ClosedAt != ""}); err != nil {
			return fmt.Errorf("task %d: %w", st.ID, mapErr(err))
		}
	}
	for _, j := range d.JournalEntries {
		date, err := domain.ParseDate(j.Date)
		if err != nil {
			return err
		}
		if err := (domain.JournalEntry{Date: date, Kind: domain.Kind(j.Kind), Text: j.Text}).Validate(); err != nil {
			return fmt.Errorf("journal %d: %w", j.ID, err)
		}
		// Tasks keep their ids (scope acks refer to them); other entries follow.
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO journal_entries (activity_id, date, kind, text) VALUES (?, ?, ?, ?)`,
			j.ActivityID, j.Date, j.Kind, j.Text); err != nil {
			return fmt.Errorf("journal %d: %w", j.ID, mapErr(err))
		}
	}
	for _, a := range d.ScopeAcks {
		if _, err := domain.ParseDate(a.Date); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO scope_acks (id, activity_id, date, baseline_count, last_task_id) VALUES (?, ?, ?, ?, ?)`,
			a.ID, a.ActivityID, a.Date, a.BaselineCount, a.LastTaskID); err != nil {
			return fmt.Errorf("scope ack %d: %w", a.ID, mapErr(err))
		}
	}
	return nil
}
