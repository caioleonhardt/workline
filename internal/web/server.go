// Package web serves Workline's HTML pages.
package web

import (
	"bytes"
	"embed"
	"errors"
	"html/template"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"workline/internal/domain"
	"workline/internal/store"
)

//go:embed templates/*.html
var templateFS embed.FS

//go:embed static
var staticFS embed.FS

// Server holds the dependencies shared by all handlers.
type Server struct {
	store *store.Store
	clock domain.Clock
	tmpl  *template.Template
}

// New returns the application's handler.
func New(st *store.Store, clock domain.Clock) (http.Handler, error) {
	tmpl, err := template.ParseFS(templateFS, "templates/*.html")
	if err != nil {
		return nil, err
	}
	s := &Server{store: st, clock: clock, tmpl: tmpl}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.dashboard)
	mux.HandleFunc("POST /activities", s.createActivity)
	mux.HandleFunc("POST /activities/{id}", s.updateActivity)
	mux.HandleFunc("POST /activities/{id}/delete", s.deleteActivity)
	mux.HandleFunc("GET /projects", s.projects)
	mux.HandleFunc("POST /projects/{id}", s.updateProject)
	mux.HandleFunc("POST /projects/{id}/delete", s.deleteProject)
	mux.Handle("GET /static/", http.FileServerFS(staticFS))
	return mux, nil
}

// render executes the named template into a buffer first, so a template error
// cannot leave a half-written page.
func (s *Server) render(w http.ResponseWriter, status int, name string, data any) {
	var buf bytes.Buffer
	if err := s.tmpl.ExecuteTemplate(&buf, name, data); err != nil {
		slog.Error("render", "template", name, "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	buf.WriteTo(w)
}

type errorView struct {
	Title, Message, Back string
}

// statusFor maps a store or domain error to an HTTP status and a message that
// is safe to show.
func statusFor(err error) (int, string) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		return http.StatusNotFound, "Not found."
	case errors.Is(err, domain.ErrInvalid), errors.Is(err, store.ErrConflict),
		errors.Is(err, store.ErrNotFlagged), errors.Is(err, store.ErrReadOnly):
		return http.StatusUnprocessableEntity, err.Error()
	}
	return http.StatusInternalServerError, "Something went wrong."
}

// fail shows a plain error page with a Back link (SPEC §10 item 27).
func (s *Server) fail(w http.ResponseWriter, r *http.Request, err error) {
	status, msg := statusFor(err)
	if status == http.StatusInternalServerError {
		slog.Error("request failed", "method", r.Method, "path", r.URL.Path, "err", err)
	}
	s.render(w, status, "error.html", errorView{Title: "Error", Message: msg, Back: nextURL(r)})
}

// nextURL returns the form's `next` field if it is a same-site relative path,
// else "/" (SPEC §10 item 29).
func nextURL(r *http.Request) string {
	next := r.PostFormValue("next")
	u, err := url.Parse(next)
	if err != nil || u.Scheme != "" || u.Host != "" ||
		!strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") || strings.Contains(next, `\`) {
		return "/"
	}
	return next
}

// redirect answers a successful mutation with 303 See Other.
func redirect(w http.ResponseWriter, r *http.Request, to string) {
	http.Redirect(w, r, to, http.StatusSeeOther)
}

// withParam returns rawURL with query parameter key set to value, or removed
// when value is empty. Other parameters, including repeated ones, are kept.
func withParam(rawURL, key, value string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "/"
	}
	q := u.Query()
	if value == "" {
		q.Del(key)
	} else {
		q.Set(key, value)
	}
	u.RawQuery = q.Encode()
	return u.String()
}
