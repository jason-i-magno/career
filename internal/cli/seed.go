package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/jason-i-magno/career/internal/model"
	"github.com/jason-i-magno/career/internal/prep"
	"github.com/jason-i-magno/career/internal/store"
)

// runSeed loads the starter prep deck and the story stubs. It is safe to run
// again: cards and stories are matched by prompt and title, so re-seeding after
// an upgrade adds only what is new and never disturbs scheduler state.
func runSeed(ctx context.Context, env *Env, args []string) error {
	fs := newFlagSet(env, "seed")
	var (
		deckOnly    = fs.Bool("deck-only", false, "load prep cards but not story stubs")
		storiesOnly = fs.Bool("stories-only", false, "load story stubs but not prep cards")
	)
	if err := parseFlags(fs, args); err != nil {
		return err
	}

	now := env.Now()
	var addedCards, addedStories int

	if !*storiesOnly {
		existing, err := env.Store.ListPrepItems(ctx, store.PrepFilter{})
		if err != nil {
			return err
		}
		have := make(map[string]bool, len(existing))
		for _, p := range existing {
			have[normalise(p.Prompt)] = true
		}
		for _, c := range seedDeck {
			if have[normalise(c.prompt)] {
				continue
			}
			ease, interval, reps, due := prep.Init(now)
			if _, err := env.Store.CreatePrepItem(ctx, model.PrepItem{
				Subject: c.subject, Prompt: c.prompt, Ref: c.ref,
				Ease: ease, IntervalDays: interval, Reps: reps, DueAt: due,
			}, now); err != nil {
				return err
			}
			addedCards++
		}
	}

	if !*deckOnly {
		existing, err := env.Store.ListStories(ctx, store.StoryFilter{})
		if err != nil {
			return err
		}
		have := make(map[string]bool, len(existing))
		for _, s := range existing {
			have[normalise(s.Title)] = true
		}
		for _, s := range seedStories {
			if have[normalise(s.title)] {
				continue
			}
			if _, err := env.Store.CreateStory(ctx, model.Story{
				Title:        s.title,
				Situation:    "",
				Competencies: s.competencies,
				// The hint rides in Metrics so it is visible in `story show`
				// and gets overwritten the moment you do the real work.
				Metrics:   "DRAFT — " + s.hint,
				Sanitized: false,
			}, now); err != nil {
				return err
			}
			addedStories++
		}
	}

	switch {
	case addedCards == 0 && addedStories == 0:
		fmt.Fprintf(env.Out, "%s already seeded — nothing new to add.\n", green("up to date:"))
	default:
		fmt.Fprintf(env.Out, "%s %d prep cards, %d story stubs\n", green("seeded"), addedCards, addedStories)
		next := []struct{ cmd, why string }{
			{"career prep review", "work through what is due"},
			{"career story gaps", "see which competencies are uncovered"},
			{"career today", "the morning dashboard"},
		}
		fmt.Fprintln(env.Out)
		tw := newTable(env.Out)
		for _, n := range next {
			fmt.Fprintf(tw, "  %s\t%s\n", bold(n.cmd), dim(n.why))
		}
		tw.Flush()
		fmt.Fprintln(env.Out)
	}
	return nil
}

// normalise makes the seed idempotency check insensitive to whitespace and case
// so that a lightly reworded card is still recognised as the same card.
func normalise(s string) string { return strings.ToLower(strings.Join(strings.Fields(s), " ")) }
