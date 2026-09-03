package store

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/jason-i-magno/career/internal/model"
)

func newTestStore(t *testing.T) (*Store, context.Context) {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s, context.Background()
}

var testNow = time.Date(2026, time.September, 15, 10, 0, 0, 0, time.UTC)

func TestApplicationRoundTrip(t *testing.T) {
	s, ctx := newTestStore(t)
	due := testNow.AddDate(0, 0, 3)

	want := model.Application{
		Company: "Cloudflare", Role: "Systems Engineer", Stage: model.StageApplied,
		Track: model.TrackSystems, Source: model.SourceReferral,
		Location: "Austin", Remote: true, URL: "https://example.invalid/job",
		Contact: "a former colleague", BaseComp: 210000,
		NextAction: "follow up", NextActionAt: &due,
	}

	id, err := s.CreateApplication(ctx, want, testNow)
	if err != nil {
		t.Fatalf("CreateApplication: %v", err)
	}

	got, err := s.GetApplication(ctx, id)
	if err != nil {
		t.Fatalf("GetApplication: %v", err)
	}

	if got.Company != want.Company || got.Role != want.Role {
		t.Errorf("identity = %q/%q, want %q/%q", got.Company, got.Role, want.Company, want.Role)
	}
	if got.Stage != want.Stage || got.Track != want.Track || got.Source != want.Source {
		t.Errorf("enums = %s/%s/%s, want %s/%s/%s",
			got.Stage, got.Track, got.Source, want.Stage, want.Track, want.Source)
	}
	if !got.Remote || got.BaseComp != want.BaseComp || got.Contact != want.Contact {
		t.Errorf("scalar fields did not round-trip: %+v", got)
	}
	if got.NextActionAt == nil || !got.NextActionAt.Equal(due) {
		t.Errorf("NextActionAt = %v, want %v", got.NextActionAt, due)
	}
	if !got.CreatedAt.Equal(testNow) {
		t.Errorf("CreatedAt = %v, want %v", got.CreatedAt, testNow)
	}
}

func TestGetApplicationMissingReturnsErrNotFound(t *testing.T) {
	s, ctx := newTestStore(t)
	_, err := s.GetApplication(ctx, 404)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestUpdateApplicationMissingReturnsErrNotFound(t *testing.T) {
	s, ctx := newTestStore(t)
	err := s.UpdateApplication(ctx, model.Application{ID: 404, Stage: model.StageLead}, testNow)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestNilNextActionRoundTrips(t *testing.T) {
	s, ctx := newTestStore(t)
	id, err := s.CreateApplication(ctx, model.Application{
		Company: "Acme", Role: "Engineer", Stage: model.StageLead,
		Track: model.TrackOther, Source: model.SourceBoard,
	}, testNow)
	if err != nil {
		t.Fatalf("CreateApplication: %v", err)
	}
	got, err := s.GetApplication(ctx, id)
	if err != nil {
		t.Fatalf("GetApplication: %v", err)
	}
	if got.NextActionAt != nil {
		t.Errorf("NextActionAt = %v, want nil", got.NextActionAt)
	}
}

func TestListApplicationsFilters(t *testing.T) {
	s, ctx := newTestStore(t)
	seed := []model.Application{
		{Company: "Alpha", Role: "A", Stage: model.StageApplied, Track: model.TrackSystems, Source: model.SourceBoard},
		{Company: "Beta", Role: "B", Stage: model.StageRejected, Track: model.TrackSystems, Source: model.SourceBoard},
		{Company: "Gamma", Role: "G", Stage: model.StageOnsite, Track: model.TrackBackend, Source: model.SourceReferral},
	}
	for _, a := range seed {
		if _, err := s.CreateApplication(ctx, a, testNow); err != nil {
			t.Fatalf("CreateApplication(%s): %v", a.Company, err)
		}
	}

	tests := []struct {
		name   string
		filter ApplicationFilter
		want   int
	}{
		{"all", ApplicationFilter{}, 3},
		{"active only excludes rejected", ApplicationFilter{ActiveOnly: true}, 2},
		{"by track", ApplicationFilter{Track: model.TrackSystems}, 2},
		{"by stage", ApplicationFilter{Stages: []model.Stage{model.StageOnsite}}, 1},
		{"company substring is case-insensitive", ApplicationFilter{Company: "amm"}, 1},
		{"no match", ApplicationFilter{Company: "nonexistent"}, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := s.ListApplications(ctx, tc.filter)
			if err != nil {
				t.Fatalf("ListApplications: %v", err)
			}
			if len(got) != tc.want {
				t.Errorf("got %d applications, want %d", len(got), tc.want)
			}
		})
	}
}

func TestListApplicationsOrdersByPipelineStage(t *testing.T) {
	s, ctx := newTestStore(t)
	// Insert out of pipeline order.
	for _, a := range []model.Application{
		{Company: "Late", Role: "r", Stage: model.StageOnsite, Track: model.TrackBackend, Source: model.SourceBoard},
		{Company: "Early", Role: "r", Stage: model.StageLead, Track: model.TrackBackend, Source: model.SourceBoard},
	} {
		if _, err := s.CreateApplication(ctx, a, testNow); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.ListApplications(ctx, ApplicationFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Company != "Early" {
		t.Errorf("first = %s, want Early (lead sorts before onsite)", got[0].Company)
	}
}

func TestEventsCascadeOnApplicationDelete(t *testing.T) {
	s, ctx := newTestStore(t)
	id, err := s.CreateApplication(ctx, model.Application{
		Company: "Acme", Role: "Engineer", Stage: model.StageApplied,
		Track: model.TrackBackend, Source: model.SourceBoard,
	}, testNow)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddEvent(ctx, model.Event{
		ApplicationID: id, At: testNow, Kind: model.EventNote, Body: "spoke to recruiter",
	}); err != nil {
		t.Fatalf("AddEvent: %v", err)
	}

	events, err := s.ListEvents(ctx, id)
	if err != nil || len(events) != 1 {
		t.Fatalf("ListEvents = %d events, %v; want 1, nil", len(events), err)
	}

	if err := s.DeleteApplication(ctx, id); err != nil {
		t.Fatalf("DeleteApplication: %v", err)
	}
	events, err = s.ListEvents(ctx, id)
	if err != nil {
		t.Fatalf("ListEvents after delete: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("got %d orphaned events, want 0 (foreign key cascade)", len(events))
	}
}

func TestStoryRoundTripWithCompetencies(t *testing.T) {
	s, ctx := newTestStore(t)
	want := model.Story{
		Title: "Zero-downtime rollout", Situation: "s", Task: "t", Action: "a", Result: "r",
		Metrics:      "cut deploy window from 6h to 0",
		Competencies: []model.Competency{model.CompDelivery, model.CompOwnership},
		Sanitized:    true,
	}
	id, err := s.CreateStory(ctx, want, testNow)
	if err != nil {
		t.Fatalf("CreateStory: %v", err)
	}
	got, err := s.GetStory(ctx, id)
	if err != nil {
		t.Fatalf("GetStory: %v", err)
	}
	if !got.Ready() {
		t.Error("round-tripped story should be interview-ready")
	}
	if len(got.Competencies) != 2 {
		t.Fatalf("competencies = %v, want 2", got.Competencies)
	}
	// Stored tags come back in canonical checklist order, not insertion order.
	if got.Competencies[0] != model.CompOwnership {
		t.Errorf("first competency = %v, want ownership", got.Competencies[0])
	}
}

func TestUpdateStoryReplacesCompetencySet(t *testing.T) {
	s, ctx := newTestStore(t)
	id, err := s.CreateStory(ctx, model.Story{
		Title: "T", Competencies: []model.Competency{model.CompScale, model.CompDebugging},
	}, testNow)
	if err != nil {
		t.Fatal(err)
	}
	st, err := s.GetStory(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	st.Competencies = []model.Competency{model.CompMentoring}
	if err := s.UpdateStory(ctx, st, testNow); err != nil {
		t.Fatalf("UpdateStory: %v", err)
	}
	got, err := s.GetStory(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Competencies) != 1 || got.Competencies[0] != model.CompMentoring {
		t.Errorf("competencies = %v, want [mentoring] only", got.Competencies)
	}
}

func TestListStoriesReadyOnlyAndByCompetency(t *testing.T) {
	s, ctx := newTestStore(t)
	ready := model.Story{Title: "Ready", Situation: "s", Task: "t", Action: "a", Result: "r",
		Sanitized: true, Competencies: []model.Competency{model.CompScale}}
	draft := model.Story{Title: "Draft", Situation: "s",
		Competencies: []model.Competency{model.CompScale}}
	for _, st := range []model.Story{ready, draft} {
		if _, err := s.CreateStory(ctx, st, testNow); err != nil {
			t.Fatal(err)
		}
	}

	all, err := s.ListStories(ctx, StoryFilter{})
	if err != nil || len(all) != 2 {
		t.Fatalf("ListStories = %d, %v; want 2, nil", len(all), err)
	}
	onlyReady, err := s.ListStories(ctx, StoryFilter{ReadyOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(onlyReady) != 1 || onlyReady[0].Title != "Ready" {
		t.Errorf("ReadyOnly returned %d stories, want just \"Ready\"", len(onlyReady))
	}
	byComp, err := s.ListStories(ctx, StoryFilter{Competency: model.CompScale})
	if err != nil || len(byComp) != 2 {
		t.Errorf("by competency = %d, %v; want 2, nil", len(byComp), err)
	}
	none, err := s.ListStories(ctx, StoryFilter{Competency: model.CompConflict})
	if err != nil || len(none) != 0 {
		t.Errorf("unmatched competency = %d, %v; want 0, nil", len(none), err)
	}
}

func TestPrepItemRoundTripAndDueFilter(t *testing.T) {
	s, ctx := newTestStore(t)
	overdue := model.PrepItem{Subject: model.SubjectCPP, Prompt: "false sharing?",
		Ease: 2.5, DueAt: testNow.AddDate(0, 0, -1)}
	future := model.PrepItem{Subject: model.SubjectGo, Prompt: "GMP model?",
		Ease: 2.5, DueAt: testNow.AddDate(0, 0, 5)}
	for _, p := range []model.PrepItem{overdue, future} {
		if _, err := s.CreatePrepItem(ctx, p, testNow); err != nil {
			t.Fatal(err)
		}
	}

	due, err := s.ListPrepItems(ctx, PrepFilter{DueBy: &testNow})
	if err != nil {
		t.Fatal(err)
	}
	if len(due) != 1 || due[0].Prompt != "false sharing?" {
		t.Errorf("due = %d items, want 1 (the overdue one)", len(due))
	}

	bySubject, err := s.ListPrepItems(ctx, PrepFilter{Subject: model.SubjectGo})
	if err != nil || len(bySubject) != 1 {
		t.Errorf("by subject = %d, %v; want 1, nil", len(bySubject), err)
	}

	dueCounts, totals, err := s.CountPrepBySubject(ctx, testNow)
	if err != nil {
		t.Fatal(err)
	}
	if dueCounts[model.SubjectCPP] != 1 || dueCounts[model.SubjectGo] != 0 {
		t.Errorf("due counts = %v, want cpp:1 go:0", dueCounts)
	}
	if totals[model.SubjectGo] != 1 {
		t.Errorf("go total = %d, want 1", totals[model.SubjectGo])
	}
}

func TestPrepItemPersistsSchedulerState(t *testing.T) {
	s, ctx := newTestStore(t)
	id, err := s.CreatePrepItem(ctx, model.PrepItem{
		Subject: model.SubjectDSA, Prompt: "sliding window?", Ease: 2.5, DueAt: testNow,
	}, testNow)
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.GetPrepItem(ctx, id)
	if err != nil {
		t.Fatal(err)
	}

	grade := model.GradeHard
	reviewed := testNow
	p.Ease, p.IntervalDays, p.Reps, p.Lapses = 2.36, 12, 4, 2
	p.LastGrade, p.ReviewedAt = &grade, &reviewed
	p.DueAt = testNow.AddDate(0, 0, 12)

	if err := s.UpdatePrepItem(ctx, p); err != nil {
		t.Fatalf("UpdatePrepItem: %v", err)
	}
	got, err := s.GetPrepItem(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if got.Ease != 2.36 || got.IntervalDays != 12 || got.Reps != 4 || got.Lapses != 2 {
		t.Errorf("scheduler state did not persist: %+v", got)
	}
	if got.LastGrade == nil || *got.LastGrade != model.GradeHard {
		t.Errorf("LastGrade = %v, want hard", got.LastGrade)
	}
	if got.ReviewedAt == nil || !got.ReviewedAt.Equal(reviewed) {
		t.Errorf("ReviewedAt = %v, want %v", got.ReviewedAt, reviewed)
	}
}

func TestMigrationsAreIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "reopen.db")
	first, err := Open(path)
	if err != nil {
		t.Fatalf("first Open: %v", err)
	}
	ctx := context.Background()
	if _, err := first.CreateApplication(ctx, model.Application{
		Company: "Acme", Role: "Engineer", Stage: model.StageLead,
		Track: model.TrackBackend, Source: model.SourceBoard,
	}, testNow); err != nil {
		t.Fatal(err)
	}
	first.Close()

	// Reopening must re-run no migrations and preserve the data.
	second, err := Open(path)
	if err != nil {
		t.Fatalf("second Open: %v", err)
	}
	defer second.Close()

	apps, err := second.ListApplications(ctx, ApplicationFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(apps) != 1 {
		t.Errorf("got %d applications after reopen, want 1", len(apps))
	}
}

// TestMigrationUpgradesExistingDatabase is the regression test for the
// append-only migration rule: a database created before the `answer` column
// existed must gain it on open, without losing a row.
func TestMigrationUpgradesExistingDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	ctx := context.Background()

	// Build a database at schema version 1, exactly as an older build left it.
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("opening raw database: %v", err)
	}
	if _, err := raw.Exec(migrations[0]); err != nil {
		t.Fatalf("applying migration 1: %v", err)
	}
	if _, err := raw.Exec(`PRAGMA user_version = 1`); err != nil {
		t.Fatalf("setting user_version: %v", err)
	}
	if _, err := raw.Exec(
		`INSERT INTO prep_items (subject, prompt, ref, ease, interval_days, reps,
		 lapses, due_at, created_at) VALUES (?,?,?,?,?,?,?,?,?)`,
		"cpp", "existing card", "", 2.5, 6, 2, 0,
		fmtTime(testNow), fmtTime(testNow)); err != nil {
		t.Fatalf("inserting pre-migration row: %v", err)
	}
	raw.Close()

	// Opening with the current code must migrate it forward.
	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open on a v1 database: %v", err)
	}
	defer s.Close()

	items, err := s.ListPrepItems(ctx, PrepFilter{})
	if err != nil {
		t.Fatalf("ListPrepItems: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("got %d items after migration, want 1 — the row was lost", len(items))
	}
	if items[0].Prompt != "existing card" {
		t.Errorf("prompt = %q, want the pre-existing row preserved", items[0].Prompt)
	}
	if items[0].Answer != "" {
		t.Errorf("answer = %q, want empty for a pre-existing card", items[0].Answer)
	}
	// Scheduler state must survive untouched; a migration that reset it would
	// silently destroy months of review history.
	if items[0].Reps != 2 || items[0].IntervalDays != 6 {
		t.Errorf("scheduler state = reps %d interval %d, want 2 and 6",
			items[0].Reps, items[0].IntervalDays)
	}
}

func TestPrepAnswerRoundTrips(t *testing.T) {
	s, ctx := newTestStore(t)
	const answer = "Two threads write to distinct variables that share a cache line,\nso each write invalidates the other's copy."

	id, err := s.CreatePrepItem(ctx, model.PrepItem{
		Subject: model.SubjectCPP, Prompt: "false sharing?", Answer: answer,
		Ease: 2.5, DueAt: testNow,
	}, testNow)
	if err != nil {
		t.Fatalf("CreatePrepItem: %v", err)
	}

	got, err := s.GetPrepItem(ctx, id)
	if err != nil {
		t.Fatalf("GetPrepItem: %v", err)
	}
	if got.Answer != answer {
		t.Errorf("answer did not round-trip:\n got %q\nwant %q", got.Answer, answer)
	}

	// And it must be editable, including back to empty.
	got.Answer = ""
	if err := s.UpdatePrepItem(ctx, got); err != nil {
		t.Fatalf("UpdatePrepItem: %v", err)
	}
	cleared, err := s.GetPrepItem(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if cleared.Answer != "" {
		t.Errorf("answer = %q, want cleared", cleared.Answer)
	}
}
