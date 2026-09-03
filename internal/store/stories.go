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

const storyColumns = `id, title, situation, task, action, result, metrics,
	sanitized, created_at, updated_at`

// CreateStory inserts a story and its competency tags in one transaction.
func (s *Store) CreateStory(ctx context.Context, st model.Story, now time.Time) (int64, error) {
	if st.CreatedAt.IsZero() {
		st.CreatedAt = now
	}
	st.UpdatedAt = now

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("beginning story insert: %w", err)
	}
	defer tx.Rollback() // no-op once Commit succeeds

	res, err := tx.ExecContext(ctx,
		`INSERT INTO stories (title, situation, task, action, result, metrics,
		 sanitized, created_at, updated_at) VALUES (?,?,?,?,?,?,?,?,?)`,
		st.Title, st.Situation, st.Task, st.Action, st.Result, st.Metrics,
		st.Sanitized, fmtTime(st.CreatedAt), fmtTime(st.UpdatedAt))
	if err != nil {
		return 0, fmt.Errorf("inserting story: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("reading inserted story id: %w", err)
	}
	if err := replaceCompetencies(ctx, tx, id, st.Competencies); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("committing story insert: %w", err)
	}
	return id, nil
}

// UpdateStory rewrites a story and replaces its competency tag set.
func (s *Store) UpdateStory(ctx context.Context, st model.Story, now time.Time) error {
	st.UpdatedAt = now

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning story update: %w", err)
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx,
		`UPDATE stories SET title=?, situation=?, task=?, action=?, result=?,
		 metrics=?, sanitized=?, updated_at=? WHERE id=?`,
		st.Title, st.Situation, st.Task, st.Action, st.Result, st.Metrics,
		st.Sanitized, fmtTime(st.UpdatedAt), st.ID)
	if err != nil {
		return fmt.Errorf("updating story %d: %w", st.ID, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("story %d: %w", st.ID, ErrNotFound)
	}
	if err := replaceCompetencies(ctx, tx, st.ID, st.Competencies); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing story update: %w", err)
	}
	return nil
}

func replaceCompetencies(ctx context.Context, tx *sql.Tx, storyID int64, cs []model.Competency) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM story_competencies WHERE story_id = ?`, storyID); err != nil {
		return fmt.Errorf("clearing competencies for story %d: %w", storyID, err)
	}
	for _, c := range model.SortCompetencies(cs) {
		if _, err := tx.ExecContext(ctx,
			`INSERT OR IGNORE INTO story_competencies (story_id, competency) VALUES (?,?)`,
			storyID, string(c)); err != nil {
			return fmt.Errorf("tagging story %d with %q: %w", storyID, c, err)
		}
	}
	return nil
}

// GetStory returns one story with its competency tags loaded.
func (s *Store) GetStory(ctx context.Context, id int64) (model.Story, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+storyColumns+` FROM stories WHERE id = ?`, id)
	st, err := scanStory(row)
	if errors.Is(err, sql.ErrNoRows) {
		return model.Story{}, fmt.Errorf("story %d: %w", id, ErrNotFound)
	}
	if err != nil {
		return model.Story{}, err
	}
	if st.Competencies, err = s.competenciesFor(ctx, id); err != nil {
		return model.Story{}, err
	}
	return st, nil
}

// StoryFilter narrows a story listing.
type StoryFilter struct {
	Competency model.Competency
	ReadyOnly  bool // interview-ready: complete and sanitized
	Search     string
}

// ListStories returns stories matching f, newest first, tags loaded.
func (s *Store) ListStories(ctx context.Context, f StoryFilter) ([]model.Story, error) {
	q := `SELECT ` + storyColumns + ` FROM stories`
	var where []string
	var args []any

	if f.Competency != "" {
		q = `SELECT ` + prefixed(storyColumns, "s.") + ` FROM stories s
		     JOIN story_competencies c ON c.story_id = s.id`
		where = append(where, `c.competency = ?`)
		args = append(args, string(f.Competency))
	}
	if f.Search != "" {
		col := "title || ' ' || situation || ' ' || task || ' ' || action || ' ' || result"
		if f.Competency != "" {
			col = "s.title || ' ' || s.situation || ' ' || s.task || ' ' || s.action || ' ' || s.result"
		}
		where = append(where, `LOWER(`+col+`) LIKE ?`)
		args = append(args, "%"+strings.ToLower(f.Search)+"%")
	}
	if len(where) > 0 {
		q += ` WHERE ` + strings.Join(where, " AND ")
	}
	q += ` ORDER BY updated_at DESC`

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("listing stories: %w", err)
	}
	defer rows.Close()

	var out []model.Story
	for rows.Next() {
		st, err := scanStory(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("listing stories: %w", err)
	}

	for i := range out {
		cs, err := s.competenciesFor(ctx, out[i].ID)
		if err != nil {
			return nil, err
		}
		out[i].Competencies = cs
	}
	if f.ReadyOnly {
		ready := out[:0]
		for _, st := range out {
			if st.Ready() {
				ready = append(ready, st)
			}
		}
		out = ready
	}
	return out, nil
}

// DeleteStory removes a story and, by cascade, its tags.
func (s *Store) DeleteStory(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM stories WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("deleting story %d: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("story %d: %w", id, ErrNotFound)
	}
	return nil
}

func (s *Store) competenciesFor(ctx context.Context, storyID int64) ([]model.Competency, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT competency FROM story_competencies WHERE story_id = ?`, storyID)
	if err != nil {
		return nil, fmt.Errorf("loading competencies for story %d: %w", storyID, err)
	}
	defer rows.Close()

	var out []model.Competency
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return nil, fmt.Errorf("scanning competency: %w", err)
		}
		out = append(out, model.Competency(c))
	}
	return model.SortCompetencies(out), rows.Err()
}

func scanStory(sc scanner) (model.Story, error) {
	var (
		st                   model.Story
		createdAt, updatedAt string
	)
	err := sc.Scan(&st.ID, &st.Title, &st.Situation, &st.Task, &st.Action,
		&st.Result, &st.Metrics, &st.Sanitized, &createdAt, &updatedAt)
	if err != nil {
		return model.Story{}, err
	}
	if st.CreatedAt, err = parseTime(createdAt); err != nil {
		return model.Story{}, err
	}
	if st.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return model.Story{}, err
	}
	return st, nil
}

// prefixed qualifies a comma-separated column list with a table alias.
func prefixed(cols, prefix string) string {
	parts := strings.Split(cols, ",")
	for i, p := range parts {
		parts[i] = prefix + strings.TrimSpace(p)
	}
	return strings.Join(parts, ", ")
}
