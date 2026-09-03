package model

import (
	"testing"
	"time"
)

func TestCoverageGapsCountsOnlyReadyStories(t *testing.T) {
	complete := Story{
		Situation: "s", Task: "t", Action: "a", Result: "r",
		Competencies: []Competency{CompOwnership},
	}

	tests := []struct {
		name    string
		stories []Story
		covered Competency
		want    bool // want covered
	}{
		{"sanitized and complete counts", []Story{withSanitized(complete, true)}, CompOwnership, true},
		{"unsanitized does not count", []Story{withSanitized(complete, false)}, CompOwnership, false},
		{"incomplete does not count", []Story{{Situation: "s", Sanitized: true,
			Competencies: []Competency{CompOwnership}}}, CompOwnership, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gaps := CoverageGaps(tc.stories)
			isGap := false
			for _, g := range gaps {
				if g == tc.covered {
					isGap = true
				}
			}
			if covered := !isGap; covered != tc.want {
				t.Errorf("covered = %v, want %v", covered, tc.want)
			}
		})
	}
}

func withSanitized(s Story, v bool) Story { s.Sanitized = v; return s }

func TestCoverageGapsEmptyBankReportsEverything(t *testing.T) {
	if got, want := len(CoverageGaps(nil)), len(AllCompetencies); got != want {
		t.Errorf("gaps = %d, want %d", got, want)
	}
}

func TestStoryReadyRequiresBothCompletenessAndSanitization(t *testing.T) {
	full := Story{Situation: "s", Task: "t", Action: "a", Result: "r", Sanitized: true}
	if !full.Ready() {
		t.Error("complete + sanitized story should be ready")
	}
	noResult := full
	noResult.Result = ""
	if noResult.Ready() {
		t.Error("story missing Result must not be ready")
	}
}

func TestApplicationStale(t *testing.T) {
	now := time.Date(2026, time.September, 20, 12, 0, 0, 0, time.UTC)
	const window = 14 * 24 * time.Hour

	tests := []struct {
		name    string
		stage   Stage
		updated time.Time
		want    bool
	}{
		{"recently touched is fresh", StageApplied, now.AddDate(0, 0, -3), false},
		{"long untouched is stale", StageApplied, now.AddDate(0, 0, -20), true},
		{"leads are never stale", StageLead, now.AddDate(0, 0, -90), false},
		{"rejected is never stale", StageRejected, now.AddDate(0, 0, -90), false},
		{"withdrawn is never stale", StageWithdrawn, now.AddDate(0, 0, -90), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a := Application{Stage: tc.stage, UpdatedAt: tc.updated}
			if got := a.Stale(now, window); got != tc.want {
				t.Errorf("Stale = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestApplicationDue(t *testing.T) {
	now := time.Date(2026, time.September, 20, 12, 0, 0, 0, time.UTC)
	past, future := now.AddDate(0, 0, -1), now.AddDate(0, 0, 1)

	if (Application{}).Due(now) {
		t.Error("application with no next action must not be due")
	}
	if !(Application{NextActionAt: &past}).Due(now) {
		t.Error("overdue action should be due")
	}
	if (Application{NextActionAt: &future}).Due(now) {
		t.Error("future action should not be due")
	}
}

func TestStageOrderPlacesTerminalStagesLast(t *testing.T) {
	for _, s := range []Stage{StageRejected, StageWithdrawn} {
		if s.Order() <= StageOffer.Order() {
			t.Errorf("%s should sort after offer", s)
		}
		if !s.Terminal() {
			t.Errorf("%s should be terminal", s)
		}
	}
	if StageOffer.Terminal() {
		t.Error("offer must not be terminal: it is still a live opportunity")
	}
}

func TestParsers(t *testing.T) {
	if _, err := ParseStage("APPLIED "); err != nil {
		t.Errorf("ParseStage should be case- and space-insensitive: %v", err)
	}
	if _, err := ParseStage("nonsense"); err == nil {
		t.Error("ParseStage should reject unknown stages")
	}
	if _, err := ParseTrack("systems"); err != nil {
		t.Errorf("ParseTrack(systems): %v", err)
	}
	if _, err := ParseSource("referral"); err != nil {
		t.Errorf("ParseSource(referral): %v", err)
	}
	if _, err := ParseCompetency("debugging"); err != nil {
		t.Errorf("ParseCompetency(debugging): %v", err)
	}
	if _, err := ParseSubject("cpp"); err != nil {
		t.Errorf("ParseSubject(cpp): %v", err)
	}
}

func TestParseGradeAcceptsWordsAndNumbers(t *testing.T) {
	tests := map[string]Grade{
		"again": GradeAgain, "0": GradeAgain,
		"hard": GradeHard, "good": GradeGood, "pass": GradeGood, "easy": GradeEasy,
	}
	for in, want := range tests {
		got, err := ParseGrade(in)
		if err != nil {
			t.Errorf("ParseGrade(%q): %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("ParseGrade(%q) = %v, want %v", in, got, want)
		}
	}
	if _, err := ParseGrade("brilliant"); err == nil {
		t.Error("ParseGrade should reject unknown grades")
	}
}

func TestSortCompetenciesIsCanonical(t *testing.T) {
	in := []Competency{CompCollab, CompOwnership, CompFailure}
	got := SortCompetencies(in)
	if got[0] != CompOwnership {
		t.Errorf("first = %v, want ownership (checklist order)", got[0])
	}
	if in[0] != CompCollab {
		t.Error("SortCompetencies must not mutate its input")
	}
}

func TestPrepItemLeechThreshold(t *testing.T) {
	if (PrepItem{Lapses: 3}).Leech() {
		t.Error("3 lapses should not yet be a leech")
	}
	if !(PrepItem{Lapses: 4}).Leech() {
		t.Error("4 lapses should be a leech")
	}
}

func TestPrepItemWantsAnswer(t *testing.T) {
	tests := []struct {
		name string
		item PrepItem
		want bool
	}{
		{"new card with no answer", PrepItem{}, false},
		{"forgotten once, no answer", PrepItem{Lapses: 1}, true},
		{"forgotten but already answered", PrepItem{Lapses: 3, Answer: "because…"}, false},
		{"answered, never forgotten", PrepItem{Answer: "because…"}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.item.WantsAnswer(); got != tc.want {
				t.Errorf("WantsAnswer = %v, want %v", got, tc.want)
			}
		})
	}
}
