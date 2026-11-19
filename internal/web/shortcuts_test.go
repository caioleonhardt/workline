package web

import (
	"strings"
	"testing"
)

// The keys themselves live in app.js; the markup gives them targets and tells
// assistive technology about them (SPEC §10 item 68).
func TestShortcutTargetsAndHints(t *testing.T) {
	h, _ := newApp(t)
	body := get(h, "/?period=q4&sel=2&new=1").Body.String()
	for _, want := range []string{
		`<a class="btn new-activity" href="/?new=1&amp;period=q4&amp;sel=2" aria-keyshortcuts="n">New activity</a>`,
		`<select name="project" id="filter-project" aria-keyshortcuts="/">`,
		`<a class="panel-close" href="/?new=1&amp;period=q4" aria-label="Close panel" aria-keyshortcuts="Escape">×</a>`,
		`<a class="cancel-new" href="/?period=q4&amp;sel=2">Cancel</a>`,
		`<p class="hint js-only small muted">Shortcuts: <kbd>n</kbd> new activity · <kbd>/</kbd> filter · <kbd>j</kbd> journal · <kbd>Esc</kbd> close panel</p>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q", want)
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
		"e.key === 'j'", "document.querySelector('#quickadd-target .target')", // j only when the bar has a target
		"t.id === 'quickadd-text'", "t.blur()", // Esc in the bar's field only blurs it
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
