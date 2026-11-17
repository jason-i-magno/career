package web

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jason-i-magno/career/internal/model"
	"github.com/jason-i-magno/career/internal/store"
)

// addDoc writes body to a file and registers it, returning its ID and path.
func addDoc(t *testing.T, st *store.Store, name, body string) (int64, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "doc.md")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("writing doc: %v", err)
	}
	id, err := st.SaveDoc(context.Background(), model.Doc{Name: name, Path: path}, testNow)
	if err != nil {
		t.Fatalf("SaveDoc: %v", err)
	}
	return id, path
}

func TestDocRendersMarkdownWithTaskLists(t *testing.T) {
	st, h := newTestServer(t)
	id, _ := addDoc(t, st, "Reading list", "## This week\n\n- [ ] Read the Raft paper\n- [x] Finish chapter 3\n\n| Topic | Done |\n| --- | --- |\n| Paxos | no |\n")

	rec := do(t, h, "GET", fmt.Sprintf("/docs/%d", id), nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	assertContains(t, body, "<h1>Reading list</h1>", "<h2>This week</h2>",
		`type="checkbox"`, "Read the Raft paper", "<table>", "Paxos")
	if n := strings.Count(body, "checked"); n != 1 {
		t.Errorf("got %d checked boxes, want 1", n)
	}
}

// The file is not something this server wrote, so markup in it must not reach
// the page as markup.
func TestDocDropsRawHTMLAndScriptLinks(t *testing.T) {
	st, h := newTestServer(t)
	id, _ := addDoc(t, st, "Notes", "Hello\n\n<script>alert(1)</script>\n\n<img src=x onerror=alert(1)>\n\n[click](javascript:alert(1))\n")

	body := do(t, h, "GET", fmt.Sprintf("/docs/%d", id), nil).Body.String()
	assertContains(t, body, "Hello", "click")
	assertOmits(t, body, "<script>alert", "onerror", `href="javascript:`)
}

// Reading on every view is the point of a doc: an edit to the file shows up
// on the next page load, with nothing re-registered.
func TestDocShowsTheFileAsItIsNow(t *testing.T) {
	st, h := newTestServer(t)
	id, path := addDoc(t, st, "Notes", "first draft\n")
	assertContains(t, do(t, h, "GET", fmt.Sprintf("/docs/%d", id), nil).Body.String(), "first draft")

	if err := os.WriteFile(path, []byte("second draft\n"), 0o600); err != nil {
		t.Fatalf("rewriting doc: %v", err)
	}
	body := do(t, h, "GET", fmt.Sprintf("/docs/%d", id), nil).Body.String()
	assertContains(t, body, "second draft")
	assertOmits(t, body, "first draft")
}

func TestDocWhoseFileIsGoneSaysSo(t *testing.T) {
	st, h := newTestServer(t)
	id, path := addDoc(t, st, "Notes", "text\n")
	if err := os.Remove(path); err != nil {
		t.Fatalf("removing doc: %v", err)
	}

	rec := do(t, h, "GET", fmt.Sprintf("/docs/%d", id), nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
	assertContains(t, rec.Body.String(), "Notes", "not there any more", path)
}

func TestDocLargerThanTheCapIsNotShown(t *testing.T) {
	st, h := newTestServer(t)
	id, _ := addDoc(t, st, "Huge", strings.Repeat("x", maxDocBytes+1))

	rec := do(t, h, "GET", fmt.Sprintf("/docs/%d", id), nil)
	assertContains(t, rec.Body.String(), "larger than")
	if len(rec.Body.String()) > maxDocBytes {
		t.Errorf("page is %d bytes; the file's text should not be in it", len(rec.Body.String()))
	}
}

func TestDocListLinksEachDoc(t *testing.T) {
	st, h := newTestServer(t)
	assertContains(t, do(t, h, "GET", "/docs", nil).Body.String(), "No docs yet")

	id, _ := addDoc(t, st, "Reading list", "text\n")
	body := do(t, h, "GET", "/docs", nil).Body.String()
	assertContains(t, body, fmt.Sprintf(`href="/docs/%d"`, id), "Reading list")
}

func TestUnknownDocIsNotFound(t *testing.T) {
	_, h := newTestServer(t)
	for _, target := range []string{"/docs/99", "/docs/abc"} {
		if rec := do(t, h, "GET", target, nil); rec.Code != http.StatusNotFound {
			t.Errorf("GET %s: status = %d, want 404", target, rec.Code)
		}
	}
}
