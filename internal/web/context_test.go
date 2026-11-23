package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// withCookie sends a request carrying the context cookie.
func withCookie(h http.Handler, method, target string, form url.Values, value string) *httptest.ResponseRecorder {
	var body *strings.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	} else {
		body = strings.NewReader("")
	}
	req := httptest.NewRequest(method, target, body)
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	req.AddCookie(&http.Cookie{Name: contextCookie, Value: value})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// `:p` and `:a` set where new entries go, kept in a cookie (SPEC §10 item 121).
func TestContextCommand(t *testing.T) {
	h, st := newApp(t)
	set := func(q string) *httptest.ResponseRecorder {
		return post(h, "/context", url.Values{"q": {q}, "next": {"/tasks"}})
	}
	cookie := func(rec *httptest.ResponseRecorder) *http.Cookie {
		for _, c := range rec.Result().Cookies() {
			if c.Name == contextCookie {
				return c
			}
		}
		return nil
	}
	for q, want := range map[string]string{
		"p opm":                    "p1", // initials of "Order platform modernisation"
		"p 2":                      "p2",
		"a kafka":                  "2",
		"p   Platform and tooling": "p2",
	} {
		rec := set(q)
		c := cookie(rec)
		if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/tasks" || c == nil || c.Value != want || !c.HttpOnly {
			t.Errorf(":%s → %d %v, want cookie %s", q, rec.Code, c, want)
		}
	}
	// alone, it clears
	if c := cookie(set("p")); c == nil || c.MaxAge >= 0 {
		t.Errorf(":p alone does not clear: %v", c)
	}
	// errors are plain text for the statusline, and set nothing
	for q, want := range map[string]string{
		"x foo":  "E492: Not a command: x",
		"p zzz":  `No project matches "zzz"`,
		"a 999":  `No activity matches "999"`,
		"a plan": `activity names match "plan"`,
	} {
		rec := set(q)
		if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), want) || cookie(rec) != nil {
			t.Errorf(":%s → %d %q", q, rec.Code, rec.Body.String())
		}
	}
	// an archived project cannot be picked
	if err := st.SetProjectArchived(ctx, 4, true); err != nil {
		t.Fatal(err)
	}
	if rec := set("p 4"); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("archived project picked: %d", rec.Code)
	}
}

// The cookie makes the context the bar's default on every page, shown in the
// statusline; a dead context is ignored.
func TestContextIsTheBarDefault(t *testing.T) {
	h, st := newApp(t)
	// pages with a select preselect it
	rec := withCookie(h, http.MethodGet, "/", nil, "p1")
	wantBody(t, rec, `<option value="p1" selected>Order platform modernisation</option>`,
		`<span class="status-ctx" title="New entries go to Order platform modernisation; :p or :a alone clears it">[Order platform modernisation]</span>`)
	// the roadmap targets it when nothing is selected, the selection wins
	wantBody(t, withCookie(h, http.MethodGet, "/roadmap", nil, "2"), `Journal for <strong>Kafka upgrade readiness</strong> · Order platform modernisation`, `<input type="hidden" name="activity" value="2">`,
		"[Order platform modernisation › Kafka upgrade readiness]")
	wantBody(t, withCookie(h, http.MethodGet, "/roadmap?sel=1", nil, "p2"), `<input type="hidden" name="activity" value="1">`)
	wantBody(t, withCookie(h, http.MethodGet, "/roadmap", nil, "p2"), `<input type="hidden" name="activity" value="p2">`)

	// a POST from the bar carries it; without the cookie nothing changes server-side
	wantStatus(t, withCookie(h, http.MethodPost, "/journal", url.Values{"activity": {"p2"}, "kind": {"update"}, "text": {"In context"}, "next": {"/"}}, "p2"), http.StatusSeeOther)
	if all, _ := st.ListAllJournal(ctx); all[0].ProjectID != 2 {
		t.Errorf("entry not in the context: %+v", all[0])
	}
	wantStatus(t, post(h, "/journal", url.Values{"kind": {"update"}, "text": {"No context"}, "next": {"/"}}), http.StatusSeeOther)
	if all, _ := st.ListAllJournal(ctx); !all[0].Unassigned() {
		t.Errorf("entry without context: %+v", all[0])
	}

	// dead or bad contexts are ignored
	if err := st.SetProjectArchived(ctx, 1, true); err != nil {
		t.Fatal(err)
	}
	for _, v := range []string{"p1", "999", "junk"} {
		if strings.Contains(withCookie(h, http.MethodGet, "/", nil, v).Body.String(), "status-ctx") {
			t.Errorf("context %q shown", v)
		}
	}
}
