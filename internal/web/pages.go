package web

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/jason-i-magno/career/internal/model"
	"github.com/jason-i-magno/career/internal/prep"
	"github.com/jason-i-magno/career/internal/store"
)

// base is what the layout needs from every page.
type base struct {
	Title string
	Tab   string // the navigation tab to mark as current
}

type subjectCount struct {
	Subject    model.Subject
	Due, Total int
}

// todayPage mirrors `career today`.
type todayPage struct {
	base
	Now          time.Time
	Due, Stale   []model.Application
	PrepDue      int
	PrepSubjects []subjectCount
	Gaps         []model.Competency
	Live, Leads  int
}

func (s *Server) today(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	now := s.now()

	apps, err := s.store.ListApplications(ctx, store.ApplicationFilter{ActiveOnly: true})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	subjects, prepDue, err := s.prepCounts(ctx, now)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	stories, err := s.store.ListStories(ctx, store.StoryFilter{})
	if err != nil {
		s.fail(w, r, err)
		return
	}

	p := todayPage{
		base:         base{Title: "Today", Tab: "today"},
		Now:          now,
		PrepDue:      prepDue,
		PrepSubjects: subjects,
		Gaps:         model.CoverageGaps(stories),
	}
	p.Due, p.Stale = model.Triage(apps, now)
	for _, a := range apps {
		if a.Stage == model.StageLead {
			p.Leads++
		} else {
			p.Live++
		}
	}
	s.render(w, http.StatusOK, "today", p)
}

// prepCounts returns the subjects that have cards due, in deck order, and the
// total due across all of them.
func (s *Server) prepCounts(ctx context.Context, now time.Time) ([]subjectCount, int, error) {
	due, total, err := s.store.CountPrepBySubject(ctx, now)
	if err != nil {
		return nil, 0, err
	}
	var out []subjectCount
	sum := 0
	for _, sub := range model.AllSubjects {
		if due[sub] == 0 {
			continue
		}
		out = append(out, subjectCount{Subject: sub, Due: due[sub], Total: total[sub]})
		sum += due[sub]
	}
	return out, sum, nil
}

// prepPage is one step of a review: the next due card, or the news that there
// is none. There is no session state to keep; the schedule is the session.
type prepPage struct {
	base
	Subject   model.Subject // the filter; empty for every subject
	Subjects  []subjectCount
	Card      *model.PrepItem
	Remaining int
	Last      *model.PrepItem // the card just graded, to confirm its new interval
}

func (s *Server) prepPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	now := s.now()

	subject, err := subjectParam(r)
	if err != nil {
		s.badRequest(w, err)
		return
	}
	subjects, remaining, err := s.prepCounts(ctx, now)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if subject != "" {
		remaining = 0
		for _, c := range subjects {
			if c.Subject == subject {
				remaining = c.Due
			}
		}
	}
	items, err := s.store.ListPrepItems(ctx, store.PrepFilter{Subject: subject, DueBy: &now, Limit: 1})
	if err != nil {
		s.fail(w, r, err)
		return
	}

	p := prepPage{
		base:      base{Title: "Prep", Tab: "prep"},
		Subject:   subject,
		Subjects:  subjects,
		Remaining: remaining,
	}
	if len(items) > 0 {
		p.Card = &items[0]
	}
	if id, err := strconv.ParseInt(r.URL.Query().Get("graded"), 10, 64); err == nil {
		switch last, err := s.store.GetPrepItem(ctx, id); {
		case err == nil:
			p.Last = &last
		case !errors.Is(err, store.ErrNotFound):
			s.fail(w, r, err)
			return
		}
	}
	s.render(w, http.StatusOK, "prep", p)
}

// prepGrade records a grade, then sends the browser on to the next card.
//
// A card is graded only while it is due. Every grade schedules the card at
// least a day out, so a resubmitted form — a double tap, or the back button and
// a second tap — finds it no longer due and changes nothing, rather than
// reviewing it twice and pushing it out further than one honest grade earns.
func (s *Server) prepGrade(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	id, err := pathID(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	grade, err := model.ParseGrade(r.PostFormValue("grade"))
	if err != nil {
		s.badRequest(w, err)
		return
	}
	subject, err := subjectParam(r)
	if err != nil {
		s.badRequest(w, err)
		return
	}
	p, err := s.store.GetPrepItem(ctx, id)
	if err != nil {
		s.fail(w, r, err)
		return
	}

	if now := s.now(); p.DueBy(now) {
		if err := s.store.UpdatePrepItem(ctx, prep.Review(p, grade, now)); err != nil {
			s.fail(w, r, err)
			return
		}
	}

	q := url.Values{"graded": {strconv.FormatInt(id, 10)}}
	if subject != "" {
		q.Set("subject", string(subject))
	}
	http.Redirect(w, r, "/prep?"+q.Encode(), http.StatusSeeOther)
}

// subjectParam reads the optional subject filter.
func subjectParam(r *http.Request) (model.Subject, error) {
	raw := r.FormValue("subject")
	if raw == "" {
		return "", nil
	}
	return model.ParseSubject(raw)
}

type appsPage struct {
	base
	All  bool
	Apps []model.Application
}

func (s *Server) appList(w http.ResponseWriter, r *http.Request) {
	all := r.URL.Query().Get("all") != ""
	apps, err := s.store.ListApplications(r.Context(), store.ApplicationFilter{ActiveOnly: !all})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, http.StatusOK, "apps", appsPage{
		base: base{Title: "Pipeline", Tab: "apps"},
		All:  all,
		Apps: apps,
	})
}

type appPage struct {
	base
	Now      time.Time
	App      model.Application
	Location string
	Events   []model.Event
}

func (s *Server) appShow(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := pathID(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	a, err := s.store.GetApplication(ctx, id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	events, err := s.store.ListEvents(ctx, id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, http.StatusOK, "app", appPage{
		base:     base{Title: a.Company, Tab: "apps"},
		Now:      s.now(),
		App:      a,
		Location: location(a),
		Events:   events,
	})
}

// location combines office and remote the way `career app show` does.
func location(a model.Application) string {
	switch {
	case !a.Remote:
		return a.Location
	case a.Location == "":
		return "remote"
	default:
		return a.Location + " (remote)"
	}
}

type storiesPage struct {
	base
	Stories []model.Story
	Gaps    []model.Competency
}

func (s *Server) storyList(w http.ResponseWriter, r *http.Request) {
	stories, err := s.store.ListStories(r.Context(), store.StoryFilter{})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, http.StatusOK, "stories", storiesPage{
		base:    base{Title: "Stories", Tab: "stories"},
		Stories: stories,
		Gaps:    model.CoverageGaps(stories),
	})
}

type storyPage struct {
	base
	Story    model.Story
	Sections []section // empty unless the story is sanitized
}

type section struct{ Name, Body string }

// storyShow shows a story's STAR sections, but only once it is marked
// sanitized. An unsanitized story is a draft that may still name programs or
// customers, and a phone gets read in more places than a terminal does.
func (s *Server) storyShow(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	st, err := s.store.GetStory(r.Context(), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	p := storyPage{base: base{Title: st.Title, Tab: "stories"}, Story: st}
	if st.Sanitized {
		p.Sections = []section{
			{"Situation", st.Situation}, {"Task", st.Task}, {"Action", st.Action},
			{"Result", st.Result}, {"Metrics", st.Metrics},
		}
	}
	s.render(w, http.StatusOK, "story", p)
}
