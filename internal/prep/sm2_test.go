package prep

import (
	"testing"
	"time"

	"github.com/jason-i-magno/career/internal/model"
)

func day(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 9, 30, 0, 0, time.UTC)
}

func TestReviewIntervalProgression(t *testing.T) {
	// A card graded "good" every time should follow SM-2's 1, 6, then
	// interval*ease progression.
	now := day(2026, time.September, 1)
	ease, interval, reps, due := Init(now)
	item := model.PrepItem{Ease: ease, IntervalDays: interval, Reps: reps, DueAt: due}

	want := []int{1, 6, 15}
	for i, wantInterval := range want {
		item = Review(item, model.GradeGood, now)
		if item.IntervalDays != wantInterval {
			t.Errorf("review %d: interval = %d, want %d", i+1, item.IntervalDays, wantInterval)
		}
		if item.Reps != i+1 {
			t.Errorf("review %d: reps = %d, want %d", i+1, item.Reps, i+1)
		}
		now = now.AddDate(0, 0, item.IntervalDays)
	}
}

func TestReviewLapseResetsRepsAndPenalisesEase(t *testing.T) {
	now := day(2026, time.September, 1)
	ease, interval, reps, due := Init(now)
	item := model.PrepItem{Ease: ease, IntervalDays: interval, Reps: reps, DueAt: due}

	// Build up two successes, then forget it.
	item = Review(item, model.GradeGood, now)
	item = Review(item, model.GradeGood, now)
	easeBefore := item.Ease

	item = Review(item, model.GradeAgain, now)

	if item.Reps != 0 {
		t.Errorf("reps = %d after lapse, want 0", item.Reps)
	}
	if item.IntervalDays != 1 {
		t.Errorf("interval = %d after lapse, want 1", item.IntervalDays)
	}
	if item.Lapses != 1 {
		t.Errorf("lapses = %d, want 1", item.Lapses)
	}
	if item.Ease >= easeBefore {
		t.Errorf("ease = %.3f after lapse, want less than %.3f", item.Ease, easeBefore)
	}
}

func TestReviewEaseNeverFallsBelowMinimum(t *testing.T) {
	now := day(2026, time.September, 1)
	item := model.PrepItem{Ease: DefaultEase, DueAt: now}
	for i := 0; i < 50; i++ {
		item = Review(item, model.GradeAgain, now)
	}
	if item.Ease < MinEase {
		t.Errorf("ease = %.3f, want >= %.3f", item.Ease, MinEase)
	}
}

func TestReviewIntervalIsCapped(t *testing.T) {
	now := day(2026, time.September, 1)
	item := model.PrepItem{Ease: DefaultEase, IntervalDays: 1, Reps: 5, DueAt: now}
	for i := 0; i < 30; i++ {
		item = Review(item, model.GradeEasy, now)
		now = now.AddDate(0, 0, item.IntervalDays)
	}
	if item.IntervalDays > MaxIntervalDays {
		t.Errorf("interval = %d, want <= %d", item.IntervalDays, MaxIntervalDays)
	}
}

func TestReviewHardGrowsSlowerThanGood(t *testing.T) {
	now := day(2026, time.September, 1)
	base := model.PrepItem{Ease: DefaultEase, IntervalDays: 10, Reps: 3, DueAt: now}

	hard := Review(base, model.GradeHard, now)
	good := Review(base, model.GradeGood, now)

	if hard.IntervalDays >= good.IntervalDays {
		t.Errorf("hard interval %d, good interval %d: hard should be shorter",
			hard.IntervalDays, good.IntervalDays)
	}
}

func TestReviewDoesNotMutateInput(t *testing.T) {
	now := day(2026, time.September, 1)
	item := model.PrepItem{Ease: DefaultEase, IntervalDays: 6, Reps: 2, DueAt: now}
	before := item

	_ = Review(item, model.GradeEasy, now)

	if item.IntervalDays != before.IntervalDays || item.Reps != before.Reps || item.Ease != before.Ease {
		t.Error("Review mutated its input; it must return a new value")
	}
}

func TestReviewSchedulesOnDayBoundary(t *testing.T) {
	// Reviewing late in the day must not push the next due date later and
	// later with each session.
	late := time.Date(2026, time.September, 1, 23, 45, 0, 0, time.UTC)
	item := model.PrepItem{Ease: DefaultEase, DueAt: late}

	out := Review(item, model.GradeGood, late)

	if h := out.DueAt.Hour(); h != 0 {
		t.Errorf("due at hour %d, want a day boundary (0)", h)
	}
	if got, want := out.DueAt.Format("2006-01-02"), "2026-09-02"; got != want {
		t.Errorf("due = %s, want %s", got, want)
	}
}

func TestReviewRecordsGradeAndTimestamp(t *testing.T) {
	now := day(2026, time.September, 1)
	item := model.PrepItem{Ease: DefaultEase, DueAt: now}

	out := Review(item, model.GradeEasy, now)

	if out.LastGrade == nil || *out.LastGrade != model.GradeEasy {
		t.Errorf("LastGrade = %v, want easy", out.LastGrade)
	}
	if out.ReviewedAt == nil || !out.ReviewedAt.Equal(now) {
		t.Errorf("ReviewedAt = %v, want %v", out.ReviewedAt, now)
	}
}
