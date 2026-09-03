package cli

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// parseWhen accepts an absolute date (2026-09-15), a relative offset (+7d, +2w,
// +1m), or one of a few words. It returns a time at the start of that day in
// the local zone, because every deadline this tool tracks is day-granular.
func parseWhen(s string, now time.Time) (time.Time, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	switch s {
	case "", "none", "clear":
		return time.Time{}, nil
	case "today":
		return today, nil
	case "tomorrow":
		return today.AddDate(0, 0, 1), nil
	case "monday", "mon":
		return nextWeekday(today, time.Monday), nil
	}

	if strings.HasPrefix(s, "+") {
		body := s[1:]
		if body == "" {
			return time.Time{}, fmt.Errorf("empty offset %q", s)
		}
		unit := body[len(body)-1]
		numPart := body[:len(body)-1]
		// Bare "+7" means days.
		if unit >= '0' && unit <= '9' {
			unit, numPart = 'd', body
		}
		n, err := strconv.Atoi(numPart)
		if err != nil {
			return time.Time{}, fmt.Errorf("bad offset %q: want e.g. +7d, +2w, +1m", s)
		}
		switch unit {
		case 'd':
			return today.AddDate(0, 0, n), nil
		case 'w':
			return today.AddDate(0, 0, 7*n), nil
		case 'm':
			return today.AddDate(0, n, 0), nil
		}
		return time.Time{}, fmt.Errorf("bad offset unit %q in %q: want d, w or m", string(unit), s)
	}

	t, err := time.ParseInLocation("2006-01-02", s, now.Location())
	if err != nil {
		return time.Time{}, fmt.Errorf("bad date %q: want YYYY-MM-DD, +7d, today or tomorrow", s)
	}
	return t, nil
}

func nextWeekday(from time.Time, wd time.Weekday) time.Time {
	delta := (int(wd) - int(from.Weekday()) + 7) % 7
	if delta == 0 {
		delta = 7
	}
	return from.AddDate(0, 0, delta)
}
