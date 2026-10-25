//go:build !unix

package cli

import "io"

// toJournal reports false: there is no systemd journal off Unix.
func toJournal(io.Writer) bool { return false }
