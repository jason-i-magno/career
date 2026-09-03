// Package cli implements the `career` command-line interface.
//
// Commands are dispatched by hand against the standard library's flag package
// rather than a framework: the surface is small, and keeping the dependency
// list short makes the binary trivial to build and audit.
package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/jason-i-magno/career/internal/store"
)

// Env carries the process environment a command needs, so that tests can supply
// their own store, clock and output streams.
type Env struct {
	Store *store.Store
	Now   func() time.Time
	Out   io.Writer
	Err   io.Writer
}

// command is one top-level subcommand.
type command struct {
	name    string
	summary string
	run     func(ctx context.Context, env *Env, args []string) error
}

var commands = []command{
	{"today", "what needs doing right now", runToday},
	{"app", "track applications through the pipeline", runApp},
	{"story", "build and audit your STAR story bank", runStory},
	{"prep", "spaced-repetition interview prep", runPrep},
	{"stats", "funnel and conversion rates by source and track", runStats},
	{"seed", "load the starter prep deck and story prompts", runSeed},
}

// Main is the process entry point. It resolves the database, builds an Env and
// dispatches, returning the exit code.
func Main(args []string) int {
	ctx := context.Background()

	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		usage(os.Stdout)
		return 0
	}

	name := args[0]
	var cmd *command
	for i := range commands {
		if commands[i].name == name {
			cmd = &commands[i]
			break
		}
	}
	if cmd == nil {
		fmt.Fprintf(os.Stderr, "career: unknown command %q\n\n", name)
		usage(os.Stderr)
		return 2
	}

	path, err := store.DefaultPath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "career: %v\n", err)
		return 1
	}
	st, err := store.Open(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "career: %v\n", err)
		return 1
	}
	defer st.Close()

	env := &Env{Store: st, Now: time.Now, Out: os.Stdout, Err: os.Stderr}
	if err := cmd.run(ctx, env, args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		fmt.Fprintf(os.Stderr, "career %s: %v\n", name, err)
		return 1
	}
	return 0
}

func usage(w io.Writer) {
	fmt.Fprintf(w, `career — an application pipeline, story bank and interview deck

Usage:
  career <command> [arguments]

Commands:
`)
	tw := newTable(w)
	for _, c := range commands {
		fmt.Fprintf(tw, "  %s\t%s\n", c.name, c.summary)
	}
	tw.Flush()
	fmt.Fprintf(w, `
Run "career <command> -h" for the flags of a single command.

The database lives at ~/.local/share/career/career.db (override with CAREER_DB).
`)
}

// newFlagSet builds a flag set that reports errors through the returned error
// rather than exiting the process, and whose usage names the full command path.
func newFlagSet(env *Env, name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(env.Err)
	return fs
}

// subcommand dispatches args against a table of named handlers.
type subcommand struct {
	name    string
	summary string
	run     func(ctx context.Context, env *Env, args []string) error
}

func dispatch(ctx context.Context, env *Env, group string, subs []subcommand, args []string) error {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		subUsage(env.Out, group, subs)
		return nil
	}
	for _, s := range subs {
		if s.name == args[0] {
			return s.run(ctx, env, args[1:])
		}
	}
	subUsage(env.Err, group, subs)
	return fmt.Errorf("unknown subcommand %q", args[0])
}

func subUsage(w io.Writer, group string, subs []subcommand) {
	fmt.Fprintf(w, "Usage: career %s <subcommand> [arguments]\n\nSubcommands:\n", group)
	tw := newTable(w)
	for _, s := range subs {
		fmt.Fprintf(tw, "  %s\t%s\n", s.name, s.summary)
	}
	tw.Flush()
}
