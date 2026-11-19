package web

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"workline/internal/domain"
)

const kafkaTarget = `<span class="target">Journal for <strong>Kafka upgrade readiness</strong> · Order platform modernisation</span>` +
	`<input type="hidden" name="activity" value="2"><input type="hidden" name="next" value="/?sel=2">`

func TestQuickAddBarOutsideTheSwapRegion(t *testing.T) {
	h, _ := newApp(t)
	body := get(h, "/").Body.String()
	if n := strings.Count(body, `<form id="quickadd"`); n != 1 {
		t.Fatalf("quick-add forms = %d", n)
	}
	if strings.Index(body, `<form id="quickadd"`) < strings.Index(body, `<aside class="detail"`) {
		t.Error("the bar must come after the swap region")
	}
	wantBody(t, get(h, "/"), `<form id="quickadd" class="quick" method="post" action="/journal" hx-post="/journal" hx-target="#dash" hx-swap="outerHTML">`,
		`<span id="quickadd-target"></span>`, `<option value="update" selected>Update</option>`,
		`<input id="quickadd-text" name="text" aria-label="Journal entry"`)
	// htmx fragments never contain the bar itself, only the target update
	frag := hxGet(h, "/?sel=2")
	if strings.Contains(frag.Body.String(), `<form id="quickadd"`) || strings.Contains(frag.Body.String(), `id="quickadd-text"`) {
		t.Error("a fragment must not replace the bar or its input")
	}
}

func TestQuickAddTargetFollowsTheSelection(t *testing.T) {
	h, st := newApp(t)
	wantBody(t, get(h, "/?sel=2"), `<span id="quickadd-target">`+kafkaTarget+`</span>`)
	// every fragment updates the target out of band, so a selection change retargets the bar
	wantBody(t, hxGet(h, "/?sel=2"), `<span id="quickadd-target" hx-swap-oob="innerHTML">`+kafkaTarget+`</span>`)
	rec := hxGet(h, "/?sel=1&status=done")
	wantBody(t, rec, `<span id="quickadd-target" hx-swap-oob="innerHTML"><span class="target">Journal for <strong>Consolidate order services into one Go service</strong>`,
		`name="next" value="/?sel=1&amp;status=done"`)
	// no selection, an unknown one and an archived project's activity give no target
	wantBody(t, hxGet(h, "/"), `<span id="quickadd-target" hx-swap-oob="innerHTML"></span>`)
	wantBody(t, hxGet(h, "/?sel=999"), `<span id="quickadd-target" hx-swap-oob="innerHTML"></span>`)
	if err := st.SetProjectArchived(ctx, 1, true); err != nil {
		t.Fatal(err)
	}
	wantBody(t, hxGet(h, "/?sel=2"), `<span id="quickadd-target" hx-swap-oob="innerHTML"></span>`)
}

func TestQuickAddWithoutJS(t *testing.T) {
	h, st := newApp(t)
	rec := post(h, "/journal", url.Values{"activity": {"2"}, "kind": {"update"}, "text": {"Called the platform team"}, "next": {"/?sel=2&period=q4"}})
	wantStatus(t, rec, http.StatusSeeOther)
	if loc := rec.Header().Get("Location"); loc != "/?sel=2&period=q4" {
		t.Errorf("Location = %q", loc)
	}
	list, _ := st.ListJournal(ctx, 2)
	e := list[0]
	if e.Text != "Called the platform team" || e.Kind != domain.KindUpdate || e.Date != date(t, testToday) || e.LoggedAt != "2026-11-18 14:05" {
		t.Errorf("entry: %+v", e)
	}
	// blank text is the plain error page with a Back link
	bad := post(h, "/journal", url.Values{"activity": {"2"}, "kind": {"update"}, "text": {"  "}, "next": {"/?sel=2"}})
	wantStatus(t, bad, http.StatusUnprocessableEntity)
	wantBody(t, bad, "Journal text is required.", `<a href="/?sel=2">Back</a>`)
}

func TestQuickAddBarStyles(t *testing.T) {
	h, _ := newApp(t)
	css := get(h, "/static/app.css").Body.String()
	for _, want := range []string{
		".quick { position: fixed; left: 0; right: 0; bottom: 0;", ".quick:not(:has(.target)) { display: none; }",
		"body:has(.quick .target) { padding-bottom: 96px; }", "html.js .apply-nojs { display: none; }",
	} {
		if !strings.Contains(css, want) {
			t.Errorf("app.css lacks %q", want)
		}
	}
}
