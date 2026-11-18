package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func hxGet(h http.Handler, target string, headers ...string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, target, nil)
	req.Header.Set("HX-Request", "true")
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestDashboardFragmentForHX(t *testing.T) {
	h, _ := newApp(t)
	rec := hxGet(h, "/?period=q4&sel=2", "HX-Boosted", "true")
	wantStatus(t, rec, http.StatusOK)
	body := rec.Body.String()
	for _, bad := range []string{"<html", "<head>", "<title", `class="top"`} {
		if strings.Contains(body, bad) {
			t.Errorf("fragment contains page chrome %q", bad)
		}
	}
	wantBody(t, rec, `<div id="dash" hx-boost="true" hx-target="#dash" hx-swap="outerHTML">`,
		"Q4 2026 · OCT–DEC", `id="panel"`, "Kafka upgrade readiness")
	if !strings.HasPrefix(strings.TrimSpace(body), `<div id="dash"`) {
		t.Errorf("fragment must start with the swap region: %.60s", body)
	}
	if rec.Header().Get("Vary") != "HX-Request" {
		t.Errorf("Vary = %q", rec.Header().Get("Vary"))
	}
}

func TestDashboardFullPageWithoutHXAndOnHistoryRestore(t *testing.T) {
	h, _ := newApp(t)
	for name, rec := range map[string]*httptest.ResponseRecorder{
		"plain GET":       get(h, "/?sel=2"),
		"history restore": hxGet(h, "/?sel=2", "HX-History-Restore-Request", "true"),
	} {
		wantStatus(t, rec, http.StatusOK)
		if !strings.Contains(rec.Body.String(), "<!doctype html>") || !strings.Contains(rec.Body.String(), `id="dash"`) {
			t.Errorf("%s: not a full page", name)
		}
		if rec.Header().Get("Vary") != "HX-Request" {
			t.Errorf("%s: Vary = %q", name, rec.Header().Get("Vary"))
		}
	}
}

// Every filter, selection and form on the dashboard is an ordinary link or form
// that hx-boost upgrades, so it still works without JS.
func TestDashboardControlsStayPlainLinksAndForms(t *testing.T) {
	h, _ := newApp(t)
	body := get(h, "/?sel=2").Body.String()
	for _, want := range []string{`<form method="get" action="/" class="toggle">`, `<a class="btn" href="/?new=1&amp;sel=2">`, `<a class="tl-label act-label" href="`, `<form method="post" action="/activities/2"`} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q", want)
		}
	}
	if strings.Contains(body, "hx-get") || strings.Contains(body, "hx-post") {
		t.Error("controls must work through hx-boost alone")
	}
}
