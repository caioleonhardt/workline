package web

import (
	"strings"
	"testing"
)

// Controls that htmx submits on change keep a plain button for no-JS use; app.js
// and the stylesheet hide those buttons only when scripting is on.
func TestAutoSubmitControlsKeepTheirButtons(t *testing.T) {
	h, _ := newApp(t)
	body := get(h, "/?sel=1&project=1").Body.String()
	for _, want := range []string{
		`<form method="get" action="/" class="inline" hx-trigger="change, submit">`,
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

func TestSubtasksAreCheckboxes(t *testing.T) {
	h, _ := newApp(t)
	body := get(h, "/?sel=2").Body.String()
	// Kafka: six subtasks, two closed; each is a checkbox in a form that toggles it
	if got := strings.Count(body, `<input type="checkbox"`); got != 6 {
		t.Errorf("checkboxes = %d, want 6", got)
	}
	if got := strings.Count(body, `<input type="checkbox" checked>`); got != 2 {
		t.Errorf("checked = %d, want 2", got)
	}
	for _, want := range []string{
		`<form class="inline" method="post" action="/subtasks/13/toggle" hx-trigger="change, submit">`,
		`<span class="done-text">Compatibility matrix</span>`,
		`<button class="apply">Reopen</button>`, `<button class="apply">Close</button>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q", want)
		}
	}
}
