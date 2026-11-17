package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jason-i-magno/career/internal/model"
)

// SaveDoc registers a doc, or points an existing name at a new path, and
// returns its ID. Names are unique, so re-adding a doc after moving its file
// keeps the ID and therefore the dashboard link.
func (s *Store) SaveDoc(ctx context.Context, d model.Doc, now time.Time) (int64, error) {
	if err := d.Validate(); err != nil {
		return 0, err
	}
	var id int64
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO docs (name, path, created_at) VALUES (?,?,?)
		 ON CONFLICT(name) DO UPDATE SET path = excluded.path
		 RETURNING id`,
		d.Name, d.Path, fmtTime(now)).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("saving doc %q: %w", d.Name, err)
	}
	return id, nil
}

// GetDoc returns the doc with the given ID.
func (s *Store) GetDoc(ctx context.Context, id int64) (model.Doc, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, name, path, created_at FROM docs WHERE id = ?`, id)
	d, err := scanDoc(row)
	if errors.Is(err, sql.ErrNoRows) {
		return model.Doc{}, fmt.Errorf("doc %d: %w", id, ErrNotFound)
	}
	if err != nil {
		return model.Doc{}, fmt.Errorf("reading doc %d: %w", id, err)
	}
	return d, nil
}

// ListDocs returns every registered doc, by name.
func (s *Store) ListDocs(ctx context.Context) ([]model.Doc, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, path, created_at FROM docs ORDER BY name COLLATE NOCASE, id`)
	if err != nil {
		return nil, fmt.Errorf("listing docs: %w", err)
	}
	defer rows.Close()

	var docs []model.Doc
	for rows.Next() {
		d, err := scanDoc(rows)
		if err != nil {
			return nil, fmt.Errorf("reading doc row: %w", err)
		}
		docs = append(docs, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("listing docs: %w", err)
	}
	return docs, nil
}

// DeleteDoc unregisters the doc with the given name. The file is untouched.
func (s *Store) DeleteDoc(ctx context.Context, name string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM docs WHERE name = ?`, name)
	if err != nil {
		return fmt.Errorf("deleting doc %q: %w", name, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("doc %q: %w", name, ErrNotFound)
	}
	return nil
}

func scanDoc(r scanner) (model.Doc, error) {
	var (
		d       model.Doc
		created string
	)
	if err := r.Scan(&d.ID, &d.Name, &d.Path, &created); err != nil {
		return model.Doc{}, err
	}
	t, err := parseTime(created)
	if err != nil {
		return model.Doc{}, err
	}
	d.CreatedAt = t
	return d, nil
}
