//go:build unix

package cli

import (
	"fmt"
	"io"
	"os"
	"syscall"
)

// toJournal reports whether w is the stream systemd connected to the journal.
//
// systemd names that stream's device and inode in JOURNAL_STREAM. The variable
// alone proves nothing: everything a service starts inherits it, including the
// shells in a terminal emulator that runs as a user service, so w must be the
// very stream it names.
func toJournal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	var dev, ino uint64
	if _, err := fmt.Sscanf(os.Getenv("JOURNAL_STREAM"), "%d:%d", &dev, &ino); err != nil {
		return false
	}
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	return ok && uint64(st.Dev) == dev && uint64(st.Ino) == ino
}
