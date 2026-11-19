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
		`<p class="hint js-only small muted">Shortcuts: <kbd>n</kbd> new activity · <kbd>/</kbd> filter · <kbd>Esc</kbd> close panel</p>`,
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
