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
		`<div class="status mono">` + "\n" + `<span class="mode" id="status-mode">NORMAL</span>`,
		`<form class="vsearch" role="search" hidden>`,
		`<ul class="rows feed" data-pane="Journal">`,
		`<ul class="rows" data-pane="Tasks">`,
		`<li class="sub" tabindex="-1" data-entry data-id="`,
		`<li tabindex="-1" data-entry data-id="`,
		`<kbd>Ctrl</kbd>+<kbd>Enter</kbd></th><td>in the search line: show only the matches`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q", want)
		}
	}
	page := get(h, "/").Body.String()
	for _, want := range []string{
		`<p class="hint js-only small muted">Press <kbd>?</kbd> for keys</p>`,
		`<h2 id="open-tasks-title">Open tasks by priority</h2>` + "\n" + `<ul class="rows" data-pane="Open tasks">`,
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
	if !strings.Contains(css, "html.js .feed details.edit:not([open]) > .edit-toggle:not(:focus-visible) {") {
		t.Error("with scripting on, the feed's Edit hides until it has keyboard focus")
	}
	if !strings.Contains(css, "html.js [data-entry]:focus-visible {") {
		t.Error("the entry cursor must show only for keyboard focus with scripting on")
	}
}

// Key handling needs a browser (checked by hand in Firefox); this pins the rules
// that keep it from stealing keystrokes and from stacking up across htmx swaps.
func TestShortcutScriptRules(t *testing.T) {
	h, _ := newApp(t)
	js := get(h, "/static/app.js").Body.String()
	for _, want := range []string{
		"if (!window.worklineBound)", // registered once although app.js re-runs after every swap
		"(e.ctrlKey && !esc) || e.altKey || e.metaKey",
		"e.key === '[' && e.ctrlKey && !e.altKey && !e.metaKey", // Ctrl+[ is Esc
		"/^(INPUT|TEXTAREA|SELECT|SUMMARY)$/.test(t.tagName)",
		"if (editor) return;",                  // the editor's buttons don't take Normal-mode keys
		"ra.left - rb.left || ra.top - rb.top", // h/l follow the screen, not the markup
		"e.key === 'Escape'", "if (esc) {",
		"t.id === 'quickadd-text'", "t.blur()", // Esc in the bar's field only blurs it
		"e.key === 'Enter' && !e.shiftKey && !e.isComposing", "t.form.requestSubmit()", // Enter logs, Shift+Enter is a newline
		"var dialog = document.querySelector('dialog[open]');",             // the keys dialog keeps its keys
		"t.matches('[data-entry]') && t.querySelector('a.tag')",            // Enter on a link or button stays theirs
		"if (key === 'g' || key === 'd')", "Date.now() - pendingAt < 1000", // gg, gj, gt, gp, dd
		"gg:", "G:", "h:", "l:", "i:", "dd:", "x:", "o: focusBar", "'[':", "']':", "gj:", "gt:", "gp:", "O:", "f:", "'?':", "Escape:",
		"d.querySelector('details.confirm').open = true", // dd only opens the confirm step
		"'/': openSearch", "n: function () { search(1); }", "N: function () { search(-1); }",
		"t.id === 'vsearch-text'", // Enter in the search line searches instead of submitting the form
		"entry.classList.toggle('match', hit)",
		"fetch('/search/ids?'", "location.assign('/search?'", "e.ctrlKey || e.metaKey ? 'filter' : 'mark'", // Enter marks, Ctrl+Enter filters
		"if (pattern && hits) fetchHits(mark)",                                                     // marks survive swaps
		"'E486: Pattern not found on this page: '",                                                 // matches off the page are counted, not reported as none
		"var HISTORY = 'workline.searches'", "e.key === 'ArrowUp' || (e.ctrlKey && e.key === 'p')", // search history survives Ctrl+Enter
		"e.target.closest('.feed [data-entry]')", "!String(window.getSelection())", // a click on a journal row opens its editor
		"fetch('/search/words')", "complete(t, e.shiftKey ? -1 : 1)", "a.clear-search",
		"name = 'COMMAND'", "name = 'INSERT'", "a.closest('.editor')", // the statusline's mode; the editor counts as Insert
		"'E486: Pattern not found: '", "'search hit BOTTOM, continuing at TOP'", "'E35: No previous regular expression'",
		"showcmd(key)", "pane.dataset.pane + ' '",
		"htmx:afterSettle", // the cursor survives the swap after x, save or delete
		"document.querySelector('#quickadd-target .target')",
		"document.getElementById('filter-project')", "a.panel-close", "a.cancel-new", "a.new-activity",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("app.js lacks %q", want)
		}
	}
	if strings.Count(js, "addEventListener('keydown'") != 1 || strings.Count(js, "addEventListener('click'") != 2 || strings.Count(js, "addEventListener('submit'") != 1 {
		t.Error("document listeners: one keydown, one submit, one click here and one for the project toggles")
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
