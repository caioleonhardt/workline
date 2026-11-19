package web

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"workline/internal/domain"
)

func TestFeedListsVisibleActivitiesNewestFirst(t *testing.T) {
	h, _ := newApp(t)
	rec := get(h, "/")
	body := rec.Body.String()
	wantBody(t, rec, `<h2 id="journal-title">Journal</h2>`, "Kick-off with manager, scope agreed",
		`<a class="tag small" href="/?sel=1">Order platform modernisation › Consolidate order services into one Go service</a>`)
	for _, done := range []string{"Recommendation sent", "Root cause: node pool autoscaling"} {
		if strings.Contains(body, done) {
			t.Errorf("entry %q of a done activity is in the default feed", done)
		}
	}
	wantBody(t, get(h, "/?status=done"), "Recommendation sent", "Root cause: node pool autoscaling")
	// newest first across activities: 2027 plan (Nov 16), Kafka (Nov 12), Consolidate (Nov 10)
	if !bodyOrderIn(body[strings.Index(body, `id="journal"`):], "Request received in 1:1", "Client upgrades pulled into this activity", "Kafka dependencies removed") {
		t.Error("feed is not newest first")
	}
}

func TestFeedFollowsTheFilters(t *testing.T) {
	h, st := newApp(t)
	feed := func(target string) string {
		body := get(h, target).Body.String()
		return body[strings.Index(body, `id="journal"`):]
	}
	if got := feed("/?project=3"); !strings.Contains(got, "Headcount approved") || strings.Contains(got, "Kick-off with manager") {
		t.Error("project filter")
	}
	if got := feed("/?creep=1"); !strings.Contains(got, "Kick-off with manager") || strings.Contains(got, "Headcount approved") {
		t.Error("creep filter")
	}
	// the period chooses activities (by overlap); their entries are not cut by date
	if got := feed("/?period=q1&status=done&status=active"); !strings.Contains(got, "Kick-off with manager") ||
		!strings.Contains(got, "Kafka dependencies removed") || strings.Contains(got, "Headcount approved") {
		t.Error("period filter picks activities, not entry dates")
	}
	if got := feed("/?period=q4&month=11"); !strings.Contains(got, "Request received in 1:1") || !strings.Contains(got, "Client upgrades pulled into this activity") {
		t.Error("month filter")
	}
	if err := st.SetProjectArchived(ctx, 3, true); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(feed("/"), "Headcount approved") {
		t.Error("archived project in the feed")
	}
}

func TestSelectionNarrowsTheFeed(t *testing.T) {
	h, _ := newApp(t)
	rec := get(h, "/?sel=2&period=q4")
	body := rec.Body.String()
	feed := body[strings.Index(body, `id="journal"`):]
	wantBody(t, rec, `<h2 id="journal-title">Journal · Kafka upgrade readiness <a class="small" href="/?period=q4">Show all</a></h2>`)
	if !strings.Contains(feed, "Waiting on platform team for a maintenance window") ||
		strings.Contains(feed, "Kick-off with manager") || strings.Contains(feed, `class="tag small"`) {
		t.Error("narrowed feed shows other activities or tags")
	}
	// even an activity the filters hide narrows the feed
	body = get(h, "/?sel=3").Body.String()
	feed = body[strings.Index(body, `id="journal"`):]
	if !strings.Contains(feed, "Last pipeline migrated") || strings.Contains(feed, "Kick-off with manager") {
		t.Error("hidden selected activity")
	}
}

func TestFeedLimitAndShowMore(t *testing.T) {
	h, st := newApp(t)
	for i := 1; i <= 35; i++ { // Kafka already has 4 entries: 39 in all
		if _, err := st.AddJournal(ctx, domain.JournalEntry{ActivityID: 2, Date: date(t, "2026-11-17"), Kind: domain.KindUpdate, Text: fmt.Sprintf("note %02d", i), LoggedAt: fmt.Sprintf("2026-11-17 10:%02d", i)}); err != nil {
			t.Fatal(err)
		}
	}
	count := func(body string) int { return strings.Count(body, `class="feed-text"`) }
	rec := get(h, "/?sel=2")
	if n := count(rec.Body.String()); n != 30 {
		t.Errorf("first page has %d entries, want 30", n)
	}
	wantBody(t, rec, `<a href="/?feed=60&amp;sel=2">Show 30 more</a>`, "note 35") // newest first: the latest note leads
	if strings.Contains(rec.Body.String(), "Waiting on platform team") {          // an older seeded entry is on the second page
		t.Error("entry beyond the limit shown")
	}
	rec = get(h, "/?sel=2&feed=60")
	if n := count(rec.Body.String()); n != 39 || strings.Contains(rec.Body.String(), "Show 30 more") {
		t.Errorf("second page: %d entries, more link present: %v", n, strings.Contains(rec.Body.String(), "Show 30 more"))
	}
	// the limit also applies to the unnarrowed feed
	if n := count(get(h, "/").Body.String()); n != 30 {
		t.Errorf("all-activities feed has %d entries", n)
	}
}

func TestFeedEntriesCanBeEditedExceptAcknowledgements(t *testing.T) {
	h, st := newApp(t)
	wantBody(t, get(h, "/?sel=2"), `<form class="inline" method="post" action="/journal/`, `Confirm delete`)
	if _, err := st.AckScope(ctx, 1, domain.FixedClock{D: date(t, testToday)}); err != nil {
		t.Fatal(err)
	}
	list, _ := st.ListJournal(ctx, 1)
	ack := fmt.Sprintf(`action="/journal/%d"`, list[0].ID)
	rec := get(h, "/?sel=1")
	wantBody(t, rec, `<span class="chip kind-scope_ack">Scope acknowledged</span>`)
	if strings.Contains(rec.Body.String(), ack) {
		t.Error("an acknowledgement entry has edit controls")
	}
}

func TestJournalLeftThePanel(t *testing.T) {
	h, _ := newApp(t)
	body := get(h, "/?sel=2").Body.String()
	panel := body[strings.Index(body, `id="panel"`):strings.Index(body, `id="journal"`)]
	if strings.Contains(panel, "<h3>Journal</h3>") || strings.Contains(panel, `action="/journal`) || strings.Contains(panel, "kind-") {
		t.Error("the panel still has a journal")
	}
	if n := strings.Count(body, `action="/journal"`); n != 1 { // only the quick-add bar logs entries
		t.Errorf("forms posting to /journal = %d", n)
	}
	wantStatus(t, hxPost(h, "/journal/1", url.Values{"kind": {"update"}, "text": {"edited"}, "date": {"2026-03-02"}, "next": {"/?sel=1"}}), http.StatusOK)
}

// The journal sits in the timeline's column, below it; the panel is beside both
// (and, on narrow screens, comes between them). Rects are checked in Firefox.
func TestJournalIsPartOfTheWorkGrid(t *testing.T) {
	h, _ := newApp(t)
	body := get(h, "/?sel=2").Body.String()
	work := body[strings.Index(body, `<div class="work">`):]
	if !bodyOrderIn(work, `<section class="card timeline"`, `<aside class="detail"`, `</aside>`, `<section class="card" id="journal"`) {
		t.Error("work grid order: timeline, panel, journal")
	}
	if !strings.Contains(work, "</aside>\n\n<section class=\"card\" id=\"journal\"") || !strings.Contains(work, "</section>\n\n</div>\n</div>") {
		t.Error("the journal must be the last item inside the work grid, closing it")
	}
	css := get(h, "/static/app.css").Body.String()
	for _, want := range []string{"#journal { grid-area: journal; align-self: start;", `grid-template-areas: "timeline" "panel" "journal"`, "grid-template-rows: auto 1fr;"} {
		if !strings.Contains(css, want) {
			t.Errorf("app.css lacks %q", want)
		}
	}
}

func TestEmptyFeedText(t *testing.T) {
	h, _ := newApp(t)
	wantBody(t, get(h, "/?status="), `<li class="muted">No journal entries for these filters.</li>`)
	if strings.Contains(get(h, "/").Body.String(), "No journal entries for these filters.") {
		t.Error("empty text shown for a non-empty feed")
	}
}
