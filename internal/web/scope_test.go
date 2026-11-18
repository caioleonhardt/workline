package web

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"workline/internal/domain"
)

func TestScopeSectionAndAck(t *testing.T) {
	h, st := newApp(t)
	// seeded activity 1 grew 4 -> 8 since kickoff
	rec := get(h, "/?sel=1")
	wantBody(t, rec, `<span class="small muted">At kickoff</span><strong class="mono">4</strong>`, `8 <span class="small muted">+100%</span>`, "Grew from 4 to 8 subtasks since kickoff (&#43;100%).",
		"Acknowledge and log to journal")

	rec = post(h, "/activities/1/scope-ack", url.Values{"next": {"/?sel=1&status=active"}})
	wantStatus(t, rec, http.StatusSeeOther)
	if loc := rec.Header().Get("Location"); loc != "/?sel=1&status=active" {
		t.Errorf("Location = %q", loc)
	}
	j, _ := st.ListJournal(ctx, 1)
	if j[0].Kind != domain.KindScopeAck || !strings.HasPrefix(j[0].Text, "Scope change acknowledged: 4 → 8 subtasks (+100%)") {
		t.Errorf("journal: %+v", j[0])
	}
	rec = get(h, "/?sel=1")
	wantBody(t, rec, `<span class="chip kind-scope_ack">Scope acknowledged</span>`)
	wantBody(t, rec, "Baseline (ack "+testToday+")</span><strong class=\"mono\">8</strong>", "acknowledged on "+testToday)
	if strings.Contains(rec.Body.String(), "Acknowledge and log to journal") {
		t.Error("button still shown after acknowledging")
	}

	// not flagged any more: refused, nothing written
	n := len(j)
	wantStatus(t, post(h, "/activities/1/scope-ack", nil), http.StatusUnprocessableEntity)
	if j, _ := st.ListJournal(ctx, 1); len(j) != n {
		t.Error("refused ack wrote a journal entry")
	}
	wantStatus(t, post(h, "/activities/999/scope-ack", nil), http.StatusNotFound)
}
