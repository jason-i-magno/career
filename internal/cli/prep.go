package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jason-i-magno/career/internal/model"
	"github.com/jason-i-magno/career/internal/prep"
	"github.com/jason-i-magno/career/internal/store"
)

func runPrep(ctx context.Context, env *Env, args []string) error {
	return dispatch(ctx, env, "prep", []subcommand{
		{"add", "add a prompt to the deck", prepAdd},
		{"answer", "write or revise a card's answer in $EDITOR", prepAnswer},
		{"due", "list what is due for review", prepDue},
		{"review", "run a review session", prepReview},
		{"list", "list the deck by subject", prepList},
		{"rm", "remove a prompt", prepRemove},
	}, args)
}

func prepAdd(ctx context.Context, env *Env, args []string) error {
	fs := newFlagSet(env, "prep add")
	var (
		subject = fs.String("subject", "", "one of: dsa, sysdes, cpp, go, distsys, cloud, behavior")
		ref     = fs.String("ref", "", "link or book reference")
		answer  = fs.String("answer", "", "the answer to reveal (optional; use `prep answer` for anything long)")
	)
	fs.Usage = func() {
		fmt.Fprintf(env.Err, "Usage: career prep add -subject <subject> <prompt...>\n\nFlags:\n")
		fs.PrintDefaults()
	}
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if *subject == "" || fs.NArg() == 0 {
		fs.Usage()
		return errors.New("need a subject and a prompt")
	}
	subj, err := model.ParseSubject(*subject)
	if err != nil {
		return err
	}

	now := env.Now()
	ease, interval, reps, due := prep.Init(now)
	id, err := env.Store.CreatePrepItem(ctx, model.PrepItem{
		Subject: subj, Prompt: strings.Join(fs.Args(), " "), Ref: *ref, Answer: *answer,
		Ease: ease, IntervalDays: interval, Reps: reps, DueAt: due,
	}, now)
	if err != nil {
		return err
	}
	fmt.Fprintf(env.Out, "%s prep #%d [%s]\n", green("added"), id, subj)
	return nil
}

func prepDue(ctx context.Context, env *Env, args []string) error {
	fs := newFlagSet(env, "prep due")
	subject := fs.String("subject", "", "filter by subject")
	limit := fs.Int("limit", 0, "cap the number shown")
	if err := parseFlags(fs, args); err != nil {
		return err
	}

	now := env.Now()
	f := store.PrepFilter{DueBy: &now, Limit: *limit}
	if *subject != "" {
		s, err := model.ParseSubject(*subject)
		if err != nil {
			return err
		}
		f.Subject = s
	}
	items, err := env.Store.ListPrepItems(ctx, f)
	if err != nil {
		return err
	}
	if len(items) == 0 {
		fmt.Fprintf(env.Out, "%s nothing due. Add prompts with `career prep add`.\n", green("clear:"))
		return nil
	}

	tw := newTable(env.Out)
	fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", dim("ID"), dim("SUBJ"), dim("PROMPT"), dim("DUE"))
	for _, p := range items {
		fmt.Fprintf(tw, "%d\t%s\t%s\t%s\n", p.ID, cyan(string(p.Subject)),
			truncate(p.Prompt, 58), dim(relDays(p.DueAt, now)))
	}
	tw.Flush()
	fmt.Fprintf(env.Out, "\n%s\n", dim(fmt.Sprintf("%d due — run `career prep review` to work through them", len(items))))
	return nil
}

func prepList(ctx context.Context, env *Env, args []string) error {
	fs := newFlagSet(env, "prep list")
	subject := fs.String("subject", "", "filter by subject")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	f := store.PrepFilter{}
	if *subject != "" {
		s, err := model.ParseSubject(*subject)
		if err != nil {
			return err
		}
		f.Subject = s
	}
	items, err := env.Store.ListPrepItems(ctx, f)
	if err != nil {
		return err
	}
	if len(items) == 0 {
		fmt.Fprintln(env.Out, dim("deck is empty — try `career seed`"))
		return nil
	}

	now := env.Now()
	tw := newTable(env.Out)
	fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
		dim("ID"), dim("SUBJ"), dim("PROMPT"), dim("REPS"), dim("EASE"), dim("DUE"))
	for _, p := range items {
		flag := ""
		if p.Answer != "" {
			flag = " " + green("✎")
		}
		if p.Leech() {
			flag += " " + red("leech")
		}
		fmt.Fprintf(tw, "%d\t%s\t%s\t%d\t%.2f\t%s%s\n", p.ID, cyan(string(p.Subject)),
			truncate(p.Prompt, 46), p.Reps, p.Ease, dim(relDays(p.DueAt, now)), flag)
	}
	tw.Flush()
	return nil
}

func prepRemove(ctx context.Context, env *Env, args []string) error {
	if len(args) != 1 {
		return errors.New("usage: career prep rm <id>")
	}
	id, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return fmt.Errorf("bad id %q", args[0])
	}
	if err := env.Store.DeletePrepItem(ctx, id); err != nil {
		return err
	}
	fmt.Fprintf(env.Out, "%s prep #%d\n", red("deleted"), id)
	return nil
}

// prepReview runs an interactive session: show a prompt, wait, reveal the
// reference, take a grade, reschedule. Answering out loud before revealing is
// the point — recognition feels like knowledge and is not.
func prepReview(ctx context.Context, env *Env, args []string) error {
	fs := newFlagSet(env, "prep review")
	var (
		subject = fs.String("subject", "", "restrict the session to one subject")
		limit   = fs.Int("limit", 20, "maximum cards in this session")
	)
	if err := parseFlags(fs, args); err != nil {
		return err
	}

	now := env.Now()
	f := store.PrepFilter{DueBy: &now, Limit: *limit}
	if *subject != "" {
		s, err := model.ParseSubject(*subject)
		if err != nil {
			return err
		}
		f.Subject = s
	}
	items, err := env.Store.ListPrepItems(ctx, f)
	if err != nil {
		return err
	}
	if len(items) == 0 {
		fmt.Fprintf(env.Out, "%s nothing due today.\n", green("clear:"))
		return nil
	}

	in := bufio.NewScanner(os.Stdin)
	fmt.Fprintf(env.Out, "\n%s %d due. Say your answer out loud before grading —\n",
		bold("Review session:"), len(items))
	fmt.Fprintf(env.Out, "%s\n", dim("silently thinking \"yes, I know that\" is how you fail the real question."))
	fmt.Fprintf(env.Out, "%s\n", dim("grade with: a=again  h=hard  g=good  e=easy  q=quit"))

	graded := 0
	for i, p := range items {
		fmt.Fprintf(env.Out, "\n%s  %s\n", dim(fmt.Sprintf("[%d/%d]", i+1, len(items))), cyan(string(p.Subject)))
		fmt.Fprintf(env.Out, "%s\n", wrap(p.Prompt, 76, ""))
		// Only promise a reveal when there is something to reveal. Cards with
		// no written answer are still perfectly good rehearsal prompts; the
		// pause is where you say the answer out loud.
		switch {
		case p.Answer != "":
			fmt.Fprintf(env.Out, "%s", dim("  answer out loud, then ↵ to check … "))
		case p.Ref != "":
			fmt.Fprintf(env.Out, "%s", dim("  ↵ to show the reference … "))
		default:
			fmt.Fprintf(env.Out, "%s", dim("  answer out loud, then ↵ … "))
		}
		if !in.Scan() {
			break
		}
		if p.Answer != "" {
			fmt.Fprintf(env.Out, "\n  %s\n", wrap(p.Answer, 74, "  "))
		}
		if p.Ref != "" {
			fmt.Fprintf(env.Out, "  %s %s\n", dim("ref:"), p.Ref)
		}

		grade, quit, err := readGrade(env, in)
		if err != nil {
			return err
		}
		if quit {
			break
		}

		updated := prep.Review(p, grade, env.Now())
		if err := env.Store.UpdatePrepItem(ctx, updated); err != nil {
			return err
		}
		graded++
		fmt.Fprintf(env.Out, "  %s next in %s\n", green("✓"), bold(fmt.Sprintf("%dd", updated.IntervalDays)))
		// A card you have now forgotten at least once is exactly the one worth
		// writing an answer for, in your own words.
		if updated.WantsAnswer() {
			fmt.Fprintf(env.Out, "  %s %s\n", yellow("↳"),
				dim(fmt.Sprintf("forgotten %s — write an answer: career prep answer %d",
					plural(updated.Lapses, "once", fmt.Sprintf("%d times", updated.Lapses)), updated.ID)))
		}
	}

	fmt.Fprintf(env.Out, "\n%s %d card(s) reviewed.\n", bold("Done."), graded)
	return nil
}

func readGrade(env *Env, in *bufio.Scanner) (model.Grade, bool, error) {
	for {
		fmt.Fprintf(env.Out, "  %s ", dim("grade [a/h/g/e/q]:"))
		if !in.Scan() {
			return 0, true, nil
		}
		switch strings.ToLower(strings.TrimSpace(in.Text())) {
		case "a", "again":
			return model.GradeAgain, false, nil
		case "h", "hard":
			return model.GradeHard, false, nil
		case "g", "good", "":
			return model.GradeGood, false, nil
		case "e", "easy":
			return model.GradeEasy, false, nil
		case "q", "quit":
			return 0, true, nil
		}
		fmt.Fprintf(env.Out, "  %s\n", yellow("enter a, h, g, e or q"))
	}
}

// nextDueSummary renders per-subject due counts for the `today` view.
func nextDueSummary(ctx context.Context, env *Env, now time.Time) (string, int, error) {
	due, total, err := env.Store.CountPrepBySubject(ctx, now)
	if err != nil {
		return "", 0, err
	}
	var parts []string
	sum := 0
	for _, s := range model.AllSubjects {
		if due[s] == 0 {
			continue
		}
		sum += due[s]
		parts = append(parts, fmt.Sprintf("%s %d/%d", cyan(string(s)), due[s], total[s]))
	}
	return strings.Join(parts, "   "), sum, nil
}

// prepAnswer opens a card's answer in $EDITOR. Answers are prose, sometimes
// several paragraphs, so they belong in an editor rather than behind a flag.
func prepAnswer(ctx context.Context, env *Env, args []string) error {
	if len(args) != 1 {
		return errors.New("usage: career prep answer <id>")
	}
	id, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return fmt.Errorf("bad id %q", args[0])
	}
	p, err := env.Store.GetPrepItem(ctx, id)
	if err != nil {
		return err
	}

	edited, err := editInEditor(renderAnswerFile(p), ".md")
	if err != nil {
		return err
	}
	p.Answer = strings.TrimSpace(stripComments(edited))

	if err := env.Store.UpdatePrepItem(ctx, p); err != nil {
		return err
	}
	if p.Answer == "" {
		fmt.Fprintf(env.Out, "%s answer on prep #%d\n", yellow("cleared"), id)
		return nil
	}
	fmt.Fprintf(env.Out, "%s answer on prep #%d [%s]\n", green("saved"), id, p.Subject)
	return nil
}

func renderAnswerFile(p model.PrepItem) string {
	ref := ""
	if p.Ref != "" {
		ref = "\n  Reference: " + p.Ref
	}
	return fmt.Sprintf(`%s
<!--
Prompt: %s
%s
Write the answer you want to see when this card comes up, in your own words.
Generating the explanation yourself is most of the benefit — copying one in
from a book gives you something to recognise rather than something to say.

Aim for what you would actually say out loud in an interview, not a textbook
definition. Leave it empty to clear the answer.

Lines in this comment block are ignored.
-->
`, p.Answer, p.Prompt, ref)
}
