package web

import (
	"net/http"
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

	rec := post(h, "/activities/2/journal", form("kind", "decision", "text", "Ship it"))
	wantStatus(t, rec, http.StatusSeeOther)
	if loc := rec.Header().Get("Location"); loc != next {
		t.Errorf("Location = %q", loc)
	}
	wantStatus(t, post(h, "/activities/2/journal", form("kind", "meeting", "text", "Sync", "date", "2026-11-01")), http.StatusSeeOther)
	list, _ := st.ListJournal(ctx, 2)
	if list[0].Text != "Ship it" || list[0].Date != date(t, testToday) || list[0].Kind != domain.KindDecision {
		t.Errorf("newest first, default date today: %+v", list[0])
	}
	wantBody(t, get(h, "/?sel=2"), "Ship it", "Sync", "kind-decision")

	for name, f := range map[string]url.Values{
		"scope_ack kind": form("kind", "scope_ack", "text", "x"),
		"blank text":     form("kind", "update", "text", " "),
		"bad kind":       form("kind", "gossip", "text", "x"),
		"bad date":       form("kind", "update", "text", "x", "date", "yesterday"),
	} {
		if rec := post(h, "/activities/2/journal", f); rec.Code != http.StatusUnprocessableEntity {
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
	wantStatus(t, post(h, "/journal/999", form("kind", "update", "text", "x", "date", "2026-11-02")), http.StatusNotFound)
}

func TestScopeAckEntriesAreReadOnly(t *testing.T) {
	h, st := newApp(t)
	// an acknowledged entry, written the way Acknowledge writes it
	if _, err := st.AckScope(ctx, 1, date(t, testToday)); err != nil {
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
