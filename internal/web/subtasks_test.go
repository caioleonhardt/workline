package web

import (
	"net/http"
	"net/url"
	"strconv"
	"testing"

	"workline/internal/domain"
)

func TestSubtaskFlows(t *testing.T) {
	h, st := newApp(t)
	next := url.Values{"next": {"/?sel=2&status=done"}}
	form := func(kv ...string) url.Values {
		f := url.Values{"next": next["next"]}
		for i := 0; i < len(kv); i += 2 {
			f.Set(kv[i], kv[i+1])
		}
		return f
	}

	// add: default created_on is today, explicit one is honoured (future too)
	rec := post(h, "/activities/2/subtasks", form("text", "Write runbook"))
	wantStatus(t, rec, http.StatusSeeOther)
	if loc := rec.Header().Get("Location"); loc != "/?sel=2&status=done" {
		t.Errorf("Location = %q", loc)
	}
	wantStatus(t, post(h, "/activities/2/subtasks", form("text", "Later", "created_on", "2027-01-05")), http.StatusSeeOther)
	subs, _ := st.ListSubtasks(ctx, 2)
	if len(subs) != 8 || subs[6].CreatedOn != date(t, testToday) || subs[7].CreatedOn != date(t, "2027-01-05") {
		t.Fatalf("subtasks: %+v", subs[6:])
	}
	for name, f := range map[string]url.Values{
		"blank text": form("text", " "), "bad date": form("text", "x", "created_on", "soon"),
	} {
		if rec := post(h, "/activities/2/subtasks", f); rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("%s: %d", name, rec.Code)
		}
	}
	wantStatus(t, post(h, "/activities/999/subtasks", form("text", "x")), http.StatusUnprocessableEntity) // FK failure

	// toggle closes today, then reopens
	id := subs[6].ID
	target := "/subtasks/" + strconv.FormatInt(id, 10) + "/toggle"
	wantStatus(t, post(h, target, next), http.StatusSeeOther)
	if got, _ := st.ListSubtasks(ctx, 2); got[6].ClosedOn != date(t, testToday) {
		t.Errorf("close: %+v", got[6])
	}
	wantBody(t, get(h, "/?sel=2"), "closed "+testToday)
	wantStatus(t, post(h, target, next), http.StatusSeeOther)
	if got, _ := st.ListSubtasks(ctx, 2); !got[6].ClosedOn.IsZero() {
		t.Errorf("reopen: %+v", got[6])
	}
	wantStatus(t, post(h, "/subtasks/999/toggle", next), http.StatusNotFound)

	// edit changes text and created_on; a posted closed_on is ignored
	wantStatus(t, post(h, "/subtasks/"+strconv.FormatInt(id, 10), form("text", "Runbook v2", "created_on", "2026-11-10", "closed_on", "2026-11-12")), http.StatusSeeOther)
	got, _ := st.ListSubtasks(ctx, 2)
	if got[6].Text != "Runbook v2" || got[6].CreatedOn != date(t, "2026-11-10") || !got[6].ClosedOn.IsZero() {
		t.Errorf("edit: %+v", got[6])
	}
	if rec := post(h, "/subtasks/"+strconv.FormatInt(id, 10), form("text", "", "created_on", "2026-11-10")); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("blank edit: %d", rec.Code)
	}
	// moving created_on after an existing closed date is refused
	closed := subs[0].ID // seeded and closed
	if rec := post(h, "/subtasks/"+strconv.FormatInt(closed, 10), form("text", "x", "created_on", "2026-11-30")); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("created after closed: %d", rec.Code)
	}

	// delete, including the last remaining subtask (SPEC §10 item 23)
	for _, s := range got {
		wantStatus(t, post(h, "/subtasks/"+strconv.FormatInt(s.ID, 10)+"/delete", next), http.StatusSeeOther)
	}
	if left, _ := st.ListSubtasks(ctx, 2); len(left) != 0 {
		t.Errorf("left: %+v", left)
	}
	wantStatus(t, post(h, "/subtasks/"+strconv.FormatInt(id, 10)+"/delete", next), http.StatusNotFound)
	if a, _ := st.GetActivity(ctx, 2); a.Status != domain.StatusBlocked {
		t.Errorf("activity changed: %+v", a)
	}
}
