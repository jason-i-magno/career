package model

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// Competency is the behavioural signal an interviewer is probing for. The set
// is deliberately small: these are the questions that actually get asked, and a
// bank with one strong story per competency beats twenty vague ones.
type Competency string

const (
	CompOwnership Competency = "ownership" // drove something end to end
	CompScale     Competency = "scale"     // performance, throughput, latency
	CompDebugging Competency = "debugging" // hardest bug you have found
	CompConflict  Competency = "conflict"  // disagreement with a colleague
	CompFailure   Competency = "failure"   // something you got wrong
	CompMentoring Competency = "mentoring" // grew someone else
	CompAmbiguity Competency = "ambiguity" // unclear requirements
	CompInfluence Competency = "influence" // change without authority
	CompDelivery  Competency = "delivery"  // shipped under a deadline
	CompCollab    Competency = "collab"    // cross-team work
)

// AllCompetencies is the full checklist. `career story gaps` reports which of
// these have no sanitized story yet.
var AllCompetencies = []Competency{
	CompOwnership, CompScale, CompDebugging, CompConflict, CompFailure,
	CompMentoring, CompAmbiguity, CompInfluence, CompDelivery, CompCollab,
}

func ParseCompetency(s string) (Competency, error) {
	c := Competency(strings.ToLower(strings.TrimSpace(s)))
	for _, k := range AllCompetencies {
		if c == k {
			return c, nil
		}
	}
	names := make([]string, len(AllCompetencies))
	for i, k := range AllCompetencies {
		names[i] = string(k)
	}
	return "", fmt.Errorf("unknown competency %q (want one of: %s)", s, strings.Join(names, ", "))
}

// Story is one STAR-format accomplishment.
//
// Sanitized is the field that matters when your best work cannot be discussed
// freely — anything under NDA, proprietary, or classified. It asserts that this
// telling contains no project or customer names, no capability specifics, and
// no figures that are sensitive in aggregate. An unsanitized story is a draft
// for your eyes only. Only sanitized stories count toward coverage, and only
// sanitized stories should ever be rehearsed out loud, because the version you
// practise is the version that comes out under pressure.
type Story struct {
	ID        int64
	Title     string
	Situation string
	Task      string
	Action    string
	Result    string

	// Metrics is the quantified outcome, stated in unclassified terms
	// ("throughput improved 3x", not an absolute figure that reveals capability).
	Metrics string

	Competencies []Competency
	Sanitized    bool

	CreatedAt time.Time
	UpdatedAt time.Time
}

// Complete reports whether all four STAR fields are filled in. A story missing
// its Result is the most common failure mode: it describes work instead of impact.
func (s Story) Complete() bool {
	return s.Situation != "" && s.Task != "" && s.Action != "" && s.Result != ""
}

// Ready reports whether the story can be used in a real interview.
func (s Story) Ready() bool { return s.Complete() && s.Sanitized }

// HasCompetency reports whether the story is tagged with c.
func (s Story) HasCompetency(c Competency) bool {
	for _, k := range s.Competencies {
		if k == c {
			return true
		}
	}
	return false
}

// CoverageGaps returns the competencies with no interview-ready story, in
// checklist order. These are the questions you currently cannot answer well.
func CoverageGaps(stories []Story) []Competency {
	covered := map[Competency]bool{}
	for _, s := range stories {
		if !s.Ready() {
			continue
		}
		for _, c := range s.Competencies {
			covered[c] = true
		}
	}
	var gaps []Competency
	for _, c := range AllCompetencies {
		if !covered[c] {
			gaps = append(gaps, c)
		}
	}
	return gaps
}

// SortCompetencies orders a tag set into canonical checklist order so that
// stored and displayed tags are stable.
func SortCompetencies(cs []Competency) []Competency {
	idx := map[Competency]int{}
	for i, c := range AllCompetencies {
		idx[c] = i
	}
	out := append([]Competency(nil), cs...)
	sort.SliceStable(out, func(i, j int) bool { return idx[out[i]] < idx[out[j]] })
	return out
}
