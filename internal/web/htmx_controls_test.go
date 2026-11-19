package web

import (
	"strings"
	"testing"
)

// Controls that htmx submits on change keep a plain button for no-JS use; app.js
// and the stylesheet hide those buttons only when scripting is on.
func TestAutoSubmitControlsKeepTheirButtons(t *testing.T) {
	h, _ := newApp(t)
	body := get(h, "/timeline?sel=1&project=1").Body.String()
	for _, want := range []string{
		`<form method="get" action="/timeline" class="inline" hx-trigger="change, submit">`,
		`<button class="apply">Apply</button>`,
		`<form class="inline" method="post" action="/activities/1" hx-trigger="change, submit">`,
		`<button class="apply">Save status</button>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q", want)
		}
	}
	css := get(h, "/static/app.css").Body.String()
	js := get(h, "/static/app.js").Body.String()
	if !strings.Contains(css, "html.js .apply { display: none; }") || !strings.Contains(js, "classList.add('js')") {
		t.Error("apply buttons must be hidden only when app.js has run")
	}
}

func TestTasksAreCheckboxes(t *testing.T) {
	h, _ := newApp(t)
	body := get(h, "/timeline?sel=2").Body.String()
	// Kafka: six tasks, two closed; each is a checkbox in a form that toggles it,
	// once in the panel and once in the feed, so a closed task can be reopened from either
	if got := strings.Count(body, `<input type="checkbox"`); got != 12 {
		t.Errorf("checkboxes = %d, want 12", got)
	}
	if got := strings.Count(body, `<input type="checkbox" checked>`); got != 2 {
		t.Errorf("checked in the panel = %d, want 2", got)
	}
	// In the feed the checkbox leads the row, so rows line up, and is named by the task's text.
	if got := strings.Count(body, `" checked>`+"\n"+`<button class="apply">Reopen</button>`); got != 2 {
		t.Errorf("Reopen forms in the feed = %d, want 2", got)
	}
	if got := strings.Count(body, `">`+"\n"+`<form class="inline sub-toggle feed-toggle" method="post" action="/tasks/`); got != 6 {
		t.Errorf("feed rows led by their task's checkbox = %d, want 6", got)
	}
	if !strings.Contains(body, `class="feed-text done-text">Compatibility matrix</span>`) {
		t.Error("the closed task's text stays in the feed")
	}
	if !strings.Contains(body, `<span class="feed-toggle" aria-hidden="true"></span>`) {
		t.Error("rows without a task keep the checkbox's place")
	}
	for _, want := range []string{
		`<form class="inline sub-toggle" method="post" action="/tasks/13/toggle" hx-trigger="change, submit">`,
		`<span class="done-text">Compatibility matrix</span>`,
		`<button class="apply">Reopen</button>`, `<button class="apply">Close</button>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q", want)
		}
	}
}

// A task is one line: one date (closed, else opened) with both in the tooltip.
func TestTaskRowsShowOneDate(t *testing.T) {
	h, _ := newApp(t)
	body := get(h, "/timeline?sel=2").Body.String() // Kafka: 2 closed, 4 open
	for _, want := range []string{
		`<span class="muted mono small" title="opened 2026-09-07 · closed 2026-10-05">closed 2026-10-05</span>`,
		`<span class="muted mono small" title="opened 2026-10-05">opened 2026-10-05</span>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q", want)
		}
	}
	if strings.Contains(body, "opened 2026-09-07 · closed") && strings.Contains(body, ">opened 2026-09-07 · closed") {
		t.Error("both dates shown in the row")
	}
	// the opened date stays editable
	wantBody(t, get(h, "/timeline?sel=2"), `<label>Opened<input type="date" name="date" value="2026-09-07" required></label>`)
}
