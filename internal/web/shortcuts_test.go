package web

import (
	"strings"
	"testing"
)

// The keys themselves live in app.js; the markup gives them targets and tells
// assistive technology about them (SPEC §10 item 68).
func TestShortcutTargetsAndHints(t *testing.T) {
	h, _ := newApp(t)
	body := get(h, "/timeline?period=q4&sel=2&new=1").Body.String()
	for _, want := range []string{
		`<a class="btn new-activity" href="/timeline?new=1&amp;period=q4&amp;sel=2" aria-keyshortcuts="O">New activity</a>`,
		`<select name="project" id="filter-project" aria-keyshortcuts="f">`,
		`<a class="panel-close" href="/timeline?new=1&amp;period=q4" aria-label="Close panel" aria-keyshortcuts="Escape">×</a>`,
		`<a class="cancel-new" href="/timeline?period=q4&amp;sel=2">Cancel</a>`,
		`<p class="hint js-only small muted">Press <kbd>?</kbd> for keys</p>`,
		`<dialog id="keys" aria-labelledby="keys-title">`,
		`<form class="vsearch" role="search" hidden>`,
		`<ul class="rows feed" data-pane>`,
		`<ul class="rows" data-pane>`,
		`<li class="sub" tabindex="-1" data-entry>`,
		`<li tabindex="-1" data-entry>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q", want)
		}
	}
	page := get(h, "/").Body.String()
	for _, want := range []string{
		`<p class="hint js-only small muted">Press <kbd>?</kbd> for keys</p>`,
		`<h2 id="open-tasks-title">Open tasks by priority</h2>` + "\n" + `<ul class="rows" data-pane>`,
		`<dialog id="keys" aria-labelledby="keys-title">`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("journal page missing %q", want)
		}
	}
	css := get(h, "/static/app.css").Body.String()
	if !strings.Contains(css, ".js-only { display: none; }") || !strings.Contains(css, "html.js .js-only { display: block; }") {
		t.Error("the hint must show only when scripting is on")
	}
}

// Key handling needs a browser (checked by hand in Firefox); this pins the rules
// that keep it from stealing keystrokes and from stacking up across htmx swaps.
func TestShortcutScriptRules(t *testing.T) {
	h, _ := newApp(t)
	js := get(h, "/static/app.js").Body.String()
	for _, want := range []string{
		"if (!window.worklineBound)", // registered once although app.js re-runs after every swap
		"e.ctrlKey || e.altKey || e.metaKey",
		"/^(INPUT|TEXTAREA|SELECT|SUMMARY)$/.test(t.tagName)",
		"e.key === 'n' && !cancel", "e.key === 'Escape'", "e.key === '/'",
		"e.key === 'j' || barKinds[e.key]", "document.querySelector('#quickadd-target .target')", // j, t, d, u, b, m focus the bar
		"t: 'task', d: 'decision', u: 'update', b: 'blocker', m: 'meeting'", "select.value = kind",
		"} else if (focusBar()) {",             // / focuses the bar where there is no filter, instead of the browser's quick find
		"t.id === 'quickadd-text'", "t.blur()", // Esc in the bar's field only blurs it
		"e.key === 'Enter' && !e.shiftKey && !e.isComposing", "t.form.requestSubmit()", // Enter logs, Shift+Enter is a newline
		"document.getElementById('filter-project')", "a.panel-close", "a.cancel-new", "a.new-activity",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("app.js lacks %q", want)
		}
	}
	if strings.Count(js, "addEventListener('keydown'") != 1 || strings.Count(js, "addEventListener('click'") != 2 {
		t.Error("document listeners: one keydown, one click here and one for the project toggles")
	}
}

// Completion popups float above the page when scripting is on, so a scrolling
// timeline cannot clip them (SPEC §10 item 96).
func TestCompletionPopupsFloat(t *testing.T) {
	h, _ := newApp(t)
	css := get(h, "/static/app.css").Body.String()
	js := get(h, "/static/app.js").Body.String()
	for _, want := range []string{"html.js .done-dot .popup,", "position: fixed;", "html.js .done-dot.open .popup { display: block; }"} {
		if !strings.Contains(css, want) {
			t.Errorf("app.css lacks %q", want)
		}
	}
	for _, want := range []string{"function placePopup(dotWrap)", "getBoundingClientRect()", "window.innerWidth - w - margin", "addEventListener('scroll'"} {
		if !strings.Contains(js, want) {
			t.Errorf("app.js lacks %q", want)
		}
	}
}
