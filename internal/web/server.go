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
	mux.HandleFunc("POST /activities/{id}/tasks", s.addTask)
	mux.HandleFunc("POST /tasks/{id}/toggle", s.toggleTask)
	mux.HandleFunc("POST /tasks/{id}", s.editTask)
	mux.HandleFunc("POST /tasks/{id}/delete", s.deleteTask)
	mux.HandleFunc("POST /journal", s.addJournal)
	mux.HandleFunc("POST /journal/{id}", s.editJournal)
	mux.HandleFunc("POST /journal/{id}/delete", s.deleteJournal)
	mux.HandleFunc("POST /activities/{id}/scope-ack", s.ackScope)
	mux.HandleFunc("GET /export/journal.md", s.exportJournal)
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

// isHX reports whether the request came from htmx and wants a fragment. A
// history restore needs the whole page.
func isHX(r *http.Request) bool {
	return r.Header.Get("HX-Request") == "true" && r.Header.Get("HX-History-Restore-Request") != "true"
}

type errorView struct {
	Title, Message, Back string
}

// statusFor maps a store or domain error to an HTTP status and a message that
// is safe to show: never the wrapping context, which is for logs.
func statusFor(err error) (int, string) {
	var invalid *domain.InvalidError
	switch {
	case errors.Is(err, store.ErrNotFound):
		return http.StatusNotFound, "Not found."
	case errors.As(err, &invalid):
		return http.StatusUnprocessableEntity, sentence(invalid.Message)
	case errors.Is(err, domain.ErrInvalid):
		return http.StatusUnprocessableEntity, "That value is not allowed."
	case errors.Is(err, store.ErrConflict):
		return http.StatusUnprocessableEntity, "That name is already in use."
	case errors.Is(err, store.ErrNotFlagged):
		return http.StatusUnprocessableEntity, "This activity is not flagged for scope creep."
	case errors.Is(err, store.ErrReadOnly):
		return http.StatusUnprocessableEntity, "Acknowledgement entries cannot be edited or deleted."
	}
	return http.StatusInternalServerError, "Something went wrong."
}

// sentence capitalises a message and ends it with a full stop.
func sentence(m string) string {
	if m == "" {
		return m
	}
	m = strings.ToUpper(m[:1]) + m[1:]
	if !strings.HasSuffix(m, ".") {
		m += "."
	}
	return m
}

// fail shows a plain error page with a Back link (SPEC §10 item 27).
func (s *Server) fail(w http.ResponseWriter, r *http.Request, err error) {
	status, msg := statusFor(err)
	if status == http.StatusInternalServerError {
		slog.Error("request failed", "method", r.Method, "path", r.URL.Path, "err", err)
	}
	if isHX(r) {
		// Show the message in the error bar and leave the dashboard, the address
		// bar and the history as they are.
		w.Header().Set("HX-Push-Url", "false")
		w.Header().Set("HX-Retarget", "#errors")
		if fromQuickAdd(r) {
			w.Header().Set("HX-Retarget", "#quickadd-error")
		}
		w.Header().Set("HX-Reswap", "innerHTML")
		s.render(w, status, "error-bar", msg)
		return
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

// fromQuickAdd reports whether htmx sent the request from the quick-add bar.
func fromQuickAdd(r *http.Request) bool { return r.Header.Get("HX-Trigger") == "quickadd" }

// done answers a successful dashboard mutation. An htmx request gets the
// dashboard fragment for `to` and the address to show in the URL bar; any other
// request is redirected there with 303 See Other.
func (s *Server) done(w http.ResponseWriter, r *http.Request, to string, mods ...func(*dashboardView)) {
	if !isHX(r) {
		redirect(w, r, to)
		return
	}
	w.Header().Set("HX-Push-Url", to)
	s.renderDashboard(w, r, to, mods...)
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
