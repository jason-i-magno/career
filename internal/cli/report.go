package cli

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jason-i-magno/career/internal/model"
	"github.com/jason-i-magno/career/internal/store"
)

// StaleAfter is how long an active application may sit untouched before it is
// flagged. Two weeks is roughly the point at which a polite nudge is still
// natural and the role is probably still open.
const StaleAfter = 14 * 24 * time.Hour

// runToday is the command you run every morning: what is overdue, what has gone
// quiet, and what is due to review.
func runToday(ctx context.Context, env *Env, args []string) error {
	now := env.Now()

	apps, err := env.Store.ListApplications(ctx, store.ApplicationFilter{ActiveOnly: true})
	if err != nil {
		return err
	}

	var due, stale []model.Application
	for _, a := range apps {
		switch {
		case a.Due(now):
			due = append(due, a)
		case a.Stale(now, StaleAfter):
			stale = append(stale, a)
		}
	}
	slices.SortStableFunc(due, func(a, b model.Application) int {
		return a.NextActionAt.Compare(*b.NextActionAt)
	})

	fmt.Fprintf(env.Out, "\n%s  %s\n", bold("career"), dim(now.Format("Mon 2 Jan 2006")))

	if len(due) > 0 {
		heading(env.Out, fmt.Sprintf("Due now (%d)", len(due)))
		tw := newTable(env.Out)
		for _, a := range due {
			fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\n", dim(fmt.Sprintf("#%d", a.ID)),
				bold(truncate(a.Company, 20)), stageBadge(a.Stage),
				red(a.NextAction)+dim(" ("+relDays(*a.NextActionAt, now)+")"))
		}
		tw.Flush()
	}

	if len(stale) > 0 {
		heading(env.Out, fmt.Sprintf("Gone quiet (%d)", len(stale)))
		fmt.Fprintf(env.Out, "%s\n", dim("  untouched for over two weeks — follow up or close them out"))
		tw := newTable(env.Out)
		for _, a := range stale {
			fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\n", dim(fmt.Sprintf("#%d", a.ID)),
				bold(truncate(a.Company, 20)), stageBadge(a.Stage),
				yellow(relDays(a.UpdatedAt, now)))
		}
		tw.Flush()
	}

	summary, dueCount, err := nextDueSummary(ctx, env, now)
	if err != nil {
		return err
	}
	if dueCount > 0 {
		heading(env.Out, fmt.Sprintf("Prep due (%d)", dueCount))
		fmt.Fprintf(env.Out, "  %s\n  %s\n", summary, dim("`career prep review`"))
	}

	stories, err := env.Store.ListStories(ctx, store.StoryFilter{})
	if err != nil {
		return err
	}
	if gaps := model.CoverageGaps(stories); len(gaps) > 0 {
		heading(env.Out, fmt.Sprintf("Story gaps (%d)", len(gaps)))
		names := make([]string, len(gaps))
		for i, g := range gaps {
			names[i] = string(g)
		}
		fmt.Fprintf(env.Out, "  %s\n  %s\n", yellow(strings.Join(names, ", ")), dim("`career story add`"))
	}

	if len(due) == 0 && len(stale) == 0 && dueCount == 0 {
		fmt.Fprintf(env.Out, "\n%s nothing due. Good time to add opportunities or draft a story.\n", green("clear:"))
	}

	// The pipeline line is the number that actually predicts an offer: how many
	// live conversations you have, not how many applications you have sent.
	live := 0
	for _, a := range apps {
		if a.Stage != model.StageLead {
			live++
		}
	}
	fmt.Fprintf(env.Out, "\n%s\n\n", dim(fmt.Sprintf("%d live %s, %d %s not yet applied to",
		live, plural(live, "opportunity", "opportunities"),
		len(apps)-live, plural(len(apps)-live, "lead", "leads"))))
	return nil
}

// runStats renders the funnel and the conversion rates that tell you which
// channel is actually working.
func runStats(ctx context.Context, env *Env, args []string) error {
	apps, err := env.Store.ListApplications(ctx, store.ApplicationFilter{})
	if err != nil {
		return err
	}
	if len(apps) == 0 {
		fmt.Fprintln(env.Out, dim("no applications yet"))
		return nil
	}

	// Funnel: an application counts toward every stage it actually reached, so
	// a rejection after an onsite still credits screen, tech and onsite. That
	// distinction is the whole point — being rejected at the final round and
	// being ignored after applying are opposite problems with opposite fixes.
	reached := map[model.Stage]int{}
	for _, a := range apps {
		events, err := env.Store.ListEvents(ctx, a.ID)
		if err != nil {
			return err
		}
		furthest := furthestStage(a, events)
		for _, s := range model.ActiveStages {
			if s.Order() <= furthest.Order() {
				reached[s]++
			}
		}
	}

	heading(env.Out, "Funnel")
	maxCount := 0
	for _, s := range model.ActiveStages {
		if reached[s] > maxCount {
			maxCount = reached[s]
		}
	}
	tw := newTable(env.Out)
	for _, s := range model.ActiveStages {
		n := reached[s]
		bar := ""
		if maxCount > 0 {
			bar = strings.Repeat("█", n*28/maxCount)
		}
		pct := ""
		if reached[model.StageApplied] > 0 && s.Order() > model.StageApplied.Order() {
			pct = fmt.Sprintf("%.0f%%", 100*float64(n)/float64(reached[model.StageApplied]))
		}
		fmt.Fprintf(tw, "  %s\t%d\t%s\t%s\n", string(s), n, blue(bar), dim(pct))
	}
	tw.Flush()

	heading(env.Out, "By source")
	renderBreakdown(env.Out, apps, func(a model.Application) string { return string(a.Source) })

	heading(env.Out, "By track")
	renderBreakdown(env.Out, apps, func(a model.Application) string { return string(a.Track) })

	fmt.Fprintln(env.Out)
	return nil
}

// renderBreakdown groups applications by key and shows how many got past the
// application stage — the only conversion number worth watching early on.
func renderBreakdown(w interface{ Write([]byte) (int, error) }, apps []model.Application, key func(model.Application) string) {
	type row struct{ total, advanced, rejected int }
	groups := map[string]*row{}
	var order []string
	for _, a := range apps {
		k := key(a)
		if groups[k] == nil {
			groups[k] = &row{}
			order = append(order, k)
		}
		g := groups[k]
		g.total++
		if a.Stage.Order() >= model.StageScreen.Order() && !a.Stage.Terminal() {
			g.advanced++
		}
		if a.Stage == model.StageRejected {
			g.rejected++
		}
	}
	slices.SortStableFunc(order, func(a, b string) int { return cmp.Compare(groups[b].total, groups[a].total) })

	tw := newTable(w)
	fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\n", dim("KEY"), dim("TOTAL"), dim("ADVANCED"), dim("REJECTED"))
	for _, k := range order {
		g := groups[k]
		rate := ""
		if g.total > 0 {
			rate = fmt.Sprintf("  %.0f%%", 100*float64(g.advanced)/float64(g.total))
		}
		fmt.Fprintf(tw, "  %s\t%d\t%d%s\t%d\n", k, g.total, g.advanced, dim(rate), g.rejected)
	}
	tw.Flush()
}

// plural picks the singular or plural form for n.
func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// furthestStage returns the most advanced non-terminal stage an application
// ever reached, reconstructed from its event log. The current stage is used as
// a floor, so an application whose log was never written still counts.
func furthestStage(a model.Application, events []model.Event) model.Stage {
	furthest := a.Stage
	if furthest.Terminal() {
		// A rejection tells us nothing about progress; start from the bottom
		// and let the log supply the real high-water mark.
		furthest = model.StageLead
	}
	consider := func(raw string) {
		s, err := model.ParseStage(strings.TrimSpace(raw))
		if err != nil || s.Terminal() {
			return
		}
		if s.Order() > furthest.Order() {
			furthest = s
		}
	}
	for _, e := range events {
		if e.Kind != model.EventStage {
			continue
		}
		// Bodies are either "created at stage X" or "from → to".
		if from, to, ok := strings.Cut(e.Body, "→"); ok {
			consider(from)
			consider(to)
			continue
		}
		if _, stage, ok := strings.Cut(e.Body, "created at stage "); ok {
			consider(stage)
		}
	}
	return furthest
}
