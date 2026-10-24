package web

import (
	"fmt"
	"html/template"
	"strconv"
	"strings"
	"time"

	"github.com/jason-i-magno/career/internal/model"
)

var funcs = template.FuncMap{
	"rel":    relDays,
	"usd":    usd,
	"plural": plural,
	"isURL":  isURL,
	"ready":  readiness,
	"comps":  joinCompetencies,
}

// relDays renders t relative to now in whole local calendar days: "today",
// "in 3d", "5d ago".
func relDays(t, now time.Time) string {
	switch days := model.DaysUntil(t, now); {
	case days == 0:
		return "today"
	case days == 1:
		return "tomorrow"
	case days == -1:
		return "yesterday"
	case days > 1:
		return fmt.Sprintf("in %dd", days)
	default:
		return fmt.Sprintf("%dd ago", -days)
	}
}

// usd groups a whole-dollar amount in thousands: 195000 → "195,000".
func usd(n int) string {
	s := strconv.Itoa(n)
	var out []byte
	for i, c := range []byte(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, c)
	}
	return string(out)
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// isURL reports whether a reference should be rendered as a link.
func isURL(s string) bool {
	return strings.HasPrefix(s, "https://") || strings.HasPrefix(s, "http://")
}

// readinessLabel says why a story is or is not usable in an interview, in the
// same words as `career story list`. Class picks its colour.
type readinessLabel struct{ Label, Class string }

func readiness(s model.Story) readinessLabel {
	switch {
	case s.Ready():
		return readinessLabel{"ready", "ok"}
	case !s.Complete() && !s.Sanitized:
		return readinessLabel{"incomplete, unsanitized", "bad"}
	case !s.Complete():
		return readinessLabel{"incomplete", "warn"}
	default:
		return readinessLabel{"unsanitized", "warn"}
	}
}

func joinCompetencies(cs []model.Competency) string {
	names := make([]string, len(cs))
	for i, c := range cs {
		names[i] = string(c)
	}
	return strings.Join(names, " · ")
}
