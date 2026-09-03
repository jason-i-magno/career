package cli

import (
	"flag"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jason-i-magno/career/internal/model"
)

func TestParseWhen(t *testing.T) {
	now := time.Date(2026, time.September, 15, 14, 30, 0, 0, time.UTC)

	tests := []struct {
		in   string
		want string // YYYY-MM-DD, or "" for the zero time
	}{
		{"", ""},
		{"clear", ""},
		{"today", "2026-09-15"},
		{"tomorrow", "2026-09-16"},
		{"+1d", "2026-09-16"},
		{"+7", "2026-09-22"},
		{"+2w", "2026-09-29"},
		{"+1m", "2026-10-15"},
		{"2026-12-01", "2026-12-01"},
		{"TOMORROW", "2026-09-16"},
		{"  today  ", "2026-09-15"},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			got, err := parseWhen(tc.in, now)
			if err != nil {
				t.Fatalf("parseWhen(%q): %v", tc.in, err)
			}
			if tc.want == "" {
				if !got.IsZero() {
					t.Errorf("got %v, want zero time", got)
				}
				return
			}
			if got.Format("2006-01-02") != tc.want {
				t.Errorf("got %s, want %s", got.Format("2006-01-02"), tc.want)
			}
			if h, m := got.Hour(), got.Minute(); h != 0 || m != 0 {
				t.Errorf("got %02d:%02d, want start of day", h, m)
			}
		})
	}
}

func TestParseWhenRejectsGarbage(t *testing.T) {
	now := time.Now()
	for _, in := range []string{"next thursday", "+", "+3q", "15/09/2026", "yesterday"} {
		if _, err := parseWhen(in, now); err == nil {
			t.Errorf("parseWhen(%q) should have failed", in)
		}
	}
}

func TestParseWhenMondayIsAlwaysInTheFuture(t *testing.T) {
	monday := time.Date(2026, time.September, 14, 9, 0, 0, 0, time.UTC)
	if monday.Weekday() != time.Monday {
		t.Fatalf("test fixture is a %s, not Monday", monday.Weekday())
	}
	got, err := parseWhen("monday", monday)
	if err != nil {
		t.Fatal(err)
	}
	if !got.After(monday) {
		t.Errorf("got %v, want the following Monday, not today", got)
	}
}

// permute is what lets flags follow positional arguments; without it,
// `app add Acme "Staff Engineer" -track systems` silently folds the flags into
// the role name.
func TestPermuteMovesFlagsAheadOfPositionals(t *testing.T) {
	newFS := func() *flag.FlagSet {
		fs := flag.NewFlagSet("test", flag.ContinueOnError)
		fs.String("track", "", "")
		fs.Bool("remote", false, "")
		fs.Int("comp", 0, "")
		return fs
	}

	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{
			"flags after positionals",
			[]string{"Acme", "Staff Engineer", "-track", "systems"},
			[]string{"-track", "systems", "Acme", "Staff Engineer"},
		},
		{
			"bool flag consumes no value",
			[]string{"Acme", "-remote", "Staff Engineer"},
			[]string{"-remote", "Acme", "Staff Engineer"},
		},
		{
			"equals form keeps its value",
			[]string{"Acme", "-track=systems", "Role"},
			[]string{"-track=systems", "Acme", "Role"},
		},
		{
			"double dash ends flag parsing",
			[]string{"-remote", "--", "-not-a-flag"},
			[]string{"-remote", "-not-a-flag"},
		},
		{
			"already in order is unchanged",
			[]string{"-comp", "200000", "Acme", "Role"},
			[]string{"-comp", "200000", "Acme", "Role"},
		},
		{
			"no flags at all",
			[]string{"Acme", "Role"},
			[]string{"Acme", "Role"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := permute(newFS(), tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("permute(%q)\n got %q\nwant %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestPermuteThenParseBindsFlagsAndArgs(t *testing.T) {
	fs := flag.NewFlagSet("app add", flag.ContinueOnError)
	track := fs.String("track", "backend", "")
	remote := fs.Bool("remote", false, "")

	args := []string{"Cloudflare", "Systems Engineer, Network", "-track", "systems", "-remote"}
	if err := parseFlags(fs, args); err != nil {
		t.Fatalf("parseFlags: %v", err)
	}
	if *track != "systems" {
		t.Errorf("track = %q, want systems", *track)
	}
	if !*remote {
		t.Error("remote = false, want true")
	}
	if got := fs.Args(); len(got) != 2 || got[0] != "Cloudflare" {
		t.Errorf("positionals = %q, want the company and role", got)
	}
}

func TestStoryFileRoundTrip(t *testing.T) {
	want := model.Story{
		ID:        7,
		Title:     "Zero-downtime rollout",
		Situation: "Updates required a maintenance window.",
		Task:      "I owned removing the downtime.",
		Action:    "Staged the config swap behind an atomic symlink flip.",
		Result:    "Updates now ship with no interruption.",
		Metrics:   "6h window to zero",
		Competencies: []model.Competency{
			model.CompOwnership, model.CompDelivery,
		},
		Sanitized: true,
	}

	got, err := parseStoryFile(renderStoryFile(want), model.Story{ID: 7})
	if err != nil {
		t.Fatalf("parseStoryFile: %v", err)
	}

	if got.Title != want.Title || got.Situation != want.Situation ||
		got.Task != want.Task || got.Action != want.Action ||
		got.Result != want.Result || got.Metrics != want.Metrics {
		t.Errorf("text fields did not round-trip:\n got %+v\nwant %+v", got, want)
	}
	if !got.Sanitized {
		t.Error("Sanitized did not round-trip")
	}
	if !reflect.DeepEqual(got.Competencies, want.Competencies) {
		t.Errorf("competencies = %v, want %v", got.Competencies, want.Competencies)
	}
	if got.ID != 7 {
		t.Errorf("ID = %d, want the caller's 7 preserved", got.ID)
	}
}

func TestStoryFileStripsGuidanceComments(t *testing.T) {
	src := renderStoryFile(model.Story{Title: "T", Situation: "real content"})
	if !strings.Contains(src, "<!--") {
		t.Fatal("template should carry inline guidance")
	}
	got, err := parseStoryFile(src, model.Story{})
	if err != nil {
		t.Fatalf("parseStoryFile: %v", err)
	}
	for _, leak := range []string{"Fill in each section", "comma-separated", "<!--"} {
		if strings.Contains(strings.ToLower(got.Situation+got.Metrics+got.Result), strings.ToLower(leak)) {
			t.Errorf("guidance text %q leaked into the parsed story", leak)
		}
	}
	if got.Situation != "real content" {
		t.Errorf("Situation = %q, want %q", got.Situation, "real content")
	}
}

func TestStoryFileRejectsBadInput(t *testing.T) {
	tests := []struct {
		name string
		src  string
	}{
		{"no title", "## Situation\nsomething\n"},
		{"unknown competency", "# T\n## Competencies\nteleportation\n"},
		{"bad sanitized value", "# T\n## Sanitized\nmaybe\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := parseStoryFile(tc.src, model.Story{}); err == nil {
				t.Error("expected an error")
			}
		})
	}
}

func TestStoryFileEmptySanitizedDefaultsToFalse(t *testing.T) {
	got, err := parseStoryFile("# T\n## Sanitized\n\n", model.Story{})
	if err != nil {
		t.Fatalf("parseStoryFile: %v", err)
	}
	if got.Sanitized {
		t.Error("an empty Sanitized section must default to false, never true")
	}
}

func TestRelDays(t *testing.T) {
	now := time.Date(2026, time.September, 15, 14, 0, 0, 0, time.UTC)
	tests := []struct {
		offset int
		want   string
	}{
		{0, "today"}, {1, "tomorrow"}, {-1, "yesterday"},
		{5, "in 5d"}, {-9, "9d ago"},
	}
	for _, tc := range tests {
		got := relDays(now.AddDate(0, 0, tc.offset), now)
		if got != tc.want {
			t.Errorf("offset %+d: got %q, want %q", tc.offset, got, tc.want)
		}
	}
}

func TestTruncateAndWrap(t *testing.T) {
	if got := truncate("hello world", 5); got != "hell…" {
		t.Errorf("truncate = %q, want %q", got, "hell…")
	}
	if got := truncate("short", 20); got != "short" {
		t.Errorf("truncate should leave short strings alone, got %q", got)
	}
	// Multi-byte input must be cut on rune boundaries, not bytes.
	if got := truncate("naïve café", 6); len([]rune(got)) != 6 {
		t.Errorf("truncate produced %d runes, want 6", len([]rune(got)))
	}

	wrapped := wrap("one two three four five", 9, "")
	for _, line := range strings.Split(wrapped, "\n") {
		if len(line) > 9 {
			t.Errorf("line %q exceeds width 9", line)
		}
	}
}

func TestHumanUSD(t *testing.T) {
	tests := map[int]string{0: "0", 999: "999", 1000: "1,000", 195000: "195,000", 1234567: "1,234,567"}
	for in, want := range tests {
		if got := humanUSD(in); got != want {
			t.Errorf("humanUSD(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestPlural(t *testing.T) {
	if got := plural(1, "opportunity", "opportunities"); got != "opportunity" {
		t.Errorf("plural(1) = %q", got)
	}
	for _, n := range []int{0, 2, 17} {
		if got := plural(n, "lead", "leads"); got != "leads" {
			t.Errorf("plural(%d) = %q, want leads", n, got)
		}
	}
}

func TestSeedDeckIsWellFormed(t *testing.T) {
	if len(seedDeck) < 50 {
		t.Errorf("seed deck has %d cards, want a substantial starter deck", len(seedDeck))
	}
	seen := map[string]bool{}
	for _, c := range seedDeck {
		if _, err := model.ParseSubject(string(c.subject)); err != nil {
			t.Errorf("card %q has invalid subject: %v", c.prompt, err)
		}
		if strings.TrimSpace(c.prompt) == "" {
			t.Error("found a card with an empty prompt")
		}
		key := normalise(c.prompt)
		if seen[key] {
			t.Errorf("duplicate prompt in seed deck: %q", c.prompt)
		}
		seen[key] = true
	}

	// Every subject should have cards, or `prep review -subject X` is a dead end.
	bySubject := map[model.Subject]int{}
	for _, c := range seedDeck {
		bySubject[c.subject]++
	}
	for _, s := range model.AllSubjects {
		if bySubject[s] == 0 {
			t.Errorf("subject %q has no seed cards", s)
		}
	}
}

func TestSeedStoriesCoverEveryCompetency(t *testing.T) {
	covered := map[model.Competency]bool{}
	for _, s := range seedStories {
		for _, c := range s.competencies {
			covered[c] = true
		}
	}
	for _, c := range model.AllCompetencies {
		if !covered[c] {
			t.Errorf("no seed story stub targets competency %q", c)
		}
	}
}

func TestCompetencyPromptExistsForEveryCompetency(t *testing.T) {
	for _, c := range model.AllCompetencies {
		if strings.TrimSpace(competencyPrompt(c)) == "" {
			t.Errorf("competency %q has no interview prompt", c)
		}
	}
}
