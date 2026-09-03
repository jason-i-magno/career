package model

import (
	"fmt"
	"strings"
	"time"
)

// Subject groups prep items into the study tracks that matter for a
// systems-C++ / backend-Go job search.
type Subject string

const (
	SubjectDSA      Subject = "dsa"      // data structures & algorithms
	SubjectSysDes   Subject = "sysdes"   // system design
	SubjectCPP      Subject = "cpp"      // C++ language & low-latency specifics
	SubjectGo       Subject = "go"       // Go language & concurrency
	SubjectDistSys  Subject = "distsys"  // distributed systems concepts
	SubjectCloud    Subject = "cloud"    // AWS / Kubernetes / Terraform
	SubjectBehavior Subject = "behavior" // behavioural questions
)

var AllSubjects = []Subject{
	SubjectDSA, SubjectSysDes, SubjectCPP, SubjectGo,
	SubjectDistSys, SubjectCloud, SubjectBehavior,
}

func ParseSubject(s string) (Subject, error) {
	sub := Subject(strings.ToLower(strings.TrimSpace(s)))
	for _, k := range AllSubjects {
		if sub == k {
			return sub, nil
		}
	}
	names := make([]string, len(AllSubjects))
	for i, k := range AllSubjects {
		names[i] = string(k)
	}
	return "", fmt.Errorf("unknown subject %q (want one of: %s)", s, strings.Join(names, ", "))
}

// Grade is a self-assessed recall score, in the Anki idiom rather than raw
// SM-2 numbers. Honest grading is the whole game: inflating "good" to "easy"
// pushes the card past the point you would actually have forgotten it.
type Grade int

const (
	GradeAgain Grade = iota // could not recall / got it wrong
	GradeHard               // recalled with real effort
	GradeGood               // recalled correctly
	GradeEasy               // instant and confident
)

func ParseGrade(s string) (Grade, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "again", "0", "fail":
		return GradeAgain, nil
	case "hard", "1":
		return GradeHard, nil
	case "good", "2", "ok", "pass":
		return GradeGood, nil
	case "easy", "3":
		return GradeEasy, nil
	}
	return 0, fmt.Errorf("unknown grade %q (want: again, hard, good, easy)", s)
}

func (g Grade) String() string {
	switch g {
	case GradeAgain:
		return "again"
	case GradeHard:
		return "hard"
	case GradeGood:
		return "good"
	case GradeEasy:
		return "easy"
	}
	return "?"
}

// PrepItem is a single thing to be recalled on a schedule: an algorithm
// pattern, a design template, a C++ memory-model rule.
//
// Ease, IntervalDays and Reps are SM-2 scheduler state; see internal/prep.
type PrepItem struct {
	ID      int64
	Subject Subject
	Prompt  string // the question, e.g. "How does a lock-free SPSC ring buffer avoid false sharing?"
	Ref     string // optional link or book/problem reference

	// Answer is what gets shown when you reveal the card. It is optional and
	// empty by default: these are open prompts rehearsed out loud, not
	// two-sided flashcards. Write one in your own words for the cards you keep
	// getting wrong — generating the answer yourself is most of the benefit,
	// and an unanswered card is still a perfectly good rehearsal prompt.
	Answer string

	Ease         float64 // SM-2 easiness factor, >= MinEase
	IntervalDays int     // current inter-repetition interval
	Reps         int     // consecutive successful reviews; reset to 0 on a lapse
	Lapses       int     // total times graded "again" after a success

	DueAt      time.Time
	LastGrade  *Grade
	ReviewedAt *time.Time
	CreatedAt  time.Time
}

// DueBy reports whether the item is due for review at or before t.
func (p PrepItem) DueBy(t time.Time) bool { return !p.DueAt.After(t) }

// New reports whether the item has never been reviewed.
func (p PrepItem) New() bool { return p.Reps == 0 && p.ReviewedAt == nil }

// Leech reports whether an item keeps being forgotten. A leech is a signal to
// rewrite the prompt or break it into smaller pieces, not to keep grinding it.
func (p PrepItem) Leech() bool { return p.Lapses >= 4 }

// WantsAnswer reports whether this card has earned a written answer: it has
// none, and it has already been forgotten at least once.
func (p PrepItem) WantsAnswer() bool { return p.Answer == "" && p.Lapses > 0 }
