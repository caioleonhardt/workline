package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"workline/internal/domain"
)

func validProjectName(name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("%w: project name is required", domain.ErrInvalid)
	}
	return nil
}

// CreateProject inserts a project.
func (s *Store) CreateProject(ctx context.Context, name string) (domain.Project, error) {
	return insertProject(ctx, s.db, name)
}

func insertProject(ctx context.Context, q queryer, name string) (domain.Project, error) {
	name = strings.TrimSpace(name)
	if err := validProjectName(name); err != nil {
		return domain.Project{}, err
	}
	res, err := q.ExecContext(ctx, `INSERT INTO projects (name) VALUES (?)`, name)
	if err != nil {
		return domain.Project{}, fmt.Errorf("create project %q: %w", name, mapErr(err))
	}
	id, err := res.LastInsertId()
	if err != nil {
		return domain.Project{}, err
	}
	return domain.Project{ID: id, Name: name}, nil
}

// ListProjects returns projects ordered by name.
func (s *Store) ListProjects(ctx context.Context, includeArchived bool) ([]domain.Project, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, archived FROM projects WHERE archived = 0 OR ? ORDER BY name COLLATE NOCASE, id`,
		includeArchived)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	defer rows.Close()
	var out []domain.Project
	for rows.Next() {
		var p domain.Project
		if err := rows.Scan(&p.ID, &p.Name, &p.Archived); err != nil {
			return nil, fmt.Errorf("scan project: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// GetProject returns one project.
func (s *Store) GetProject(ctx context.Context, id int64) (domain.Project, error) {
	var p domain.Project
	err := s.db.QueryRowContext(ctx, `SELECT id, name, archived FROM projects WHERE id = ?`, id).
		Scan(&p.ID, &p.Name, &p.Archived)
	if errors.Is(err, sql.ErrNoRows) {
		return p, fmt.Errorf("project %d: %w", id, ErrNotFound)
	}
	if err != nil {
		return p, fmt.Errorf("get project %d: %w", id, err)
	}
	return p, nil
}

// RenameProject changes a project's name.
func (s *Store) RenameProject(ctx context.Context, id int64, name string) error {
	name = strings.TrimSpace(name)
	if err := validProjectName(name); err != nil {
		return err
	}
	res, err := s.db.ExecContext(ctx, `UPDATE projects SET name = ? WHERE id = ?`, name, id)
	if err != nil {
		return fmt.Errorf("rename project %d: %w", id, mapErr(err))
	}
	return affected(res, "project", id)
}

// SetProjectArchived archives or restores a project.
func (s *Store) SetProjectArchived(ctx context.Context, id int64, archived bool) error {
	res, err := s.db.ExecContext(ctx, `UPDATE projects SET archived = ? WHERE id = ?`, archived, id)
	if err != nil {
		return fmt.Errorf("archive project %d: %w", id, err)
	}
	return affected(res, "project", id)
}

// DeleteProject removes a project and, by cascade, everything under it.
func (s *Store) DeleteProject(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM projects WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete project %d: %w", id, err)
	}
	return affected(res, "project", id)
}
