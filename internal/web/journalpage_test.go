package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"workline/internal/domain"
)

// The journal page shows the last two weeks: tiles, one burndown row for all
// tasks, open tasks by priority and the entries dated in the range
// (SPEC §10 item 95). testToday is Wednesday 2026-11-18.
func TestJournalPage(t *testing.T) {
	h, st := newApp(t)
	if _, err := st.AddJournal(ctx, domain.JournalEntry{Date: date(t, "2026-11-17"), Kind: domain.KindTask, Text: "Loose urgent task", Priority: domain.PriorityHigh}); err != nil {
		t.Fatal(err)
	}
	rec := get(h, "/")
	wantStatus(t, rec, http.StatusOK)
	body := rec.Body.String()
	wantBody(t, rec, "2026-11-09 – 2026-11-22", `<a href="/?week=2026-11-09" aria-label="Previous week">‹</a>`,
		`<a href="/?week=2026-11-23" aria-label="Next week">›</a>`, `href="/export/journal.md?from=2026-11-09&amp;to=2026-11-22" hx-boost="false"`,
		`<div class="col now" style="left:64.29%;width:7.14%"><span class="col-title">Wed <span class="muted">18</span>`,
		`<span class="today-flag" style="left:67.86%">Today</span>`,
		`<span class="done-dot left" style="left:10.71%"><button type="button" class="dot" aria-label="1 task completed on 2026-11-10" aria-describedby="done-all-2026-11-10">`,
		"Remove Kafka topic dependencies")
	if strings.Contains(body, "This week</a>") {
		t.Error("this week offers a This week link")
	}
	// tiles count tasks of the range (Unassigned too); bins stop at today
	for label, want := range map[string]int{"Tasks opened": 6, "Tasks closed": 1, "Open tasks now": 18, "High priority open": 1} {
		if got := tileValue(t, body, label); got != want {
			t.Errorf("tile %s = %d, want %d", label, got, want)
		}
	}
	if n := strings.Count(body, `<span class="bin active"`); n != 10 {
		t.Errorf("%d daily bins, want 10 (Nov 9 to today)", n)
	}
	// open tasks: high priority first, with its Unassigned tag; activity links load the timeline
	tasks := body[strings.Index(body, `id="open-tasks-title"`):strings.Index(body, `id="journal"`)]
	if !bodyOrderIn(tasks, "Loose urgent task", "Unassigned", "Kafka") || strings.Contains(tasks, "Remove Kafka topic dependencies") {
		t.Error("open tasks are not by priority, or include closed ones")
	}
	wantBody(t, rec, `<a class="tag small" href="/timeline?sel=2" hx-boost="false">`)
	// the feed holds only entries dated in the range, newest first
	feed := body[strings.Index(body, `id="journal"`):]
	if !bodyOrderIn(feed, "Loose urgent task", "Request received in 1:1", "Client upgrades pulled into this activity", "Kafka dependencies removed") ||
		strings.Contains(feed, "Waiting on platform team") {
		t.Error("feed range or order")
	}
	// the bar picks the activity itself, starting at Unassigned
	wantBody(t, rec, `<span id="quickadd-target"><span class="target"><label for="quickadd-activity">Log to</label></span><input type="hidden" name="next" value="/"></span>`,
		`<select id="quickadd-activity" name="activity"><option value="" selected>Unassigned</option><optgroup label=`)
}

func TestJournalPageWeeks(t *testing.T) {
	h, _ := newApp(t)
	for target, want := range map[string]string{
		"/?week=2026-11-04": "2026-10-26 – 2026-11-08", // any day names its week
		"/?week=bogus":      "2026-11-09 – 2026-11-22",
	} {
		wantBody(t, get(h, target), want)
	}
	wantBody(t, get(h, "/?week=2026-11-09"), `<a class="small" href="/">This week</a>`, `<a href="/" aria-label="Next week">›</a>`)
	rec := get(h, "/?week=2026-11-02")
	if strings.Contains(rec.Body.String(), "today-flag") || strings.Contains(rec.Body.String(), "Request received in 1:1") {
		t.Error("a past range shows today or later entries")
	}
	// a future range has no bins yet
	if strings.Contains(get(h, "/?week=2026-12-07").Body.String(), `<span class="bin active"`) {
		t.Error("bins drawn after today")
	}
	// any other key on / is timeline state
	wantStatus(t, get(h, "/?week=2026-11-02&sel=2"), http.StatusFound)
}

// The page swaps like the timeline: htmx gets its fragment, and mutations sent
// from it answer with it (SPEC §10 item 95).
func TestJournalPageMutations(t *testing.T) {
	h, st := newApp(t)
	frag := hxGet(h, "/?week=2026-11-02")
	if !strings.HasPrefix(strings.TrimSpace(frag.Body.String()), `<div id="dash" class="journal-page"`) || strings.Contains(frag.Body.String(), "<html") {
		t.Error("not a journal fragment")
	}
	wantBody(t, frag, `<span id="quickadd-target" hx-swap-oob="innerHTML">`)

	rec := hxPostFrom(h, "/journal", url.Values{"activity": {"2"}, "kind": {"update"}, "text": {"/t1 From the 1:1"}, "next": {"/"}}, "quickadd")
	if rec.Code != http.StatusOK || rec.Header().Get("HX-Push-Url") != "/" || !strings.Contains(rec.Body.String(), `class="journal-page"`) {
		t.Fatalf("log from the page: %d %q", rec.Code, rec.Header().Get("HX-Push-Url"))
	}
	wantBody(t, rec, `<textarea id="quickadd-text"`, `<span class="chip prio-high">High</span>`)
	ts, _ := st.ListTasks(ctx, 2)
	added := ts[len(ts)-1]
	if added.Text != "From the 1:1" || added.Priority != domain.PriorityHigh {
		t.Fatalf("logged: %+v", added)
	}
	before := tileValue(t, get(h, "/").Body.String(), "Tasks closed")
	rec = hxPost(h, "/tasks/"+strconv.FormatInt(added.ID, 10)+"/toggle", url.Values{"next": {"/"}})
	if rec.Code != http.StatusOK || tileValue(t, rec.Body.String(), "Tasks closed") != before+1 {
		t.Errorf("toggle from the page: %d", rec.Code)
	}
	// without JS it is a redirect back to the page
	plain := post(h, "/tasks/"+strconv.FormatInt(added.ID, 10)+"/toggle", url.Values{"next": {"/?week=2026-11-02"}})
	if loc := plain.Header().Get("Location"); plain.Code != http.StatusSeeOther || loc != "/?week=2026-11-02" {
		t.Errorf("without JS: %d %q", plain.Code, loc)
	}
}

// A task opened before the weeks shown and closed in them is in the feed, at
// its close (SPEC §10 item 109).
func TestJournalPageFeedShowsClosedTasks(t *testing.T) {
	h, st := newApp(t)
	task, err := st.AddTask(ctx, 2, "Closed for the 1:1", date(t, "2026-10-01"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.ToggleTask(ctx, task.ID, domain.FixedClock{D: date(t, "2026-11-12"), Hour: 16, Minute: 30}); err != nil {
		t.Fatal(err)
	}
	for name, rec := range map[string]*httptest.ResponseRecorder{"page": get(h, "/"), "fragment": hxGet(h, "/")} {
		body := rec.Body.String()
		feed := body[strings.Index(body, `id="journal"`):]
		if !bodyOrderIn(feed, "Request received in 1:1", `<span class="muted mono">2026-11-12 16:30</span>`, "Closed for the 1:1", "opened 2026-10-01", "Kafka dependencies removed") {
			t.Errorf("%s: closed task not in the feed at its close", name)
		}
	}
	// the week it was opened in no longer lists it
	if body := get(h, "/?week=2026-10-05").Body.String(); strings.Contains(body[strings.Index(body, `id="journal"`):], "Closed for the 1:1") {
		t.Error("closed task still in the feed of its opening week")
	}
}

func TestJournalPageHidesArchivedProjects(t *testing.T) {
	h, st := newApp(t)
	if err := st.SetProjectArchived(ctx, 4, true); err != nil { // holds "Request received in 1:1"
		t.Fatal(err)
	}
	if strings.Contains(get(h, "/").Body.String(), "Request received in 1:1") {
		t.Error("archived project's entry on the journal page")
	}
}
