package model

import (
	"errors"
	"path/filepath"
	"strings"
	"time"
)

// Doc is a file you have chosen to read on the dashboard: notes, a checklist,
// anything written in Markdown.
//
// Only the name and path are stored. The text stays in the file and is read
// afresh on every view, so it is never copied into the database, never goes
// stale, and is edited wherever you already edit it.
type Doc struct {
	ID        int64
	Name      string
	Path      string // absolute, so the server finds it whatever its working directory
	CreatedAt time.Time
}

// Validate reports whether d can be stored.
func (d Doc) Validate() error {
	if strings.TrimSpace(d.Name) == "" {
		return errors.New("a doc needs a name")
	}
	if !filepath.IsAbs(d.Path) {
		return errors.New("a doc's path must be absolute")
	}
	return nil
}
