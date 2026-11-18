package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jason-i-magno/career/internal/store"
)

func newDocEnv(t *testing.T) (*Env, *bytes.Buffer) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	var out bytes.Buffer
	now := time.Date(2026, time.September, 15, 10, 0, 0, 0, time.UTC)
	return &Env{Store: st, Now: func() time.Time { return now }, Out: &out, Err: &out}, &out
}

// The server reads the file later, from its own working directory, so a
// relative path must be stored as the absolute path it meant when typed.
func TestDocAddStoresAnAbsolutePath(t *testing.T) {
	env, out := newDocEnv(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "notes.md"), []byte("# Notes\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	if err := docAdd(context.Background(), env, []string{"My notes", "notes.md"}); err != nil {
		t.Fatalf("doc add: %v", err)
	}
	docs, err := env.Store.ListDocs(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 1 {
		t.Fatalf("got %d docs, want 1", len(docs))
	}
	if want := filepath.Join(dir, "notes.md"); docs[0].Path != want {
		t.Errorf("path = %q, want %q", docs[0].Path, want)
	}
	if !strings.Contains(out.String(), "every device") {
		t.Errorf("output should warn that the doc is readable on the dashboard:\n%s", out)
	}
}

func TestDocAddRejectsWhatItCannotServe(t *testing.T) {
	dir := t.TempDir()
	tests := []struct {
		name string
		args []string
	}{
		{"missing file", []string{"Notes", filepath.Join(dir, "nope.md")}},
		{"directory", []string{"Notes", dir}},
		{"no path", []string{"Notes"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			env, _ := newDocEnv(t)
			if err := docAdd(context.Background(), env, tc.args); err == nil {
				t.Error("doc add should have failed")
			}
			docs, err := env.Store.ListDocs(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if len(docs) != 0 {
				t.Errorf("a failed add stored %d docs", len(docs))
			}
		})
	}
}
