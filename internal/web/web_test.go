package web

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"workline/internal/domain"
	"workline/internal/store"
)

// testToday matches the seeded baseline's as_of date.
const testToday = "2026-11-18"

var ctx = context.Background()

func date(t *testing.T, s string) domain.Date {
	t.Helper()
	d, err := domain.ParseDate(s)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// newApp returns the handler over a freshly seeded temp database and its store.
func newApp(t *testing.T) (http.Handler, *store.Store) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "web.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if err := st.Seed(ctx); err != nil {
		t.Fatal(err)
	}
	h, err := New(st, domain.FixedClock{D: date(t, testToday), Hour: 14, Minute: 5})
	if err != nil {
		t.Fatal(err)
	}
	return h, st
}

func get(h http.Handler, target string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
	return rec
}

func post(h http.Handler, target string, form url.Values) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, target, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func wantStatus(t *testing.T, rec *httptest.ResponseRecorder, status int) {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("status = %d, want %d; body:\n%s", rec.Code, status, rec.Body)
	}
}

func wantBody(t *testing.T, rec *httptest.ResponseRecorder, parts ...string) {
	t.Helper()
	for _, p := range parts {
		if !strings.Contains(rec.Body.String(), p) {
			t.Errorf("body lacks %q", p)
		}
	}
}

func TestDashboardListsSeededData(t *testing.T) {
	h, _ := newApp(t)
	rec := get(h, "/?status=active&status=blocked")
	wantStatus(t, rec, http.StatusOK)
	wantBody(t, rec, "Order platform modernisation", "Kafka upgrade readiness", "3 open / 8")
	// links keep the rest of the query (repeated status included) and set sel
	wantBody(t, rec, "sel=1&amp;status=active&amp;status=blocked")
}

func TestStaticAndUnknownPaths(t *testing.T) {
	h, _ := newApp(t)
	rec := get(h, "/static/app.css")
	wantStatus(t, rec, http.StatusOK)
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/css") {
		t.Errorf("content type %q", ct)
	}
	wantStatus(t, get(h, "/nope"), http.StatusNotFound)
	wantStatus(t, post(h, "/", nil), http.StatusMethodNotAllowed)
}

func TestNextURL(t *testing.T) {
	for next, want := range map[string]string{
		"/":                         "/",
		"/?status=a&status=b&sel=3": "/?status=a&status=b&sel=3",
		"/projects":                 "/projects",
		"":                          "/",
		"//evil.com":                "/",
		"//evil.com/x":              "/",
		"https://evil.com/":         "/",
		"evil.com":                  "/",
		`/\evil.com`:                "/",
		"javascript:alert(1)":       "/",
		"/%zz":                      "/",
	} {
		r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(url.Values{"next": {next}}.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if got := nextURL(r); got != want {
			t.Errorf("nextURL(%q) = %q, want %q", next, got, want)
		}
	}
}

func TestWithParam(t *testing.T) {
	for _, c := range []struct{ in, key, val, want string }{
		{"/", "sel", "3", "/?sel=3"},
		{"/?status=a&status=b", "sel", "3", "/?sel=3&status=a&status=b"},
		{"/?sel=1&new=1", "sel", "", "/?new=1"},
		{"/?sel=1", "sel", "", "/"},
	} {
		if got := withParam(c.in, c.key, c.val); got != c.want {
			t.Errorf("withParam(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestStatusForShowsOnlyTheUserMessage(t *testing.T) {
	for _, c := range []struct {
		err  error
		want string
	}{
		{fmt.Errorf("add journal to activity 1: %w", domain.Invalid("journal text is required")), "Journal text is required."},
		{fmt.Errorf("a: %w", fmt.Errorf("b: %w", domain.Invalid("due date 2026-01-01 is before start date 2026-02-01."))), "Due date 2026-01-01 is before start date 2026-02-01."},
		{fmt.Errorf("create project %q: %w: UNIQUE constraint failed: projects.name", "x", store.ErrConflict), "That name is already in use."},
		{fmt.Errorf("acknowledge scope: activity 3: %w", store.ErrNotFlagged), "This activity is not flagged for scope creep."},
		{fmt.Errorf("journal entry 4: %w", store.ErrReadOnly), "Acknowledgement entries cannot be edited or deleted."},
		{fmt.Errorf("%w: FOREIGN KEY constraint failed", domain.ErrInvalid), "That value is not allowed."},
		{fmt.Errorf("disk on fire: %w", errors.New("boom")), "Something went wrong."},
	} {
		if _, got := statusFor(c.err); got != c.want {
			t.Errorf("statusFor(%q) message = %q, want %q", c.err, got, c.want)
		}
	}
}

func TestStatusFor(t *testing.T) {
	for err, want := range map[error]int{
		store.ErrNotFound:   404,
		domain.ErrInvalid:   422,
		store.ErrConflict:   422,
		store.ErrNotFlagged: 422,
		store.ErrReadOnly:   422,
		context.Canceled:    500,
	} {
		if got, _ := statusFor(err); got != want {
			t.Errorf("statusFor(%v) = %d, want %d", err, got, want)
		}
	}
}
