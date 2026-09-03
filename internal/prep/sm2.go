// Package prep implements the SM-2 spaced-repetition scheduler.
//
// SM-2 (Wozniak, 1987) schedules each item by an "easiness factor" that rises
// when recall is effortless and falls when it is hard, multiplying the gap
// between reviews so that items you know drop out of the rotation while items
// you do not keep coming back.
//
// The scheduler is a pure function of (item state, grade, now), which keeps it
// trivially testable and keeps all clock and storage concerns in the caller.
package prep

import (
	"math"
	"time"

	"github.com/jason-i-magno/career/internal/model"
)

const (
	// DefaultEase is the SM-2 starting easiness factor.
	DefaultEase = 2.5
	// MinEase is the floor; below this, intervals collapse and the item
	// should be rewritten rather than rescheduled.
	MinEase = 1.3
	// MaxIntervalDays caps growth so nothing disappears for years. Six months
	// is long enough to be out of the way, short enough to catch decay before
	// an interview loop.
	MaxIntervalDays = 180
)

// gradeQuality maps the four-button grade onto the 0..5 SM-2 quality scale.
// GradeAgain is a lapse; the rest are successes of differing effort.
var gradeQuality = map[model.Grade]int{
	model.GradeAgain: 2,
	model.GradeHard:  3,
	model.GradeGood:  4,
	model.GradeEasy:  5,
}

// Init returns the scheduler state for a freshly created item: due immediately,
// at the default ease, with no review history.
func Init(now time.Time) (ease float64, intervalDays, reps int, dueAt time.Time) {
	return DefaultEase, 0, 0, now
}

// Review applies grade to item at time now and returns the updated item.
//
// The input is not mutated. On a lapse (GradeAgain) the repetition count resets
// and the item returns tomorrow; ease is still penalised, so repeatedly
// forgetting an item permanently shortens its intervals.
func Review(item model.PrepItem, grade model.Grade, now time.Time) model.PrepItem {
	q := gradeQuality[grade]
	out := item

	if out.Ease == 0 {
		out.Ease = DefaultEase
	}

	// SM-2 ease update, applied on every review including lapses.
	delta := 0.1 - float64(5-q)*(0.08+float64(5-q)*0.02)
	out.Ease = math.Max(MinEase, out.Ease+delta)

	if grade == model.GradeAgain {
		if out.Reps > 0 {
			out.Lapses++
		}
		out.Reps = 0
		out.IntervalDays = 1
	} else {
		out.Reps++
		switch out.Reps {
		case 1:
			out.IntervalDays = 1
		case 2:
			out.IntervalDays = 6
		default:
			next := float64(out.IntervalDays) * out.Ease
			if grade == model.GradeHard {
				// Hard recalls grow more slowly than the raw ease implies.
				next = float64(out.IntervalDays) * 1.2
			}
			out.IntervalDays = int(math.Round(next))
		}
		if out.IntervalDays > MaxIntervalDays {
			out.IntervalDays = MaxIntervalDays
		}
	}

	reviewed := now
	out.ReviewedAt = &reviewed
	g := grade
	out.LastGrade = &g
	// Schedule from the start of the review day so that due dates land on day
	// boundaries rather than drifting later with each session.
	day := now.Truncate(24 * time.Hour)
	out.DueAt = day.AddDate(0, 0, out.IntervalDays)

	return out
}
