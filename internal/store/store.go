// Package store persists career-tracking data in a local SQLite database.
//
// Timestamps are stored as RFC3339 strings in UTC. SQLite has no native time
// type, and text timestamps keep the file greppable and diff-friendly, which
// matters for something you may want to inspect by hand.
package store

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite" // pure-Go driver: no cgo, so the binary cross-compiles
)

// ErrNotFound is returned when a lookup by ID matches no row.
var ErrNotFound = errors.New("not found")

// Store is a handle on the career database.
type Store struct {
	db *sql.DB
}

// migrations are applied in order, exactly once each, tracked by user_version.
// Never edit an applied migration; append a new one instead.
var migrations = []string{
	`CREATE TABLE applications (
		id             INTEGER PRIMARY KEY AUTOINCREMENT,
		company        TEXT    NOT NULL,
		role           TEXT    NOT NULL,
		stage          TEXT    NOT NULL,
		track          TEXT    NOT NULL,
		source         TEXT    NOT NULL,
		location       TEXT    NOT NULL DEFAULT '',
		remote         INTEGER NOT NULL DEFAULT 0,
		url            TEXT    NOT NULL DEFAULT '',
		contact        TEXT    NOT NULL DEFAULT '',
		base_comp      INTEGER NOT NULL DEFAULT 0,
		created_at     TEXT    NOT NULL,
		updated_at     TEXT    NOT NULL,
		next_action    TEXT    NOT NULL DEFAULT '',
		next_action_at TEXT
	);
	CREATE INDEX idx_applications_stage ON applications(stage);
	CREATE INDEX idx_applications_next_action_at ON applications(next_action_at);

	CREATE TABLE events (
		id             INTEGER PRIMARY KEY AUTOINCREMENT,
		application_id INTEGER NOT NULL REFERENCES applications(id) ON DELETE CASCADE,
		at             TEXT    NOT NULL,
		kind           TEXT    NOT NULL,
		body           TEXT    NOT NULL
	);
	CREATE INDEX idx_events_application_id ON events(application_id, at);

	CREATE TABLE stories (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		title      TEXT    NOT NULL,
		situation  TEXT    NOT NULL DEFAULT '',
		task       TEXT    NOT NULL DEFAULT '',
		action     TEXT    NOT NULL DEFAULT '',
		result     TEXT    NOT NULL DEFAULT '',
		metrics    TEXT    NOT NULL DEFAULT '',
		sanitized  INTEGER NOT NULL DEFAULT 0,
		created_at TEXT    NOT NULL,
		updated_at TEXT    NOT NULL
	);

	CREATE TABLE story_competencies (
		story_id   INTEGER NOT NULL REFERENCES stories(id) ON DELETE CASCADE,
		competency TEXT    NOT NULL,
		PRIMARY KEY (story_id, competency)
	);

	CREATE TABLE prep_items (
		id            INTEGER PRIMARY KEY AUTOINCREMENT,
		subject       TEXT    NOT NULL,
		prompt        TEXT    NOT NULL,
		ref           TEXT    NOT NULL DEFAULT '',
		ease          REAL    NOT NULL,
		interval_days INTEGER NOT NULL DEFAULT 0,
		reps          INTEGER NOT NULL DEFAULT 0,
		lapses        INTEGER NOT NULL DEFAULT 0,
		due_at        TEXT    NOT NULL,
		last_grade    INTEGER,
		reviewed_at   TEXT,
		created_at    TEXT    NOT NULL
	);
	CREATE INDEX idx_prep_items_due_at ON prep_items(due_at);`,

	// 2: optional written answer, revealed during review. Empty by default —
	// most cards are open prompts with no single right answer.
	`ALTER TABLE prep_items ADD COLUMN answer TEXT NOT NULL DEFAULT '';`,
}

// DefaultPath returns the database location, honouring CAREER_DB when set so
// that tests and throwaway experiments can point somewhere else.
func DefaultPath() (string, error) {
	if p := os.Getenv("CAREER_DB"); p != "" {
		return p, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locating home directory: %w", err)
	}
	return filepath.Join(home, ".local", "share", "career", "career.db"), nil
}

// Open opens (creating if needed) the database at path and applies migrations.
func Open(path string) (*Store, error) {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("creating database directory: %w", err)
		}
	}
	// Foreign keys are off by default in SQLite and must be enabled per
	// connection; the DSN parameter applies it to every pooled connection.
	db, err := sql.Open("sqlite", path+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

// migrate applies any migrations the database has not yet seen, using SQLite's
// user_version pragma as the schema version counter.
func (s *Store) migrate() error {
	var version int
	if err := s.db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		return fmt.Errorf("reading schema version: %w", err)
	}
	for i := version; i < len(migrations); i++ {
		tx, err := s.db.Begin()
		if err != nil {
			return fmt.Errorf("beginning migration %d: %w", i+1, err)
		}
		if _, err := tx.Exec(migrations[i]); err != nil {
			tx.Rollback()
			return fmt.Errorf("applying migration %d: %w", i+1, err)
		}
		// PRAGMA does not accept a bound parameter, and i+1 is loop-controlled.
		if _, err := tx.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, i+1)); err != nil {
			tx.Rollback()
			return fmt.Errorf("recording schema version %d: %w", i+1, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("committing migration %d: %w", i+1, err)
		}
	}
	return nil
}

// --- time helpers -----------------------------------------------------------

const timeLayout = time.RFC3339

func fmtTime(t time.Time) string { return t.UTC().Format(timeLayout) }

func fmtTimePtr(t *time.Time) any {
	if t == nil {
		return nil
	}
	return fmtTime(*t)
}

func parseTime(s string) (time.Time, error) {
	t, err := time.Parse(timeLayout, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("parsing timestamp %q: %w", s, err)
	}
	return t.UTC(), nil
}

func parseTimePtr(s sql.NullString) (*time.Time, error) {
	if !s.Valid || s.String == "" {
		return nil, nil
	}
	t, err := parseTime(s.String)
	if err != nil {
		return nil, err
	}
	return &t, nil
}
