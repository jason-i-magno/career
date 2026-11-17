package web

import (
	"bytes"
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"net/http"
	"os"
	"time"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"

	"github.com/jason-i-magno/career/internal/model"
)

// maxDocBytes caps what one view reads. A doc is a page of notes, not a book;
// the cap only stops a mistyped path to something huge from tying up the
// server.
const maxDocBytes = 1 << 20

// markdown renders GitHub-flavoured Markdown: tables, task lists and
// autolinks, which is what notes kept in a repository tend to use.
//
// Raw HTML in the file is dropped, and links with dangerous schemes such as
// javascript: are neutralised. Both are goldmark's defaults. Never add
// html.WithUnsafe: a doc is a file on disk, not something this server wrote,
// and it is rendered into a page that carries the site's origin.
var markdown = goldmark.New(goldmark.WithExtensions(extension.GFM))

type docsPage struct {
	base
	Docs []model.Doc
}

func (s *Server) docList(w http.ResponseWriter, r *http.Request) {
	docs, err := s.store.ListDocs(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, http.StatusOK, "docs", docsPage{
		base: base{Title: "Docs", Tab: "docs"},
		Docs: docs,
	})
}

type docPage struct {
	base
	Doc      model.Doc
	Body     template.HTML // rendered Markdown; empty when Problem is set
	Modified time.Time
	Now      time.Time
	Problem  string // why the file could not be shown
}

// docShow reads the doc's file now, on this request, and renders it. Reading
// on every view is the point: the file is edited elsewhere, and the phone
// should show what is on disk, not a copy from when it was registered.
func (s *Server) docShow(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	d, err := s.store.GetDoc(r.Context(), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	p := docPage{base: base{Title: d.Name, Tab: "docs"}, Doc: d, Now: s.now()}

	body, modified, err := readDoc(d.Path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		p.Problem = "The file is not there any more. If it moved, register it again with the same name."
		s.render(w, http.StatusNotFound, "doc", p)
		return
	case errors.Is(err, errDocTooLarge):
		p.Problem = fmt.Sprintf("The file is larger than %d KiB, so it is not shown here.", maxDocBytes>>10)
		s.render(w, http.StatusOK, "doc", p)
		return
	case err != nil:
		s.fail(w, r, err)
		return
	}

	var out bytes.Buffer
	if err := markdown.Convert(body, &out); err != nil {
		s.fail(w, r, fmt.Errorf("rendering doc %d: %w", d.ID, err))
		return
	}
	// goldmark has escaped the text and dropped raw HTML, so its output is
	// safe to mark as HTML here.
	p.Body = template.HTML(out.String())
	p.Modified = modified
	s.render(w, http.StatusOK, "doc", p)
}

var errDocTooLarge = errors.New("doc too large")

func readDoc(path string) ([]byte, time.Time, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("opening doc: %w", err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("checking doc: %w", err)
	}
	body, err := io.ReadAll(io.LimitReader(f, maxDocBytes+1))
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("reading doc: %w", err)
	}
	if len(body) > maxDocBytes {
		return nil, time.Time{}, errDocTooLarge
	}
	return body, info.ModTime(), nil
}
