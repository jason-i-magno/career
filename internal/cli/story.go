package cli

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/jason-i-magno/career/internal/model"
	"github.com/jason-i-magno/career/internal/store"
)

func runStory(ctx context.Context, env *Env, args []string) error {
	return dispatch(ctx, env, "story", []subcommand{
		{"add", "draft a new STAR story in $EDITOR", storyAdd},
		{"list", "list stories and their readiness", storyList},
		{"show", "print one story in full", storyShow},
		{"edit", "revise a story in $EDITOR", storyEdit},
		{"gaps", "which competencies have no ready story", storyGaps},
		{"rm", "delete a story", storyRemove},
	}, args)
}

func storyAdd(ctx context.Context, env *Env, args []string) error {
	fs := newFlagSet(env, "story add")
	title := fs.String("title", "", "story title (otherwise taken from the draft)")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	seedTitle := *title
	if seedTitle == "" && fs.NArg() > 0 {
		seedTitle = strings.Join(fs.Args(), " ")
	}
	if seedTitle == "" {
		seedTitle = "Untitled story"
	}

	draft := renderStoryFile(model.Story{Title: seedTitle})
	edited, err := editInEditor(draft, ".md")
	if err != nil {
		return err
	}
	s, err := parseStoryFile(edited, model.Story{})
	if err != nil {
		return err
	}
	if !s.Complete() {
		fmt.Fprintf(env.Err, "%s some STAR sections are empty; saving as a draft\n", yellow("note:"))
	}

	id, err := env.Store.CreateStory(ctx, s, env.Now())
	if err != nil {
		return err
	}
	fmt.Fprintf(env.Out, "%s story #%d %q %s\n", green("saved"), id, s.Title, readyBadge(s))
	return nil
}

func storyList(ctx context.Context, env *Env, args []string) error {
	fs := newFlagSet(env, "story list")
	var (
		competency = fs.String("competency", "", "filter by competency")
		ready      = fs.Bool("ready", false, "only interview-ready stories")
		search     = fs.String("search", "", "substring search across all fields")
	)
	if err := parseFlags(fs, args); err != nil {
		return err
	}

	f := store.StoryFilter{ReadyOnly: *ready, Search: *search}
	if *competency != "" {
		c, err := model.ParseCompetency(*competency)
		if err != nil {
			return err
		}
		f.Competency = c
	}

	stories, err := env.Store.ListStories(ctx, f)
	if err != nil {
		return err
	}
	if len(stories) == 0 {
		fmt.Fprintln(env.Out, dim("no matching stories — draft one with `career story add`"))
		return nil
	}

	tw := newTable(env.Out)
	fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", dim("ID"), dim("TITLE"), dim("COMPETENCIES"), dim("STATE"))
	for _, s := range stories {
		comps := make([]string, len(s.Competencies))
		for i, c := range s.Competencies {
			comps[i] = string(c)
		}
		label := strings.Join(comps, ",")
		if label == "" {
			label = dim("untagged")
		}
		fmt.Fprintf(tw, "%d\t%s\t%s\t%s\n", s.ID, bold(truncate(s.Title, 40)), truncate(label, 34), readyBadge(s))
	}
	tw.Flush()
	return nil
}

func storyShow(ctx context.Context, env *Env, args []string) error {
	if len(args) != 1 {
		return errors.New("usage: career story show <id>")
	}
	id, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return fmt.Errorf("bad id %q", args[0])
	}
	s, err := env.Store.GetStory(ctx, id)
	if err != nil {
		return err
	}

	fmt.Fprintf(env.Out, "\n%s  %s\n", bold(s.Title), readyBadge(s))
	comps := make([]string, len(s.Competencies))
	for i, c := range s.Competencies {
		comps[i] = string(c)
	}
	if len(comps) > 0 {
		fmt.Fprintf(env.Out, "%s\n", cyan(strings.Join(comps, " · ")))
	}
	for _, sec := range []struct{ name, body string }{
		{"Situation", s.Situation}, {"Task", s.Task},
		{"Action", s.Action}, {"Result", s.Result}, {"Metrics", s.Metrics},
	} {
		if strings.TrimSpace(sec.body) == "" {
			fmt.Fprintf(env.Out, "\n%s\n  %s\n", bold(sec.name), red("(empty)"))
			continue
		}
		fmt.Fprintf(env.Out, "\n%s\n  %s\n", bold(sec.name), wrap(sec.body, 76, "  "))
	}
	fmt.Fprintln(env.Out)
	return nil
}

func storyEdit(ctx context.Context, env *Env, args []string) error {
	if len(args) != 1 {
		return errors.New("usage: career story edit <id>")
	}
	id, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return fmt.Errorf("bad id %q", args[0])
	}
	s, err := env.Store.GetStory(ctx, id)
	if err != nil {
		return err
	}

	edited, err := editInEditor(renderStoryFile(s), ".md")
	if err != nil {
		return err
	}
	updated, err := parseStoryFile(edited, s)
	if err != nil {
		return err
	}
	if err := env.Store.UpdateStory(ctx, updated, env.Now()); err != nil {
		return err
	}
	fmt.Fprintf(env.Out, "%s story #%d %q %s\n", green("updated"), id, updated.Title, readyBadge(updated))
	return nil
}

func storyGaps(ctx context.Context, env *Env, args []string) error {
	stories, err := env.Store.ListStories(ctx, store.StoryFilter{})
	if err != nil {
		return err
	}
	gaps := model.CoverageGaps(stories)

	heading(env.Out, "Story bank coverage")
	ready := 0
	for _, s := range stories {
		if s.Ready() {
			ready++
		}
	}
	fmt.Fprintf(env.Out, "  %d stories, %s interview-ready\n\n", len(stories), green(strconv.Itoa(ready)))

	covered := map[model.Competency]bool{}
	for _, c := range model.AllCompetencies {
		covered[c] = true
	}
	for _, g := range gaps {
		covered[g] = false
	}

	tw := newTable(env.Out)
	for _, c := range model.AllCompetencies {
		if covered[c] {
			fmt.Fprintf(tw, "  %s\t%s\n", green("✓"), string(c))
			continue
		}
		fmt.Fprintf(tw, "  %s\t%s\t%s\n", red("✗"), string(c), dim(competencyPrompt(c)))
	}
	tw.Flush()

	if len(gaps) > 0 {
		fmt.Fprintf(env.Out, "\n%s %d competencies have no ready story. Draft one with %s\n",
			yellow("gap:"), len(gaps), bold("career story add"))
	} else {
		fmt.Fprintf(env.Out, "\n%s every competency has an interview-ready story.\n", green("covered:"))
	}
	fmt.Fprintln(env.Out)
	return nil
}

func storyRemove(ctx context.Context, env *Env, args []string) error {
	if len(args) != 1 {
		return errors.New("usage: career story rm <id>")
	}
	id, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return fmt.Errorf("bad id %q", args[0])
	}
	s, err := env.Store.GetStory(ctx, id)
	if err != nil {
		return err
	}
	if err := env.Store.DeleteStory(ctx, id); err != nil {
		return err
	}
	fmt.Fprintf(env.Out, "%s story #%d %q\n", red("deleted"), id, s.Title)
	return nil
}

// readyBadge summarises why a story is or is not usable in an interview.
func readyBadge(s model.Story) string {
	switch {
	case s.Ready():
		return green("ready")
	case !s.Complete() && !s.Sanitized:
		return red("incomplete, unsanitized")
	case !s.Complete():
		return yellow("incomplete")
	default:
		return yellow("unsanitized")
	}
}

// competencyPrompt is the question this competency usually arrives as.
func competencyPrompt(c model.Competency) string {
	switch c {
	case model.CompOwnership:
		return "\"Tell me about a project you owned end to end.\""
	case model.CompScale:
		return "\"Describe the most performance-sensitive thing you've built.\""
	case model.CompDebugging:
		return "\"What's the hardest bug you've tracked down?\""
	case model.CompConflict:
		return "\"Tell me about a disagreement with a colleague.\""
	case model.CompFailure:
		return "\"Tell me about a time you were wrong.\""
	case model.CompMentoring:
		return "\"How have you helped someone else grow?\""
	case model.CompAmbiguity:
		return "\"When have requirements been unclear? What did you do?\""
	case model.CompInfluence:
		return "\"How did you drive a change you had no authority over?\""
	case model.CompDelivery:
		return "\"Tell me about shipping under a hard deadline.\""
	case model.CompCollab:
		return "\"Describe working across team boundaries.\""
	}
	return ""
}
