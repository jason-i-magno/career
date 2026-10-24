package model

import "time"

// Deadlines and review schedules are day-granular, and a day is the user's
// calendar day: it turns over at local midnight, not at midnight UTC. Times are
// stored in UTC, so these helpers take the zone from the time they are given,
// which in practice is now from time.Now.

// StartOfDay returns midnight at the start of t's calendar day, in t's zone.
func StartOfDay(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}

// DaysUntil returns how many calendar days t falls after now, both counted in
// now's zone: 0 for later today, 1 for tomorrow, -1 for yesterday.
func DaysUntil(t, now time.Time) int {
	ty, tm, td := t.In(now.Location()).Date()
	ny, nm, nd := now.Date()
	// Subtract the dates at UTC midnight, where every day is 24 hours long, so
	// that a daylight-saving change cannot shift the count.
	return int(time.Date(ty, tm, td, 0, 0, 0, 0, time.UTC).
		Sub(time.Date(ny, nm, nd, 0, 0, 0, 0, time.UTC)).Hours() / 24)
}
