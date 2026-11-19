package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
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
	for _, want := range []string{`<form method="get" action="/" class="toggle">`, `<a class="btn new-activity" href="/?new=1&amp;sel=2"`, `<a class="tl-label act-label" href="`, `<form method="post" action="/activities/2"`} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q", want)
		}
	}
	dash := body[:strings.Index(body, `<form id="quickadd"`)] // the bar outside #dash needs explicit attributes
	if strings.Contains(dash, "hx-get") || strings.Contains(dash, "hx-post") {
		t.Error("controls must work through hx-boost alone")
	}
}

func hxPost(h http.Handler, target string, form url.Values) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, target, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// Every dashboard mutation redirects without JS and answers htmx with the
// fragment for the same address (SPEC §10 item 59).
func TestMutationsAnswerHXWithTheFragment(t *testing.T) {
	next := "/?sel=2&status=active&status=blocked"
	for _, c := range []struct {
		name, target string
		form         url.Values
		want         string // the address both paths end up at
	}{
		{"add subtask", "/activities/2/subtasks", url.Values{"text": {"x"}}, next},
		{"toggle subtask", "/subtasks/1/toggle", nil, next},
		{"edit subtask", "/subtasks/1", url.Values{"text": {"y"}, "created_on": {"2026-03-01"}}, next},
		{"delete subtask", "/subtasks/1/delete", nil, next},
		{"add journal", "/journal", url.Values{"activity": {"2"}, "kind": {"update"}, "text": {"z"}}, next},
		{"edit journal", "/journal/1", url.Values{"kind": {"update"}, "text": {"z"}, "date": {"2026-03-01"}}, next},
		{"delete journal", "/journal/1/delete", nil, next},
		{"status", "/activities/2", url.Values{"status": {"active"}}, next},
		{"edit activity", "/activities/2", url.Values{"project": {"1"}, "name": {"n"}, "start": {"2026-09-01"}, "due": {"2026-12-01"}}, next},
		{"acknowledge", "/activities/1/scope-ack", nil, next},
		{"delete activity", "/activities/2/delete", nil, "/?status=active&status=blocked"},
	} {
		form := url.Values{"next": {next}}
		for k, v := range c.form {
			form[k] = v
		}
		h, _ := newApp(t)
		rec := post(h, c.target, form)
		wantStatus(t, rec, http.StatusSeeOther)
		if loc := rec.Header().Get("Location"); loc != c.want {
			t.Errorf("%s without JS: Location %q, want %q", c.name, loc, c.want)
		}

		h, _ = newApp(t)
		rec = hxPost(h, c.target, form)
		if rec.Code != http.StatusOK || rec.Header().Get("HX-Push-Url") != c.want ||
			!strings.HasPrefix(strings.TrimSpace(rec.Body.String()), `<div id="dash"`) ||
			!strings.Contains(rec.Body.String(), `<div id="errors" aria-live="polite" hx-swap-oob="true"></div>`) {
			t.Errorf("%s with htmx: status %d, push %q", c.name, rec.Code, rec.Header().Get("HX-Push-Url"))
		}
	}
}

func TestCreateActivityAnswersHXWithTheNewSelection(t *testing.T) {
	h, _ := newApp(t)
	rec := hxPost(h, "/activities", newForm(nil))
	wantStatus(t, rec, http.StatusOK)
	push, _ := url.Parse(rec.Header().Get("HX-Push-Url"))
	if push.Query().Get("sel") == "" || push.Query().Has("new") || len(push.Query()["status"]) != 2 {
		t.Errorf("HX-Push-Url = %s", push)
	}
	wantBody(t, rec, "Pilot", `id="panel"`)
}

func TestHXToggleUpdatesPanelTimelineAndTiles(t *testing.T) {
	h, _ := newApp(t)
	before := get(h, "/?sel=1").Body.String()
	wantStatus(t, hxPost(h, "/subtasks/1/toggle", url.Values{"next": {"/?sel=1"}}), http.StatusOK)
	rec := hxGet(h, "/?sel=1")
	body := rec.Body.String()
	if !strings.Contains(before, "5 of 8 subtasks done") || !strings.Contains(body, "4 of 8 subtasks done") ||
		!strings.Contains(body, "4 open / 8") || tileValue(t, body, "Open subtasks now") != 18 {
		t.Error("toggle did not reach the panel, timeline and tiles")
	}
}

func TestHXErrorsGoToTheErrorBar(t *testing.T) {
	h, _ := newApp(t)
	for name, c := range map[string]struct {
		target string
		form   url.Values
		status int
		msg    string
	}{
		"validation": {"/activities/2/subtasks", url.Values{"text": {" "}}, http.StatusUnprocessableEntity, "Subtask text is required."},
		"not found":  {"/subtasks/999/toggle", nil, http.StatusNotFound, "Not found."},
		"This activity is not flagged for scope creep.": {"/activities/3/scope-ack", nil, http.StatusUnprocessableEntity, "This activity is not flagged for scope creep."},
	} {
		form := url.Values{"next": {"/?sel=2"}}
		for k, v := range c.form {
			form[k] = v
		}
		rec := hxPost(h, c.target, form)
		body := rec.Body.String()
		if rec.Code != c.status || rec.Header().Get("HX-Push-Url") != "false" || rec.Header().Get("HX-Retarget") != "#errors" || rec.Header().Get("HX-Reswap") != "innerHTML" ||
			!strings.Contains(body, c.msg) || !strings.Contains(body, `<div class="error-bar" role="alert">`) ||
			!strings.Contains(body, `<button type="button" class="dismiss">Dismiss</button>`) || strings.Contains(body, "hx-get") ||
			strings.Contains(body, `id="dash"`) || strings.Contains(body, "<html") || leaksInternals(body) {
			t.Errorf("%s: status %d, retarget %q:\n%s", name, rec.Code, rec.Header().Get("HX-Retarget"), body)
		}
		// without JS the same request still gets the plain error page with a Back link
		plain := post(h, c.target, form)
		if plain.Code != c.status || !strings.Contains(plain.Body.String(), "<!doctype html>") ||
			!strings.Contains(plain.Body.String(), `<a href="/?sel=2">Back</a>`) || !strings.Contains(plain.Body.String(), c.msg) || leaksInternals(plain.Body.String()) || plain.Header().Get("HX-Retarget") != "" || plain.Header().Get("HX-Push-Url") != "" {
			t.Errorf("%s without JS: status %d", name, plain.Code)
		}
	}
	// dismissing is done by app.js; the page has a place for the bar and no clear route
	wantStatus(t, get(h, "/errors/clear"), http.StatusNotFound)
	wantBody(t, get(h, "/"), `<div id="errors" aria-live="polite"></div>`, `"responseHandling"`)
	js := get(h, "/static/app.js").Body.String()
	if !strings.Contains(js, "'.error-bar .dismiss'") || !strings.Contains(js, ".remove()") {
		t.Error("app.js must remove the error bar on Dismiss")
	}
}

// leaksInternals reports error-chain context that belongs in logs, not on screen.
func leaksInternals(body string) bool {
	for _, s := range []string{"invalid value", "add subtask to", "toggle subtask", "acknowledge scope", "constraint", "sqlite"} {
		if strings.Contains(body, s) {
			return true
		}
	}
	return false
}
