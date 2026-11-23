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

// Home is the journal of one day, today by default (SPEC §10 item 124).
// testToday is Wednesday 2026-11-18.
func TestJournalPage(t *testing.T) {
	h, st := newApp(t)
	if _, err := st.AddJournal(ctx, domain.JournalEntry{Date: date(t, "2026-11-18"), Kind: domain.KindTask, Text: "Loose urgent task", Priority: domain.PriorityHigh}); err != nil {
		t.Fatal(err)
	}
	rec := get(h, "/")
	wantStatus(t, rec, http.StatusOK)
	wantBody(t, rec, `<strong class="mono">2026-11-18</strong>`,
		`<a href="/?date=2026-11-17" aria-label="Previous day">‹</a>`, `<a href="/?date=2026-11-19" aria-label="Next day">›</a>`,
		`href="/export/journal.md?from=2026-11-18&amp;to=2026-11-18" hx-boost="false"`, `href="/report" hx-boost="false"`,
		"Loose urgent task")
	body := rec.Body.String()
	if strings.Contains(body, "today-link") || strings.Contains(body, "tile") || strings.Contains(body, "open-tasks") {
		t.Error("today offers a Today link, or home still shows tiles or open tasks")
	}
	// only the day's entries
	if strings.Contains(body, "Request received in 1:1") || strings.Contains(body, "Kafka dependencies removed") {
		t.Error("home lists entries of other days")
	}
	// the bar picks the activity itself, starting at Unassigned, and logs to today
	wantBody(t, rec, `<span id="quickadd-target"><span class="target"><label for="quickadd-activity">Log to</label></span><input type="hidden" name="next" value="/"></span>`,
		`<select id="quickadd-activity" name="activity"><option value="" selected>Unassigned</option><optgroup label=`)
}

func TestJournalPageDays(t *testing.T) {
	h, _ := newApp(t)
	rec := get(h, "/?date=2026-11-16")
	wantBody(t, rec, `<strong class="mono">2026-11-16</strong>`, "Request received in 1:1",
		`<a href="/?date=2026-11-17" aria-label="Next day">›</a>`, `<a class="small today-link" href="/">Today</a>`,
		// the bar logs to the day shown
		`<input type="hidden" name="date" value="2026-11-16"><input type="hidden" name="next" value="/?date=2026-11-16">`)
	wantBody(t, get(h, "/?date=2026-11-17"), `<a href="/" aria-label="Next day">›</a>`)
	// future days are allowed; a bad date is today
	wantBody(t, get(h, "/?date=2026-12-01"), `<strong class="mono">2026-12-01</strong>`)
	wantBody(t, get(h, "/?date=bogus"), `<strong class="mono">2026-11-18</strong>`)
	// old week links land on today; any other key on / is timeline state
	wantStatus(t, get(h, "/?week=2026-11-02"), http.StatusOK)
	wantStatus(t, get(h, "/?date=2026-11-02&sel=2"), http.StatusFound)
}

// The page swaps like the timeline: htmx gets its fragment, and mutations sent
// from it answer with it. Logging from a past day logs on that day.
func TestJournalPageMutations(t *testing.T) {
	h, st := newApp(t)
	frag := hxGet(h, "/?date=2026-11-02")
	if !strings.HasPrefix(strings.TrimSpace(frag.Body.String()), `<div id="dash" class="journal-page"`) || strings.Contains(frag.Body.String(), "<html") {
		t.Error("not a journal fragment")
	}
	wantBody(t, frag, `<span id="quickadd-target" hx-swap-oob="innerHTML">`, `<input type="hidden" name="date" value="2026-11-02">`)

	rec := hxPostFrom(h, "/journal", url.Values{"activity": {"2"}, "kind": {"update"}, "date": {"2026-11-02"}, "text": {"/t1 Back-filled"}, "next": {"/?date=2026-11-02"}}, "quickadd")
	if rec.Code != http.StatusOK || rec.Header().Get("HX-Push-Url") != "/?date=2026-11-02" || !strings.Contains(rec.Body.String(), `class="journal-page"`) {
		t.Fatalf("log from the page: %d %q", rec.Code, rec.Header().Get("HX-Push-Url"))
	}
	wantBody(t, rec, `<textarea id="quickadd-text"`, `<span class="chip prio-high">High</span>`, "Back-filled")
	ts, _ := st.ListTasks(ctx, 2)
	added := ts[len(ts)-1]
	if added.Text != "Back-filled" || added.Priority != domain.PriorityHigh || added.CreatedOn != date(t, "2026-11-02") {
		t.Fatalf("logged: %+v", added)
	}
	// closing it today moves it to today's page (item 109)
	rec = hxPost(h, "/tasks/"+strconv.FormatInt(added.ID, 10)+"/toggle", url.Values{"next": {"/"}})
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Back-filled") {
		t.Errorf("toggle from the page: %d", rec.Code)
	}
	// without JS it is a redirect back to the page
	plain := post(h, "/tasks/"+strconv.FormatInt(added.ID, 10)+"/toggle", url.Values{"next": {"/?date=2026-11-02"}})
	if loc := plain.Header().Get("Location"); plain.Code != http.StatusSeeOther || loc != "/?date=2026-11-02" {
		t.Errorf("without JS: %d %q", plain.Code, loc)
	}
}

// A task opened earlier and closed on a day is in that day's feed, at its
// close, and not in the feed of the day it was opened (SPEC §10 item 109).
func TestJournalPageFeedShowsClosedTasks(t *testing.T) {
	h, st := newApp(t)
	task, err := st.AddTask(ctx, 2, "Closed later", date(t, "2026-10-01"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.ToggleTask(ctx, task.ID, domain.FixedClock{D: date(t, "2026-11-12"), Hour: 16, Minute: 30}); err != nil {
		t.Fatal(err)
	}
	for name, rec := range map[string]*httptest.ResponseRecorder{"page": get(h, "/?date=2026-11-12"), "fragment": hxGet(h, "/?date=2026-11-12")} {
		body := rec.Body.String()
		feed := body[strings.Index(body, `id="journal"`):]
		if !bodyOrderIn(feed, `<li class="day-head"><h2>12/11/2026</h2></li>`, `<span class="muted mono">16:30</span>`, "Closed later", "opened 2026-10-01") {
			t.Errorf("%s: closed task not in the feed at its close", name)
		}
	}
	if body := get(h, "/?date=2026-10-01").Body.String(); strings.Contains(body[strings.Index(body, `id="journal"`):], "Closed later") {
		t.Error("closed task still in the feed of its opening day")
	}
}

func TestJournalPageHidesArchivedProjects(t *testing.T) {
	h, st := newApp(t)
	if err := st.SetProjectArchived(ctx, 4, true); err != nil { // holds "Request received in 1:1"
		t.Fatal(err)
	}
	if strings.Contains(get(h, "/?date=2026-11-16").Body.String(), "Request received in 1:1") {
		t.Error("archived project's entry on the journal page")
	}
}

// The week view shows the 7 days up to the date, newest first, under day
// headings; rows show only their time (SPEC §10 item 127). testToday is
// Wednesday 2026-11-18.
func TestJournalPageWeek(t *testing.T) {
	h, st := newApp(t)
	if _, err := st.AddJournal(ctx, domain.JournalEntry{Date: date(t, testToday), Kind: domain.KindUpdate, Text: "Today's update", LoggedAt: testToday + " 09:15"}); err != nil {
		t.Fatal(err)
	}
	rec := get(h, "/?view=week")
	wantStatus(t, rec, http.StatusOK)
	wantBody(t, rec, `<strong class="mono">2026-11-12 – 2026-11-18</strong>`,
		`<a href="/?date=2026-11-11&amp;view=week" aria-label="Previous week">‹</a>`, `<a href="/?date=2026-11-25&amp;view=week" aria-label="Next week">›</a>`,
		`<a class="pill view-day" href="/">Day</a><a class="pill view-week" href="/?view=week" aria-current="page">Week</a>`,
		`href="/export/journal.md?from=2026-11-12&amp;to=2026-11-18"`, `href="/report?from=2026-11-12&amp;to=2026-11-18"`)
	body := rec.Body.String()
	feed := body[strings.Index(body, `id="journal"`):]
	if !bodyOrderIn(feed, "<h2>18/11/2026</h2>", `<span class="muted mono">09:15</span>`, "Today&#39;s update", "<h2>16/11/2026</h2>", "Request received in 1:1") {
		t.Error("week feed is not in day sections, newest first")
	}
	if strings.Count(feed, `class="day-head"`) != 3 || !strings.Contains(feed, "<h2>12/11/2026</h2>") || strings.Contains(feed, "Kafka dependencies removed") {
		t.Error("a section per day with entries, nothing older than the week")
	}
	// the day view of another date keeps the view's links; Today keeps the week
	wantBody(t, get(h, "/?date=2026-11-16&view=week"), `<a class="small today-link" href="/?view=week">Today</a>`)
	wantBody(t, get(h, "/?date=2026-11-16"), `<a class="pill view-week" href="/?date=2026-11-16&amp;view=week">Week</a>`,
		`<a class="pill view-day" href="/?date=2026-11-16" aria-current="page">Day</a>`, "<h2>16/11/2026</h2>")
	// view is journal state, not roadmap state
	wantStatus(t, get(h, "/?view=week&date=2026-11-02"), http.StatusOK)
}
