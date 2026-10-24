//go:build unix

package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestToJournalNeedsTheStreamItNames(t *testing.T) {
	create := func(name string) *os.File {
		f, err := os.Create(filepath.Join(t.TempDir(), name))
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		t.Cleanup(func() { f.Close() })
		return f
	}
	journal, other := create("journal"), create("other")
	fi, err := journal.Stat()
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	st := fi.Sys().(*syscall.Stat_t)
	t.Setenv("JOURNAL_STREAM", fmt.Sprintf("%d:%d", st.Dev, st.Ino))

	if !toJournal(journal) {
		t.Error("the stream JOURNAL_STREAM names should count as the journal")
	}
	// A shell in a terminal emulator that runs as a user service inherits the
	// variable, but its stderr is the terminal.
	if toJournal(other) {
		t.Error("an inherited JOURNAL_STREAM must not make another stream the journal")
	}
	if toJournal(&bytes.Buffer{}) {
		t.Error("a buffer is never the journal")
	}

	t.Setenv("JOURNAL_STREAM", "")
	if toJournal(journal) {
		t.Error("without JOURNAL_STREAM nothing is the journal")
	}
}
