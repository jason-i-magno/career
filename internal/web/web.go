// Package web serves the dashboard, the prep review session and read-only views
// of the pipeline and story bank over HTTP, so the database can be used from a
// phone.
//
// It is meant to listen on loopback behind a proxy that adds TLS and access
// control, such as `tailscale serve`. There is no login: anyone who can reach
// the server can read everything in the database.
package web

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jason-i-magno/career/internal/store"
)

//go:embed templates static
var assets embed.FS

// Server holds what the handlers share. Construct it with New.
type Server struct {
	store *store.Store
	now   func() time.Time
	log   *slog.Logger
	pages map[string]*template.Template
}

// New returns the handler for the whole site.
func New(st *store.Store, now func() time.Time, log *slog.Logger) (http.Handler, error) {
	pages, err := parsePages()
	if err != nil {
		return nil, err
	}
	s := &Server{store: st, now: now, log: log, pages: pages}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.today)
	mux.HandleFunc("GET /prep", s.prepPage)
	mux.HandleFunc("POST /prep/{id}/grade", s.prepGrade)
	mux.HandleFunc("GET /apps", s.appList)
	mux.HandleFunc("GET /apps/{id}", s.appShow)
	mux.HandleFunc("GET /stories", s.storyList)
	mux.HandleFunc("GET /stories/{id}", s.storyShow)
	mux.HandleFunc("GET /healthz", s.healthz)
	mux.HandleFunc("GET /static/style.css", s.asset("static/style.css", "text/css; charset=utf-8"))
	mux.HandleFunc("GET /manifest.webmanifest", s.asset("static/manifest.webmanifest", "application/manifest+json"))
	mux.HandleFunc("GET /static/icon.svg", s.asset("static/icon.svg", "image/svg+xml"))
	mux.HandleFunc("GET /static/icon-192.png", s.asset("static/icon-192.png", "image/png"))
	mux.HandleFunc("GET /static/icon-512.png", s.asset("static/icon-512.png", "image/png"))
	// The conventional path, which Safari also tries without being told.
	mux.HandleFunc("GET /apple-touch-icon.png", s.asset("static/icon-180.png", "image/png"))
	mux.HandleFunc("/", s.notFound)

	// Grading is the only write. CrossOriginProtection refuses a form post
	// that some other site's page makes on the phone's behalf.
	var h http.Handler = http.NewCrossOriginProtection().Handler(mux)
	h = checkHost(h)
	h = securityHeaders(h)
	return s.logRequests(h), nil
}

var pageNames = []string{"today", "prep", "apps", "app", "stories", "story", "error"}

// parsePages pairs the layout with each page, so that every page can define
// its own "content" block.
func parsePages() (map[string]*template.Template, error) {
	pages := make(map[string]*template.Template, len(pageNames))
	for _, name := range pageNames {
		t, err := template.New(name).Funcs(funcs).
			ParseFS(assets, "templates/layout.html", "templates/"+name+".html")
		if err != nil {
			return nil, fmt.Errorf("parsing %s template: %w", name, err)
		}
		pages[name] = t
	}
	return pages, nil
}

// render executes a page into a buffer first, so that a template error becomes
// a clean 500 rather than half a page.
func (s *Server) render(w http.ResponseWriter, status int, name string, data any) {
	var buf bytes.Buffer
	if err := s.pages[name].ExecuteTemplate(&buf, "layout", data); err != nil {
		s.log.Error("rendering page", "page", name, "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	buf.WriteTo(w)
}

// errorPage is the data for every error response.
type errorPage struct {
	base
	Message string
}

// fail reports err to the client. A missing record gets a 404; anything else
// is logged and reported without detail.
func (s *Server) fail(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, store.ErrNotFound) {
		s.notFound(w, r)
		return
	}
	s.log.Error("handling request", "method", r.Method, "path", r.URL.Path, "err", err)
	s.render(w, http.StatusInternalServerError, "error", errorPage{
		base:    base{Title: "Something went wrong"},
		Message: "The details are in the server log.",
	})
}

func (s *Server) notFound(w http.ResponseWriter, r *http.Request) {
	s.render(w, http.StatusNotFound, "error", errorPage{
		base:    base{Title: "Not found"},
		Message: "There is nothing here.",
	})
}

func (s *Server) badRequest(w http.ResponseWriter, err error) {
	s.render(w, http.StatusBadRequest, "error", errorPage{
		base:    base{Title: "Bad request"},
		Message: err.Error(),
	})
}

// pathID reads the {id} path segment. A malformed ID cannot match a record, so
// it is reported the same way as a missing one.
func pathID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("bad id %q: %w", r.PathValue("id"), store.ErrNotFound)
	}
	return id, nil
}

func (s *Server) asset(name, contentType string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := assets.ReadFile(name)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		w.Header().Set("Content-Type", contentType)
		w.Write(body)
	}
}

func (s *Server) healthz(w http.ResponseWriter, r *http.Request) {
	if err := s.store.Ping(r.Context()); err != nil {
		s.log.Error("health check", "err", err)
		http.Error(w, "database unavailable", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	io.WriteString(w, "ok\n")
}

// --- middleware --------------------------------------------------------------

// checkHost refuses requests addressed to any name but this machine's.
//
// A server on loopback is still reachable from pages the workstation's own
// browser opens, through DNS rebinding: a hostile domain re-resolves to
// 127.0.0.1, and its scripts then read the responses as same-origin. Those
// requests carry the hostile name in Host. Loopback names and IP literals
// cannot be rebound, and ts.net names are controlled by Tailscale, so
// accepting only those closes the hole without any configuration.
func checkHost(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !allowedHost(r.Host) {
			http.Error(w, "unrecognised host", http.StatusMisdirectedRequest)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func allowedHost(hostport string) bool {
	host := hostport
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		host = h
	}
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	switch {
	case host == "localhost":
		return true
	case net.ParseIP(strings.Trim(host, "[]")) != nil:
		return true
	default:
		return strings.HasSuffix(host, ".ts.net")
	}
}

// securityHeaders applies to every response. The pages run no script and load
// nothing from elsewhere, so the policy can be strict. no-store keeps personal
// data out of caches, and stops the back button showing a card already graded.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", "default-src 'none'; style-src 'self'; img-src 'self'; "+
			"manifest-src 'self'; form-action 'self'; frame-ancestors 'none'; base-uri 'none'")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		s.log.Info("request", "method", r.Method, "path", r.URL.Path,
			"status", sw.status, "dur", time.Since(start).Round(time.Microsecond))
	})
}

// statusWriter records the status code a handler sends, for the request log.
type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}
