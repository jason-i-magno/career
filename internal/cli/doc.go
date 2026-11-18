package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jason-i-magno/career/internal/model"
)

func runDoc(ctx context.Context, env *Env, args []string) error {
	return dispatch(ctx, env, "doc", []subcommand{
		{"add", "register a Markdown file to read on the dashboard", docAdd},
		{"list", "list registered docs", docList},
		{"rm", "unregister a doc (the file is untouched)", docRemove},
	}, args)
}

func docAdd(ctx context.Context, env *Env, args []string) error {
	fs := newFlagSet(env, "doc add")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 2 {
		return errors.New(`usage: career doc add "<name>" <path>`)
	}
	name, path := strings.TrimSpace(fs.Arg(0)), fs.Arg(1)

	abs, err := docPath(path)
	if err != nil {
		return err
	}
	id, err := env.Store.SaveDoc(ctx, model.Doc{Name: name, Path: abs}, env.Now())
	if err != nil {
		return err
	}
	fmt.Fprintf(env.Out, "%s doc #%d %q → %s\n", green("saved"), id, name, abs)
	// The dashboard has no login, and unlike a story there is no sanitized
	// gate in front of a doc, so say once what registering one means.
	fmt.Fprintf(env.Out, "%s anything in this file can be read by every device that reaches `career serve`, at /docs/%d\n",
		dim("note:"), id)
	return nil
}

// docPath resolves path to an absolute path and checks that it names a
// readable regular file. The server reads it later from its own working
// directory, so a relative path would point somewhere else by then.
func docPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolving %s: %w", path, err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("checking %s: %w", abs, err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("%s is not a regular file", abs)
	}
	f, err := os.Open(abs)
	if err != nil {
		return "", fmt.Errorf("opening %s: %w", abs, err)
	}
	f.Close()
	return abs, nil
}

func docList(ctx context.Context, env *Env, args []string) error {
	docs, err := env.Store.ListDocs(ctx)
	if err != nil {
		return err
	}
	if len(docs) == 0 {
		fmt.Fprintln(env.Out, dim(`no docs — register one with career doc add "<name>" <path>`))
		return nil
	}
	tw := newTable(env.Out)
	fmt.Fprintf(tw, "%s\t%s\t%s\n", dim("ID"), dim("NAME"), dim("PATH"))
	for _, d := range docs {
		path := d.Path
		if _, err := os.Stat(d.Path); err != nil {
			path += " " + red("(missing)")
		}
		fmt.Fprintf(tw, "%d\t%s\t%s\n", d.ID, bold(truncate(d.Name, 40)), path)
	}
	tw.Flush()
	return nil
}

func docRemove(ctx context.Context, env *Env, args []string) error {
	if len(args) != 1 {
		return errors.New(`usage: career doc rm "<name>"`)
	}
	if err := env.Store.DeleteDoc(ctx, args[0]); err != nil {
		return err
	}
	fmt.Fprintf(env.Out, "%s doc %q (the file is untouched)\n", red("removed"), args[0])
	return nil
}
