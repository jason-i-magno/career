package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jason-i-magno/career/internal/model"
)

const appColumns = `id, company, role, stage, track, source, location, remote, url,
	contact, base_comp, created_at, updated_at, next_action, next_action_at`

// CreateApplication inserts a and returns its assigned ID. CreatedAt and
// UpdatedAt are set to now if the caller left them zero.
func (s *Store) CreateApplication(ctx context.Context, a model.Application, now time.Time) (int64, error) {
	if a.CreatedAt.IsZero() {
		a.CreatedAt = now
	}
	if a.UpdatedAt.IsZero() {
		a.UpdatedAt = now
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO applications
		 (company, role, stage, track, source, location, remote, url, contact,
		  base_comp, created_at, updated_at, next_action, next_action_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		a.Company, a.Role, string(a.Stage), string(a.Track), string(a.Source),
		a.Location, a.Remote, a.URL, a.Contact, a.BaseComp,
		fmtTime(a.CreatedAt), fmtTime(a.UpdatedAt), a.NextAction, fmtTimePtr(a.NextActionAt))
	if err != nil {
		return 0, fmt.Errorf("inserting application: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("reading inserted application id: %w", err)
	}
	return id, nil
}

// GetApplication returns the application with the given ID, or ErrNotFound.
func (s *Store) GetApplication(ctx context.Context, id int64) (model.Application, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+appColumns+` FROM applications WHERE id = ?`, id)
	a, err := scanApplication(row)
	if errors.Is(err, sql.ErrNoRows) {
		return model.Application{}, fmt.Errorf("application %d: %w", id, ErrNotFound)
	}
	return a, err
}

// ApplicationFilter narrows a listing. Zero values mean "no constraint".
type ApplicationFilter struct {
	Stages     []model.Stage
	Track      model.Track
	ActiveOnly bool
	Company    string // case-insensitive substring match
}

// ListApplications returns applications matching f, ordered by pipeline stage
// then most recently updated.
func (s *Store) ListApplications(ctx context.Context, f ApplicationFilter) ([]model.Application, error) {
	var where []string
	var args []any

	if len(f.Stages) > 0 {
		ph := make([]string, len(f.Stages))
		for i, st := range f.Stages {
			ph[i] = "?"
			args = append(args, string(st))
		}
		where = append(where, `stage IN (`+strings.Join(ph, ",")+`)`)
	}
	if f.ActiveOnly {
		where = append(where, `stage NOT IN (?, ?)`)
		args = append(args, string(model.StageRejected), string(model.StageWithdrawn))
	}
	if f.Track != "" {
		where = append(where, `track = ?`)
		args = append(args, string(f.Track))
	}
	if f.Company != "" {
		where = append(where, `LOWER(company) LIKE ?`)
		args = append(args, "%"+strings.ToLower(f.Company)+"%")
	}

	q := `SELECT ` + appColumns + ` FROM applications`
	if len(where) > 0 {
		q += ` WHERE ` + strings.Join(where, " AND ")
	}
	q += ` ORDER BY updated_at DESC`

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("listing applications: %w", err)
	}
	defer rows.Close()

	var out []model.Application
	for rows.Next() {
		a, err := scanApplication(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("listing applications: %w", err)
	}
	// Sort by pipeline progress in Go rather than SQL: stage order is domain
	// knowledge, not a lexical property of the stored strings.
	stableSortByStage(out)
	return out, nil
}

// UpdateApplication writes every mutable field of a and stamps UpdatedAt.
func (s *Store) UpdateApplication(ctx context.Context, a model.Application, now time.Time) error {
	a.UpdatedAt = now
	res, err := s.db.ExecContext(ctx,
		`UPDATE applications SET company=?, role=?, stage=?, track=?, source=?,
		 location=?, remote=?, url=?, contact=?, base_comp=?, updated_at=?,
		 next_action=?, next_action_at=? WHERE id=?`,
		a.Company, a.Role, string(a.Stage), string(a.Track), string(a.Source),
		a.Location, a.Remote, a.URL, a.Contact, a.BaseComp, fmtTime(a.UpdatedAt),
		a.NextAction, fmtTimePtr(a.NextActionAt), a.ID)
	if err != nil {
		return fmt.Errorf("updating application %d: %w", a.ID, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("updating application %d: %w", a.ID, err)
	}
	if n == 0 {
		return fmt.Errorf("application %d: %w", a.ID, ErrNotFound)
	}
	return nil
}

// DeleteApplication removes an application and, by cascade, its events.
func (s *Store) DeleteApplication(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM applications WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("deleting application %d: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("application %d: %w", id, ErrNotFound)
	}
	return nil
}

// AddEvent appends an entry to an application's activity log.
func (s *Store) AddEvent(ctx context.Context, e model.Event) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO events (application_id, at, kind, body) VALUES (?,?,?,?)`,
		e.ApplicationID, fmtTime(e.At), string(e.Kind), e.Body)
	if err != nil {
		return 0, fmt.Errorf("inserting event: %w", err)
	}
	return res.LastInsertId()
}

// ListEvents returns an application's log, oldest first.
func (s *Store) ListEvents(ctx context.Context, applicationID int64) ([]model.Event, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, application_id, at, kind, body FROM events
		 WHERE application_id = ? ORDER BY at ASC, id ASC`, applicationID)
	if err != nil {
		return nil, fmt.Errorf("listing events: %w", err)
	}
	defer rows.Close()

	var out []model.Event
	for rows.Next() {
		var e model.Event
		var at, kind string
		if err := rows.Scan(&e.ID, &e.ApplicationID, &at, &kind, &e.Body); err != nil {
			return nil, fmt.Errorf("scanning event: %w", err)
		}
		t, err := parseTime(at)
		if err != nil {
			return nil, err
		}
		e.At, e.Kind = t, model.EventKind(kind)
		out = append(out, e)
	}
	return out, rows.Err()
}

// scanner abstracts *sql.Row and *sql.Rows so one scan helper serves both.
type scanner interface{ Scan(dest ...any) error }

func scanApplication(sc scanner) (model.Application, error) {
	var (
		a                    model.Application
		stage, track, source string
		createdAt, updatedAt string
		nextActionAt         sql.NullString
	)
	err := sc.Scan(&a.ID, &a.Company, &a.Role, &stage, &track, &source,
		&a.Location, &a.Remote, &a.URL, &a.Contact, &a.BaseComp,
		&createdAt, &updatedAt, &a.NextAction, &nextActionAt)
	if err != nil {
		return model.Application{}, err
	}
	a.Stage, a.Track, a.Source = model.Stage(stage), model.Track(track), model.Source(source)
	if a.CreatedAt, err = parseTime(createdAt); err != nil {
		return model.Application{}, err
	}
	if a.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return model.Application{}, err
	}
	if a.NextActionAt, err = parseTimePtr(nextActionAt); err != nil {
		return model.Application{}, err
	}
	return a, nil
}
