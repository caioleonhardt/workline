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

func TestJournalFlows(t *testing.T) {
	h, st := newApp(t)
	next := "/?sel=2&creep=1"
	form := func(kv ...string) url.Values {
		f := url.Values{"next": {next}}
		for i := 0; i < len(kv); i += 2 {
			f.Set(kv[i], kv[i+1])
		}
		return f
	}

	rec := post(h, "/journal", form("activity", "2", "kind", "decision", "text", "Ship it"))
	wantStatus(t, rec, http.StatusSeeOther)
	if loc := rec.Header().Get("Location"); loc != next {
		t.Errorf("Location = %q", loc)
	}
	wantStatus(t, post(h, "/journal", form("activity", "2", "kind", "meeting", "text", "Sync", "date", "2026-11-01")), http.StatusSeeOther)
	list, _ := st.ListJournal(ctx, 2)
	if list[0].Text != "Ship it" || list[0].Date != date(t, testToday) || list[0].Kind != domain.KindDecision {
		t.Errorf("newest first, default date today: %+v", list[0])
	}
	wantBody(t, get(h, "/?sel=2"), "Ship it", "Sync", `<span class="chip kind-decision">Decision</span>`, `<span class="chip kind-meeting">Meeting</span>`)
	if list[0].LoggedAt != "2026-11-18 14:05" {
		t.Errorf("entry stamped %q, want the clock's time", list[0].LoggedAt)
	}

	for name, f := range map[string]url.Values{
		"scope_ack kind": form("activity", "2", "kind", "scope_ack", "text", "x"),
		"blank text":     form("activity", "2", "kind", "update", "text", " "),
		"bad kind":       form("activity", "2", "kind", "gossip", "text", "x"),
		"bad date":       form("activity", "2", "kind", "update", "text", "x", "date", "yesterday"),
	} {
		if rec := post(h, "/journal", f); rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("%s: %d", name, rec.Code)
		}
	}

	id := strconv.FormatInt(list[0].ID, 10)
	wantStatus(t, post(h, "/journal/"+id, form("kind", "blocker", "text", "Edited", "date", "2026-11-02")), http.StatusSeeOther)
	got, _ := st.ListJournal(ctx, 2)
	var edited domain.JournalEntry
	for _, j := range got {
		if j.ID == list[0].ID {
			edited = j
		}
	}
	if edited.Text != "Edited" || edited.Kind != domain.KindBlocker || edited.Date != date(t, "2026-11-02") {
		t.Errorf("edit: %+v", edited)
	}
	if rec := post(h, "/journal/"+id, form("kind", "scope_ack", "text", "x", "date", "2026-11-02")); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("edit into scope_ack: %d", rec.Code)
	}
	wantStatus(t, post(h, "/journal/"+id+"/delete", form()), http.StatusSeeOther)
	wantStatus(t, post(h, "/journal/"+id+"/delete", form()), http.StatusNotFound)
	// the activity, when named, must exist
	for name, f := range map[string]url.Values{
		"bad activity":     form("activity", "abc", "kind", "update", "text", "x"),
		"unknown activity": form("activity", "999", "kind", "update", "text", "x"),
	} {
		if rec := post(h, "/journal", f); rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("%s: %d", name, rec.Code)
		}
	}
	// the old per-activity route is gone
	wantStatus(t, post(h, "/activities/2/journal", form("kind", "update", "text", "x")), http.StatusNotFound)
	wantStatus(t, post(h, "/journal/999", form("kind", "update", "text", "x", "date", "2026-11-02")), http.StatusNotFound)
}

func TestScopeAckEntriesAreReadOnly(t *testing.T) {
	h, st := newApp(t)
	// an acknowledged entry, written the way Acknowledge writes it
	if _, err := st.AckScope(ctx, 1, domain.FixedClock{D: date(t, testToday)}); err != nil {
		t.Fatal(err)
	}
	list, _ := st.ListJournal(ctx, 1)
	var ack domain.JournalEntry
	for _, j := range list {
		if j.Kind == domain.KindScopeAck {
			ack = j
		}
	}
	id := strconv.FormatInt(ack.ID, 10)
	f := url.Values{"kind": {"update"}, "text": {"x"}, "date": {"2026-11-18"}}
	wantStatus(t, post(h, "/journal/"+id, f), http.StatusUnprocessableEntity)
	wantStatus(t, post(h, "/journal/"+id+"/delete", nil), http.StatusUnprocessableEntity)
	body := get(h, "/?sel=1").Body.String()
	if !strings.Contains(body, "Scope change acknowledged") || strings.Contains(body, `action="/journal/`+id+`"`) {
		t.Error("scope_ack entry should be listed without edit controls")
	}
}

func TestPanelJournalShowsLoggedTime(t *testing.T) {
	h, _ := newApp(t)
	wantStatus(t, post(h, "/journal", url.Values{"activity": {"2"}, "kind": {"update"}, "text": {"Fresh note"}}), http.StatusSeeOther)
	wantStatus(t, post(h, "/journal", url.Values{"activity": {"2"}, "kind": {"update"}, "text": {"Backfilled"}, "date": {"2026-11-12"}}), http.StatusSeeOther)
	rec := get(h, "/?sel=2")
	body := rec.Body.String()
	// new entries show the date and the time they were logged, even when backfilled
	wantBody(t, rec, `2026-11-18 14:05</span>`, `2026-11-12 14:05</span>`)
	// seeded entries have no time
	wantBody(t, rec, `<span class="muted mono">2026-11-03</span>`)
	// newest date first; within a date the later time first, entries without time last
	if !bodyOrder(body[strings.Index(body, `<section class="card" id="journal"`):], "Fresh note", "Backfilled", "Client upgrades pulled into this activity") {
		t.Error("journal order")
	}
}

func TestJournalTextIsMultiline(t *testing.T) {
	h, st := newApp(t)
	wantStatus(t, post(h, "/journal", url.Values{"activity": {"2"}, "kind": {"update"}, "text": {"First line\r\n\r\n- detail  \r\n\r\n"}}), http.StatusSeeOther)
	list, _ := st.ListJournal(ctx, 2)
	if list[0].Text != "First line\n\n- detail" {
		t.Fatalf("stored %q", list[0].Text)
	}
	id := strconv.FormatInt(list[0].ID, 10)
	wantBody(t, get(h, "/?sel=2"), "<span class=\"feed-text\">First line\n\n- detail</span>",
		"<textarea name=\"text\" rows=\"3\" required>First line\n\n- detail</textarea>")
	wantStatus(t, post(h, "/journal/"+id, url.Values{"kind": {"update"}, "text": {"one\r\ntwo"}, "date": {"2026-11-18"}}), http.StatusSeeOther)
	if list, _ := st.ListJournal(ctx, 2); list[0].Text != "one\ntwo" {
		t.Errorf("edited %q", list[0].Text)
	}
}

// Logging needs no activity: the entry is Unassigned until filed from its Edit
// form (SPEC §10 item 91).
func TestUnassignedEntries(t *testing.T) {
	h, st := newApp(t)
	wantStatus(t, post(h, "/journal", url.Values{"kind": {"task"}, "text": {"Loose task"}, "next": {"/"}}), http.StatusSeeOther)
	before := tileValue(t, get(h, "/").Body.String(), "Open tasks now")
	feed, _, _ := st.ListJournalFeed(ctx, nil, true, 10)
	if len(feed) != 1 || feed[0].ActivityID != 0 {
		t.Fatalf("unassigned: %+v", feed)
	}
	id := strconv.FormatInt(feed[0].ID, 10)

	// the feed shows it, with a filing select, unless a project or selection narrows it
	body := get(h, "/").Body.String()
	for _, want := range []string{`<span class="tag small unassigned">Unassigned</span>`, `<select name="activity"><option value="" selected>Unassigned</option><optgroup label=`, `<option value="2">Kafka upgrade readiness</option>`} {
		if !strings.Contains(body, want) {
			t.Errorf("feed lacks %q", want)
		}
	}
	for _, target := range []string{"/?project=1", "/?sel=2"} {
		if strings.Contains(get(h, target).Body.String(), "Loose task") {
			t.Errorf("%s shows the unassigned entry", target)
		}
	}
	// its open task counts in the tile until it is closed
	wantStatus(t, post(h, "/tasks/"+id+"/toggle", url.Values{"next": {"/"}}), http.StatusSeeOther)
	if after := tileValue(t, get(h, "/").Body.String(), "Open tasks now"); after != before-1 {
		t.Errorf("tile %d → %d", before, after)
	}

	// filing it moves it to the activity; an edit without the field keeps it there
	edit := func(activity ...string) *httptest.ResponseRecorder {
		f := url.Values{"kind": {"task"}, "text": {"Filed task"}, "date": {testToday}, "next": {"/"}}
		if len(activity) > 0 {
			f.Set("activity", activity[0])
		}
		return post(h, "/journal/"+id, f)
	}
	wantStatus(t, edit("2"), http.StatusSeeOther)
	wantStatus(t, edit(), http.StatusSeeOther)
	if ts, _ := st.ListTasks(ctx, 2); ts[len(ts)-1].Text != "Filed task" {
		t.Errorf("not filed: %+v", ts[len(ts)-1])
	}
	if strings.Contains(get(h, "/?sel=2").Body.String(), `<select name="activity">`) {
		t.Error("filed entries offer no filing select")
	}
	wantStatus(t, edit(""), http.StatusSeeOther)
	if feed, _, _ := st.ListJournalFeed(ctx, nil, true, 10); len(feed) != 1 {
		t.Errorf("not unassigned again: %+v", feed)
	}
	if rec := edit("999"); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("filed under a missing activity: %d", rec.Code)
	}
	wantBody(t, get(h, "/export/journal.md"), "## Unassigned\n\n- "+testToday+" 14:05 **Task** — Filed task (closed "+testToday+")")
}
