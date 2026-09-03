package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jason-i-magno/career/internal/model"
	"github.com/jason-i-magno/career/internal/store"
)

func runApp(ctx context.Context, env *Env, args []string) error {
	return dispatch(ctx, env, "app", []subcommand{
		{"add", "record a new opportunity", appAdd},
		{"list", "list opportunities (active by default)", appList},
		{"show", "show one opportunity and its history", appShow},
		{"set", "update fields, including stage", appSet},
		{"note", "append a note to the history", appNote},
		{"rm", "delete an opportunity", appRemove},
	}, args)
}

func appAdd(ctx context.Context, env *Env, args []string) error {
	fs := newFlagSet(env, "app add")
	var (
		stage    = fs.String("stage", "lead", "pipeline stage: "+model.JoinStages(model.AllStages))
		track    = fs.String("track", "backend", "role family: systems, backend, other")
		source   = fs.String("source", "board", "how you found it: referral, recruiter, board, direct, network")
		location = fs.String("location", "", "office location")
		remote   = fs.Bool("remote", false, "role is remote")
		url      = fs.String("url", "", "job posting URL")
		contact  = fs.String("contact", "", "recruiter or referrer")
		comp     = fs.Int("comp", 0, "base salary in dollars, if known")
		next     = fs.String("next", "", "next action, e.g. \"tailor resume\"")
		due      = fs.String("due", "", "when the next action is due: YYYY-MM-DD, +7d, today")
	)
	fs.Usage = func() {
		fmt.Fprintf(env.Err, "Usage: career app add <company> <role> [flags]\n\nFlags:\n")
		fs.PrintDefaults()
	}
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() < 2 {
		fs.Usage()
		return errors.New("need a company and a role")
	}

	st, err := model.ParseStage(*stage)
	if err != nil {
		return err
	}
	tr, err := model.ParseTrack(*track)
	if err != nil {
		return err
	}
	src, err := model.ParseSource(*source)
	if err != nil {
		return err
	}

	now := env.Now()
	a := model.Application{
		Company:    fs.Arg(0),
		Role:       strings.Join(fs.Args()[1:], " "),
		Stage:      st,
		Track:      tr,
		Source:     src,
		Location:   *location,
		Remote:     *remote,
		URL:        *url,
		Contact:    *contact,
		BaseComp:   *comp,
		NextAction: *next,
	}
	if *due != "" {
		t, err := parseWhen(*due, now)
		if err != nil {
			return err
		}
		if !t.IsZero() {
			a.NextActionAt = &t
		}
	}

	id, err := env.Store.CreateApplication(ctx, a, now)
	if err != nil {
		return err
	}
	if _, err := env.Store.AddEvent(ctx, model.Event{
		ApplicationID: id, At: now, Kind: model.EventStage,
		Body: "created at stage " + string(st),
	}); err != nil {
		return err
	}
	fmt.Fprintf(env.Out, "%s #%d %s — %s (%s)\n", green("added"), id, bold(a.Company), a.Role, a.Stage)
	return nil
}

func appList(ctx context.Context, env *Env, args []string) error {
	fs := newFlagSet(env, "app list")
	var (
		stage   = fs.String("stage", "", "filter by stage")
		track   = fs.String("track", "", "filter by track: systems, backend, other")
		company = fs.String("company", "", "filter by company substring")
		all     = fs.Bool("all", false, "include rejected and withdrawn")
	)
	if err := parseFlags(fs, args); err != nil {
		return err
	}

	f := store.ApplicationFilter{ActiveOnly: !*all, Company: *company}
	if *stage != "" {
		s, err := model.ParseStage(*stage)
		if err != nil {
			return err
		}
		f.Stages, f.ActiveOnly = []model.Stage{s}, false
	}
	if *track != "" {
		t, err := model.ParseTrack(*track)
		if err != nil {
			return err
		}
		f.Track = t
	}

	apps, err := env.Store.ListApplications(ctx, f)
	if err != nil {
		return err
	}
	if len(apps) == 0 {
		fmt.Fprintln(env.Out, dim("no matching applications — add one with `career app add`"))
		return nil
	}

	now := env.Now()
	tw := newTable(env.Out)
	fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
		dim("ID"), dim("COMPANY"), dim("ROLE"), dim("STAGE"), dim("TRACK"), dim("SOURCE"), dim("NEXT"))
	for _, a := range apps {
		fmt.Fprintf(tw, "%d\t%s\t%s\t%s\t%s\t%s\t%s\n",
			a.ID, bold(truncate(a.Company, 22)), truncate(a.Role, 30),
			stageBadge(a.Stage), a.Track, a.Source, nextCell(a, now))
	}
	tw.Flush()
	fmt.Fprintf(env.Out, "\n%s\n", dim(fmt.Sprintf("%d shown", len(apps))))
	return nil
}

func appShow(ctx context.Context, env *Env, args []string) error {
	if len(args) != 1 {
		return errors.New("usage: career app show <id>")
	}
	id, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return fmt.Errorf("bad id %q", args[0])
	}
	a, err := env.Store.GetApplication(ctx, id)
	if err != nil {
		return err
	}
	now := env.Now()

	fmt.Fprintf(env.Out, "\n%s  %s\n", bold(a.Company), dim("#"+strconv.FormatInt(a.ID, 10)))
	fmt.Fprintf(env.Out, "%s\n", a.Role)

	tw := newTable(env.Out)
	fmt.Fprintf(tw, "\n%s\t%s\n", dim("stage"), stageBadge(a.Stage))
	fmt.Fprintf(tw, "%s\t%s / %s\n", dim("track"), a.Track, a.Source)
	loc := a.Location
	if a.Remote {
		if loc == "" {
			loc = "remote"
		} else {
			loc += " (remote)"
		}
	}
	if loc != "" {
		fmt.Fprintf(tw, "%s\t%s\n", dim("location"), loc)
	}
	if a.Contact != "" {
		fmt.Fprintf(tw, "%s\t%s\n", dim("contact"), a.Contact)
	}
	if a.BaseComp > 0 {
		fmt.Fprintf(tw, "%s\t$%s\n", dim("base"), humanUSD(a.BaseComp))
	}
	if a.URL != "" {
		fmt.Fprintf(tw, "%s\t%s\n", dim("url"), a.URL)
	}
	fmt.Fprintf(tw, "%s\t%s\n", dim("updated"), relDays(a.UpdatedAt, now))
	if a.NextAction != "" {
		fmt.Fprintf(tw, "%s\t%s\n", dim("next"), nextCell(a, now))
	}
	tw.Flush()

	events, err := env.Store.ListEvents(ctx, a.ID)
	if err != nil {
		return err
	}
	if len(events) > 0 {
		heading(env.Out, "History")
		// The body wraps under itself: 2 gutter + 10 date + 2 + 5 kind + 2.
		const bodyIndent = "                     "
		for _, e := range events {
			fmt.Fprintf(env.Out, "  %s  %s  %s\n",
				dim(e.At.Local().Format("2006-01-02")),
				cyan(fmt.Sprintf("%-5s", e.Kind)),
				wrap(e.Body, 66, bodyIndent))
		}
	}
	fmt.Fprintln(env.Out)
	return nil
}

func appSet(ctx context.Context, env *Env, args []string) error {
	fs := newFlagSet(env, "app set")
	var (
		stage    = fs.String("stage", "", "move to a new stage")
		next     = fs.String("next", "", "set the next action")
		due      = fs.String("due", "", "set when the next action is due (or \"clear\")")
		contact  = fs.String("contact", "", "set the recruiter or referrer")
		comp     = fs.Int("comp", -1, "set base salary in dollars")
		url      = fs.String("url", "", "set the posting URL")
		location = fs.String("location", "", "set the office location")
		track    = fs.String("track", "", "set the role family")
		source   = fs.String("source", "", "set how you found it")
	)
	fs.Usage = func() {
		fmt.Fprintf(env.Err, "Usage: career app set <id> [flags]\n\nFlags:\n")
		fs.PrintDefaults()
	}
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return errors.New("need exactly one application id")
	}
	id, err := strconv.ParseInt(fs.Arg(0), 10, 64)
	if err != nil {
		return fmt.Errorf("bad id %q", fs.Arg(0))
	}

	a, err := env.Store.GetApplication(ctx, id)
	if err != nil {
		return err
	}
	now := env.Now()
	oldStage := a.Stage

	// Only flags the user actually passed are applied, so that `app set` can
	// change one field without clobbering the rest.
	var touched []string
	fs.Visit(func(f *flag.Flag) { touched = append(touched, f.Name) })
	if len(touched) == 0 {
		return errors.New("nothing to change: pass at least one flag")
	}
	for _, name := range touched {
		switch name {
		case "stage":
			s, err := model.ParseStage(*stage)
			if err != nil {
				return err
			}
			a.Stage = s
		case "next":
			a.NextAction = *next
		case "due":
			t, err := parseWhen(*due, now)
			if err != nil {
				return err
			}
			if t.IsZero() {
				a.NextActionAt = nil
			} else {
				a.NextActionAt = &t
			}
		case "contact":
			a.Contact = *contact
		case "comp":
			a.BaseComp = *comp
		case "url":
			a.URL = *url
		case "location":
			a.Location = *location
		case "track":
			t, err := model.ParseTrack(*track)
			if err != nil {
				return err
			}
			a.Track = t
		case "source":
			s, err := model.ParseSource(*source)
			if err != nil {
				return err
			}
			a.Source = s
		}
	}

	if err := env.Store.UpdateApplication(ctx, a, now); err != nil {
		return err
	}
	if a.Stage != oldStage {
		if _, err := env.Store.AddEvent(ctx, model.Event{
			ApplicationID: a.ID, At: now, Kind: model.EventStage,
			Body: fmt.Sprintf("%s → %s", oldStage, a.Stage),
		}); err != nil {
			return err
		}
		fmt.Fprintf(env.Out, "%s #%d %s: %s → %s\n", green("moved"), a.ID, bold(a.Company), oldStage, stageBadge(a.Stage))
		return nil
	}
	fmt.Fprintf(env.Out, "%s #%d %s (%s)\n", green("updated"), a.ID, bold(a.Company), strings.Join(touched, ", "))
	return nil
}

func appNote(ctx context.Context, env *Env, args []string) error {
	if len(args) < 2 {
		return errors.New("usage: career app note <id> <text...>")
	}
	id, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return fmt.Errorf("bad id %q", args[0])
	}
	a, err := env.Store.GetApplication(ctx, id)
	if err != nil {
		return err
	}
	now := env.Now()
	body := strings.Join(args[1:], " ")
	if _, err := env.Store.AddEvent(ctx, model.Event{
		ApplicationID: id, At: now, Kind: model.EventNote, Body: body,
	}); err != nil {
		return err
	}
	// Touch the application so a noted opportunity stops counting as stale.
	if err := env.Store.UpdateApplication(ctx, a, now); err != nil {
		return err
	}
	fmt.Fprintf(env.Out, "%s note on #%d %s\n", green("added"), id, bold(a.Company))
	return nil
}

func appRemove(ctx context.Context, env *Env, args []string) error {
	if len(args) != 1 {
		return errors.New("usage: career app rm <id>")
	}
	id, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return fmt.Errorf("bad id %q", args[0])
	}
	a, err := env.Store.GetApplication(ctx, id)
	if err != nil {
		return err
	}
	if err := env.Store.DeleteApplication(ctx, id); err != nil {
		return err
	}
	fmt.Fprintf(env.Out, "%s #%d %s\n", red("deleted"), id, bold(a.Company))
	return nil
}

// stageBadge colours a stage by how far along it is.
func stageBadge(s model.Stage) string {
	switch s {
	case model.StageOffer:
		return green(string(s))
	case model.StageOnsite, model.StageTech:
		return cyan(string(s))
	case model.StageScreen, model.StageApplied:
		return blue(string(s))
	case model.StageRejected:
		return red(string(s))
	case model.StageWithdrawn:
		return dim(string(s))
	default:
		return dim(string(s))
	}
}

// nextCell renders the next action with its due date, reddened when overdue.
func nextCell(a model.Application, now time.Time) string {
	if a.NextAction == "" {
		return dim("—")
	}
	if a.NextActionAt == nil {
		return a.NextAction
	}
	when := relDays(*a.NextActionAt, now)
	if a.Due(now) {
		return red(a.NextAction + " (" + when + ")")
	}
	return a.NextAction + dim(" ("+when+")")
}

func humanUSD(n int) string {
	s := strconv.Itoa(n)
	var out []byte
	for i, c := range []byte(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, c)
	}
	return string(out)
}
