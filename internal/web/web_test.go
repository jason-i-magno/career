package web

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jason-i-magno/career/internal/model"
	"github.com/jason-i-magno/career/internal/prep"
	"github.com/jason-i-magno/career/internal/store"
)

// testZone is six hours behind UTC, so local and UTC calendar days disagree
// every evening.
var testZone = time.FixedZone("UTC-6", -6*60*60)

var testNow = time.Date(2026, time.September, 15, 10, 0, 0, 0, testZone)

func newTestServer(t *testing.T) (*store.Store, http.Handler) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	h, err := New(st, func() time.Time { return testNow }, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return st, h
}

// do sends a request the way the phone does through `tailscale serve`: to a
// ts.net name, from a page on the same origin.
func do(t *testing.T, h http.Handler, method, target string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req := httptest.NewRequest(method, target, body)
	req.Host = "workstation.example.ts.net"
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func addApplication(t *testing.T, st *store.Store, a model.Application, at time.Time) int64 {
	t.Helper()
	if a.Role == "" {
		a.Role = "Engineer"
	}
	a.Track, a.Source = model.TrackBackend, model.SourceBoard
	id, err := st.CreateApplication(context.Background(), a, at)
	if err != nil {
		t.Fatalf("CreateApplication: %v", err)
	}
	return id
}

func addPrepItem(t *testing.T, st *store.Store, prompt string, dueAt time.Time) int64 {
	t.Helper()
	ease, interval, reps, _ := prep.Init(testNow)
	id, err := st.CreatePrepItem(context.Background(), model.PrepItem{
		Subject: model.SubjectGo, Prompt: prompt,
		Ease: ease, IntervalDays: interval, Reps: reps, DueAt: dueAt,
	}, testNow)
	if err != nil {
		t.Fatalf("CreatePrepItem: %v", err)
	}
	return id
}

func assertContains(t *testing.T, body string, wants ...string) {
	t.Helper()
	for _, w := range wants {
		if !strings.Contains(body, w) {
			t.Errorf("body is missing %q", w)
		}
	}
}

func assertOmits(t *testing.T, body string, unwanted ...string) {
	t.Helper()
	for _, u := range unwanted {
		if strings.Contains(body, u) {
			t.Errorf("body contains %q, want it absent", u)
		}
	}
}

func TestTodayListsDueAndQuietApplications(t *testing.T) {
	st, h := newTestServer(t)
	due := testNow.Add(-time.Hour)
	addApplication(t, st, model.Application{Company: "Acme", Stage: model.StageScreen,
		NextAction: "send thank-you", NextActionAt: &due}, testNow)
	addApplication(t, st, model.Application{Company: "Globex", Stage: model.StageApplied}, testNow.AddDate(0, 0, -20))
	addApplication(t, st, model.Application{Company: "Initech", Stage: model.StageLead}, testNow.AddDate(0, 0, -90))

	rec := do(t, h, "GET", "/", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	assertContains(t, body, "Due now", "Acme", "send thank-you · today",
		"Gone quiet", "Globex", "last touched 20d ago", "2 live opportunities, 1 lead not yet applied to")
	// A lead is neither due nor stale, so today has nothing to say about it.
	assertOmits(t, body, "Initech")
}

func TestPrepReviewShowsTheNextDueCardAndGradesIt(t *testing.T) {
	st, h := newTestServer(t)
	id := addPrepItem(t, st, "Describe the GMP scheduler.", testNow.Add(-time.Hour))
	addPrepItem(t, st, "A card for later.", testNow.AddDate(0, 0, 3))

	body := do(t, h, "GET", "/prep", nil).Body.String()
	assertContains(t, body, "Describe the GMP scheduler.", "1 due", fmt.Sprintf(`action="/prep/%d/grade"`, id))
	assertOmits(t, body, "A card for later.")

	rec := do(t, h, "POST", fmt.Sprintf("/prep/%d/grade", id), url.Values{"grade": {"good"}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("grade status = %d, want 303", rec.Code)
	}
	got, err := st.GetPrepItem(context.Background(), id)
	if err != nil {
		t.Fatalf("GetPrepItem: %v", err)
	}
	if got.Reps != 1 || !got.DueAt.After(testNow) {
		t.Errorf("after grading good: reps = %d, due %v; want 1 rep and due after %v", got.Reps, got.DueAt, testNow)
	}

	next := rec.Header().Get("Location")
	if want := fmt.Sprintf("/prep?graded=%d", id); next != want {
		t.Fatalf("redirect = %q, want %q", next, want)
	}
	assertContains(t, do(t, h, "GET", next, nil).Body.String(), "next in 1d", "Nothing due")
}

func TestGradingKeepsTheSubjectFilter(t *testing.T) {
	st, h := newTestServer(t)
	id := addPrepItem(t, st, "p", testNow.Add(-time.Hour))

	rec := do(t, h, "POST", fmt.Sprintf("/prep/%d/grade", id), url.Values{"grade": {"hard"}, "subject": {"go"}})
	if got, want := rec.Header().Get("Location"), fmt.Sprintf("/prep?graded=%d&subject=go", id); got != want {
		t.Errorf("redirect = %q, want %q", got, want)
	}
}

// A double tap, or the back button and a second tap, resubmits the grade form.
// The card must be reviewed once, not pushed out twice.
func TestResubmittedGradeChangesNothing(t *testing.T) {
	st, h := newTestServer(t)
	ctx := context.Background()
	id := addPrepItem(t, st, "p", testNow.Add(-time.Hour))
	path := fmt.Sprintf("/prep/%d/grade", id)

	do(t, h, "POST", path, url.Values{"grade": {"good"}})
	first, err := st.GetPrepItem(ctx, id)
	if err != nil {
		t.Fatalf("GetPrepItem: %v", err)
	}
	if rec := do(t, h, "POST", path, url.Values{"grade": {"easy"}}); rec.Code != http.StatusSeeOther {
		t.Fatalf("resubmit status = %d, want 303", rec.Code)
	}
	second, err := st.GetPrepItem(ctx, id)
	if err != nil {
		t.Fatalf("GetPrepItem: %v", err)
	}
	if second.Reps != first.Reps || second.IntervalDays != first.IntervalDays ||
		!second.DueAt.Equal(first.DueAt) || second.Ease != first.Ease {
		t.Errorf("resubmission rescheduled the card:\n first %+v\nsecond %+v", first, second)
	}
}

func TestCrossSiteGradeIsRefused(t *testing.T) {
	st, h := newTestServer(t)
	id := addPrepItem(t, st, "p", testNow.Add(-time.Hour))

	req := httptest.NewRequest("POST", fmt.Sprintf("/prep/%d/grade", id), strings.NewReader("grade=easy"))
	req.Host = "workstation.example.ts.net"
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rec.Code)
	}
	got, err := st.GetPrepItem(context.Background(), id)
	if err != nil {
		t.Fatalf("GetPrepItem: %v", err)
	}
	if got.Reps != 0 || got.ReviewedAt != nil {
		t.Errorf("a refused grade still changed the card: %+v", got)
	}
}

func TestOnlyThisMachinesNamesAreServed(t *testing.T) {
	_, h := newTestServer(t)
	tests := []struct {
		host string
		want int
	}{
		{"127.0.0.1:8080", http.StatusOK},
		{"localhost:8080", http.StatusOK},
		{"[::1]:8080", http.StatusOK},
		{"100.101.102.103", http.StatusOK},
		{"workstation.example.ts.net", http.StatusOK},
		{"WORKSTATION.example.ts.net.", http.StatusOK},
		{"rebind.example", http.StatusMisdirectedRequest},
		{"rebind.example:8080", http.StatusMisdirectedRequest},
		{"ts.net.rebind.example", http.StatusMisdirectedRequest},
		{"", http.StatusMisdirectedRequest},
	}
	for _, tc := range tests {
		t.Run(tc.host, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/healthz", nil)
			req.Host = tc.host
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Errorf("status = %d, want %d", rec.Code, tc.want)
			}
		})
	}
}

func TestUserTextIsEscaped(t *testing.T) {
	st, h := newTestServer(t)
	id := addApplication(t, st, model.Application{
		Company: "<script>alert(1)</script>", Stage: model.StageApplied,
		URL: "javascript:alert(1)",
	}, testNow)

	body := do(t, h, "GET", fmt.Sprintf("/apps/%d", id), nil).Body.String()
	assertContains(t, body, "&lt;script&gt;alert(1)&lt;/script&gt;")
	assertOmits(t, body, "<script>", `href="javascript:`)
}

func TestUnsanitizedStoryTextIsWithheld(t *testing.T) {
	st, h := newTestServer(t)
	ctx := context.Background()
	draftID, err := st.CreateStory(ctx, model.Story{Title: "Draft story",
		Situation: "draft situation", Task: "t", Action: "a", Result: "r"}, testNow)
	if err != nil {
		t.Fatalf("CreateStory: %v", err)
	}
	readyID, err := st.CreateStory(ctx, model.Story{Title: "Ready story",
		Situation: "ready situation", Task: "t", Action: "a", Result: "r", Sanitized: true}, testNow)
	if err != nil {
		t.Fatalf("CreateStory: %v", err)
	}

	draft := do(t, h, "GET", fmt.Sprintf("/stories/%d", draftID), nil).Body.String()
	assertContains(t, draft, "Draft story", fmt.Sprintf("career story show %d", draftID))
	assertOmits(t, draft, "draft situation")

	assertContains(t, do(t, h, "GET", fmt.Sprintf("/stories/%d", readyID), nil).Body.String(), "ready situation")
	assertContains(t, do(t, h, "GET", "/stories", nil).Body.String(),
		"Draft story", "unsanitized", "Ready story", "badge-ok")
}

func TestApplicationDetailShowsHistory(t *testing.T) {
	st, h := newTestServer(t)
	id := addApplication(t, st, model.Application{Company: "Acme", Stage: model.StageTech,
		Location: "Springfield", Remote: true, BaseComp: 120000}, testNow)
	if _, err := st.AddEvent(context.Background(), model.Event{ApplicationID: id, At: testNow,
		Kind: model.EventNote, Body: "Recruiter said the team is hiring two."}); err != nil {
		t.Fatalf("AddEvent: %v", err)
	}

	assertContains(t, do(t, h, "GET", fmt.Sprintf("/apps/%d", id), nil).Body.String(),
		"Springfield (remote)", "$120,000", "Recruiter said the team is hiring two.")
}

func TestBadAndMissingRequests(t *testing.T) {
	_, h := newTestServer(t)
	tests := []struct {
		method, path string
		form         url.Values
		want         int
	}{
		{"GET", "/apps/999", nil, http.StatusNotFound},
		{"GET", "/apps/abc", nil, http.StatusNotFound},
		{"GET", "/stories/999", nil, http.StatusNotFound},
		{"GET", "/nowhere", nil, http.StatusNotFound},
		{"POST", "/prep/999/grade", url.Values{"grade": {"good"}}, http.StatusNotFound},
		{"POST", "/prep/1/grade", url.Values{"grade": {"brilliant"}}, http.StatusBadRequest},
		{"GET", "/prep?subject=astrology", nil, http.StatusBadRequest},
	}
	for _, tc := range tests {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			if got := do(t, h, tc.method, tc.path, tc.form).Code; got != tc.want {
				t.Errorf("status = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestStaticFilesAndHeaders(t *testing.T) {
	_, h := newTestServer(t)
	for path, contentType := range map[string]string{
		"/manifest.webmanifest": "application/manifest+json",
		"/static/style.css":     "text/css; charset=utf-8",
		"/static/icon.svg":      "image/svg+xml",
		"/static/icon-192.png":  "image/png",
		"/static/icon-512.png":  "image/png",
		"/apple-touch-icon.png": "image/png",
		"/":                     "text/html; charset=utf-8",
	} {
		rec := do(t, h, "GET", path, nil)
		if rec.Code != http.StatusOK {
			t.Errorf("%s: status = %d, want 200", path, rec.Code)
		}
		if got := rec.Header().Get("Content-Type"); got != contentType {
			t.Errorf("%s: Content-Type = %q, want %q", path, got, contentType)
		}
		if !strings.Contains(rec.Header().Get("Content-Security-Policy"), "default-src 'none'") {
			t.Errorf("%s: missing the content security policy", path)
		}
	}
}

func TestIconsAreTheSizesThePagesPromise(t *testing.T) {
	_, h := newTestServer(t)
	decode := func(path string) image.Image {
		t.Helper()
		rec := do(t, h, "GET", path, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status = %d, want 200", path, rec.Code)
		}
		img, err := png.Decode(rec.Body)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		return img
	}

	assertContains(t, do(t, h, "GET", "/", nil).Body.String(),
		`<link rel="apple-touch-icon" href="/apple-touch-icon.png">`)
	touch := decode("/apple-touch-icon.png")
	if b := touch.Bounds(); b.Dx() != 180 || b.Dy() != 180 {
		t.Errorf("apple-touch-icon is %v, want 180x180", b.Size())
	}
	// iOS fills transparent pixels with black.
	if o, ok := touch.(interface{ Opaque() bool }); !ok || !o.Opaque() {
		t.Error("apple-touch-icon must be opaque")
	}

	var manifest struct {
		Icons []struct{ Src, Sizes string }
	}
	if err := json.NewDecoder(do(t, h, "GET", "/manifest.webmanifest", nil).Body).Decode(&manifest); err != nil {
		t.Fatalf("decoding manifest: %v", err)
	}
	if len(manifest.Icons) == 0 {
		t.Fatal("manifest lists no icons")
	}
	for _, icon := range manifest.Icons {
		b := decode(icon.Src).Bounds()
		if got := fmt.Sprintf("%dx%d", b.Dx(), b.Dy()); got != icon.Sizes {
			t.Errorf("%s is %s, manifest says %s", icon.Src, got, icon.Sizes)
		}
	}
}

func TestRelDaysCountsLocalCalendarDays(t *testing.T) {
	// 7 pm six hours behind UTC is already the next day in UTC: the case that
	// breaks a count taken in UTC.
	evening := time.Date(2026, time.October, 23, 19, 0, 0, 0, testZone)

	tests := []struct {
		name   string
		t, now time.Time
		want   string
	}{
		{"due this morning", time.Date(2026, time.October, 23, 0, 0, 0, 0, testZone), evening, "today"},
		{"stored in UTC, same local day", time.Date(2026, time.October, 23, 6, 0, 0, 0, time.UTC), evening, "today"},
		{"tomorrow", time.Date(2026, time.October, 24, 0, 0, 0, 0, testZone), evening, "tomorrow"},
		{"yesterday", time.Date(2026, time.October, 22, 12, 0, 0, 0, testZone), evening, "yesterday"},
		{"a week out", time.Date(2026, time.October, 30, 0, 0, 0, 0, testZone), evening, "in 7d"},
		{"days ago", time.Date(2026, time.October, 18, 0, 0, 0, 0, testZone), evening, "5d ago"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := relDays(tc.t, tc.now); got != tc.want {
				t.Errorf("relDays = %q, want %q", got, tc.want)
			}
		})
	}
}
