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

const prepColumns = `id, subject, prompt, ref, answer, ease, interval_days, reps,
	lapses, due_at, last_grade, reviewed_at, created_at`

// CreatePrepItem inserts a prep item.
func (s *Store) CreatePrepItem(ctx context.Context, p model.PrepItem, now time.Time) (int64, error) {
	if p.CreatedAt.IsZero() {
		p.CreatedAt = now
	}
	if p.DueAt.IsZero() {
		p.DueAt = now
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO prep_items (subject, prompt, ref, answer, ease, interval_days, reps,
		 lapses, due_at, last_grade, reviewed_at, created_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		string(p.Subject), p.Prompt, p.Ref, p.Answer, p.Ease, p.IntervalDays, p.Reps,
		p.Lapses, fmtTime(p.DueAt), gradePtr(p.LastGrade), fmtTimePtr(p.ReviewedAt),
		fmtTime(p.CreatedAt))
	if err != nil {
		return 0, fmt.Errorf("inserting prep item: %w", err)
	}
	return res.LastInsertId()
}

// GetPrepItem returns one prep item by ID.
func (s *Store) GetPrepItem(ctx context.Context, id int64) (model.PrepItem, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+prepColumns+` FROM prep_items WHERE id = ?`, id)
	p, err := scanPrepItem(row)
	if errors.Is(err, sql.ErrNoRows) {
		return model.PrepItem{}, fmt.Errorf("prep item %d: %w", id, ErrNotFound)
	}
	return p, err
}

// PrepFilter narrows a prep listing.
type PrepFilter struct {
	Subject model.Subject
	DueBy   *time.Time // only items due at or before this instant
	Limit   int
}

// ListPrepItems returns items matching f, soonest-due first.
func (s *Store) ListPrepItems(ctx context.Context, f PrepFilter) ([]model.PrepItem, error) {
	var where []string
	var args []any

	if f.Subject != "" {
		where = append(where, `subject = ?`)
		args = append(args, string(f.Subject))
	}
	if f.DueBy != nil {
		where = append(where, `due_at <= ?`)
		args = append(args, fmtTime(*f.DueBy))
	}

	q := `SELECT ` + prepColumns + ` FROM prep_items`
	if len(where) > 0 {
		q += ` WHERE ` + strings.Join(where, " AND ")
	}
	q += ` ORDER BY due_at ASC, id ASC`
	if f.Limit > 0 {
		q += fmt.Sprintf(` LIMIT %d`, f.Limit)
	}

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("listing prep items: %w", err)
	}
	defer rows.Close()

	var out []model.PrepItem
	for rows.Next() {
		p, err := scanPrepItem(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// UpdatePrepItem persists scheduler state after a review.
func (s *Store) UpdatePrepItem(ctx context.Context, p model.PrepItem) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE prep_items SET subject=?, prompt=?, ref=?, answer=?, ease=?, interval_days=?,
		 reps=?, lapses=?, due_at=?, last_grade=?, reviewed_at=? WHERE id=?`,
		string(p.Subject), p.Prompt, p.Ref, p.Answer, p.Ease, p.IntervalDays, p.Reps,
		p.Lapses, fmtTime(p.DueAt), gradePtr(p.LastGrade), fmtTimePtr(p.ReviewedAt), p.ID)
	if err != nil {
		return fmt.Errorf("updating prep item %d: %w", p.ID, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("prep item %d: %w", p.ID, ErrNotFound)
	}
	return nil
}

// DeletePrepItem removes a prep item.
func (s *Store) DeletePrepItem(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM prep_items WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("deleting prep item %d: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("prep item %d: %w", id, ErrNotFound)
	}
	return nil
}

// CountPrepBySubject returns due and total counts per subject.
func (s *Store) CountPrepBySubject(ctx context.Context, now time.Time) (due, total map[model.Subject]int, err error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT subject, SUM(CASE WHEN due_at <= ? THEN 1 ELSE 0 END), COUNT(*)
		 FROM prep_items GROUP BY subject`, fmtTime(now))
	if err != nil {
		return nil, nil, fmt.Errorf("counting prep items: %w", err)
	}
	defer rows.Close()

	due, total = map[model.Subject]int{}, map[model.Subject]int{}
	for rows.Next() {
		var subj string
		var d, t int
		if err := rows.Scan(&subj, &d, &t); err != nil {
			return nil, nil, fmt.Errorf("scanning prep counts: %w", err)
		}
		due[model.Subject(subj)], total[model.Subject(subj)] = d, t
	}
	return due, total, rows.Err()
}

func gradePtr(g *model.Grade) any {
	if g == nil {
		return nil
	}
	return int(*g)
}

func scanPrepItem(sc scanner) (model.PrepItem, error) {
	var (
		p          model.PrepItem
		subject    string
		dueAt      string
		createdAt  string
		lastGrade  sql.NullInt64
		reviewedAt sql.NullString
	)
	err := sc.Scan(&p.ID, &subject, &p.Prompt, &p.Ref, &p.Answer, &p.Ease, &p.IntervalDays,
		&p.Reps, &p.Lapses, &dueAt, &lastGrade, &reviewedAt, &createdAt)
	if err != nil {
		return model.PrepItem{}, err
	}
	p.Subject = model.Subject(subject)
	if p.DueAt, err = parseTime(dueAt); err != nil {
		return model.PrepItem{}, err
	}
	if p.CreatedAt, err = parseTime(createdAt); err != nil {
		return model.PrepItem{}, err
	}
	if p.ReviewedAt, err = parseTimePtr(reviewedAt); err != nil {
		return model.PrepItem{}, err
	}
	if lastGrade.Valid {
		g := model.Grade(lastGrade.Int64)
		p.LastGrade = &g
	}
	return p, nil
}
