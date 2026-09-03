package model

import (
	"fmt"
	"strings"
	"time"
)

// Stage is a position in the hiring pipeline. Stages are ordered by progress so
// that reports can render a funnel; terminal stages sort last.
type Stage string

const (
	StageLead      Stage = "lead"      // identified, not yet applied
	StageApplied   Stage = "applied"   // application submitted
	StageScreen    Stage = "screen"    // recruiter / hiring-manager screen
	StageTech      Stage = "tech"      // technical phone screen or take-home
	StageOnsite    Stage = "onsite"    // final loop
	StageOffer     Stage = "offer"     // offer extended
	StageRejected  Stage = "rejected"  // terminal
	StageWithdrawn Stage = "withdrawn" // terminal, by our choice
)

// stageOrder drives funnel rendering and validation. Terminal stages are given
// high indices so they collect at the bottom of a report.
var stageOrder = map[Stage]int{
	StageLead: 0, StageApplied: 1, StageScreen: 2, StageTech: 3,
	StageOnsite: 4, StageOffer: 5, StageRejected: 90, StageWithdrawn: 91,
}

// ActiveStages are the stages that still represent a live opportunity.
var ActiveStages = []Stage{StageLead, StageApplied, StageScreen, StageTech, StageOnsite, StageOffer}

// AllStages is every stage in pipeline order.
var AllStages = []Stage{
	StageLead, StageApplied, StageScreen, StageTech, StageOnsite,
	StageOffer, StageRejected, StageWithdrawn,
}

func ParseStage(s string) (Stage, error) {
	st := Stage(strings.ToLower(strings.TrimSpace(s)))
	if _, ok := stageOrder[st]; !ok {
		return "", fmt.Errorf("unknown stage %q (want one of: %s)", s, JoinStages(AllStages))
	}
	return st, nil
}

func (s Stage) Order() int     { return stageOrder[s] }
func (s Stage) String() string { return string(s) }

// Terminal reports whether the stage closes out an opportunity.
func (s Stage) Terminal() bool { return s == StageRejected || s == StageWithdrawn }

// Active reports whether the opportunity is still live.
func (s Stage) Active() bool { return !s.Terminal() }

func JoinStages(ss []Stage) string {
	out := make([]string, len(ss))
	for i, s := range ss {
		out[i] = string(s)
	}
	return strings.Join(out, ", ")
}

// Source records how the opportunity was found. Tracking this is the point:
// referrals convert at multiples of cold applications, and you only learn your
// own conversion rates if you record the channel up front.
type Source string

const (
	SourceReferral  Source = "referral"
	SourceRecruiter Source = "recruiter"
	SourceBoard     Source = "board"   // LinkedIn, Ashby, Greenhouse, etc.
	SourceDirect    Source = "direct"  // applied on company careers page
	SourceNetwork   Source = "network" // conference, meetup, ex-colleague
)

var AllSources = []Source{SourceReferral, SourceRecruiter, SourceBoard, SourceDirect, SourceNetwork}

func ParseSource(s string) (Source, error) {
	src := Source(strings.ToLower(strings.TrimSpace(s)))
	for _, k := range AllSources {
		if src == k {
			return src, nil
		}
	}
	return "", fmt.Errorf("unknown source %q (want one of: referral, recruiter, board, direct, network)", s)
}

// Track is which of the two target role families this opportunity belongs to.
// Keeping them separate stops one track's volume from masking the other's
// conversion rate.
type Track string

const (
	TrackSystems Track = "systems" // low-latency / systems C++
	TrackBackend Track = "backend" // backend & distributed systems
	TrackOther   Track = "other"
)

var AllTracks = []Track{TrackSystems, TrackBackend, TrackOther}

func ParseTrack(s string) (Track, error) {
	t := Track(strings.ToLower(strings.TrimSpace(s)))
	for _, k := range AllTracks {
		if t == k {
			return t, nil
		}
	}
	return "", fmt.Errorf("unknown track %q (want one of: systems, backend, other)", s)
}

// Application is a single opportunity moving through the pipeline.
type Application struct {
	ID       int64
	Company  string
	Role     string
	Stage    Stage
	Track    Track
	Source   Source
	Location string
	Remote   bool
	URL      string

	// Contact is the recruiter or referrer to follow up with.
	Contact string

	// BaseComp is the base salary in whole dollars, 0 when unknown.
	BaseComp int

	CreatedAt time.Time
	UpdatedAt time.Time

	// NextAction is a short imperative ("send follow-up", "prep sys design")
	// and NextActionAt is when it comes due. Together they drive `career today`.
	NextAction   string
	NextActionAt *time.Time
}

// Stale reports whether an active application has gone untouched for longer
// than d. Stale applications are the ones that quietly die without a nudge.
func (a Application) Stale(now time.Time, d time.Duration) bool {
	if a.Stage.Terminal() || a.Stage == StageLead {
		return false
	}
	return now.Sub(a.UpdatedAt) > d
}

// Due reports whether the next action is due on or before now.
func (a Application) Due(now time.Time) bool {
	return a.NextActionAt != nil && !a.NextActionAt.After(now)
}

// EventKind classifies entries in an application's activity log.
type EventKind string

const (
	EventNote      EventKind = "note"
	EventStage     EventKind = "stage"
	EventContact   EventKind = "contact"
	EventInterview EventKind = "interview"
)

// Event is one timestamped entry in an application's history. The log is
// append-only: it is the record you reread before an interview.
type Event struct {
	ID            int64
	ApplicationID int64
	At            time.Time
	Kind          EventKind
	Body          string
}
