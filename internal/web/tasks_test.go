package web

import (
	"net/http"
	"net/url"
	"strconv"
	"testing"

	"workline/internal/domain"
)

// Tasks are journal entries of kind task: added from the quick-add bar, edited
// and deleted like any entry, and toggled from the panel.
func TestTaskFlows(t *testing.T) {
	h, st := newApp(t)
	next := url.Values{"next": {"/roadmap?sel=2&status=done"}}
	form := func(kv ...string) url.Values {
		f := url.Values{"next": next["next"]}
		for i := 0; i < len(kv); i += 2 {
			f.Set(kv[i], kv[i+1])
		}
		return f
	}

	// add: dated today, stamped like any entry
	rec := post(h, "/journal", form("activity", "2", "kind", "task", "text", "Write runbook"))
	wantStatus(t, rec, http.StatusSeeOther)
	if loc := rec.Header().Get("Location"); loc != "/roadmap?sel=2&status=done" {
		t.Errorf("Location = %q", loc)
	}
	subs, _ := st.ListTasks(ctx, 2)
	if len(subs) != 7 || subs[6].Text != "Write runbook" || subs[6].CreatedOn != date(t, testToday) {
		t.Fatalf("tasks: %+v", subs[6:])
	}
	wantBody(t, get(h, "/roadmap?sel=2"), `<span class="chip kind-task">Task</span>`)

	// toggle closes today, then reopens; only tasks toggle
	id := strconv.FormatInt(subs[6].ID, 10)
	target := "/tasks/" + id + "/toggle"
	wantStatus(t, post(h, target, next), http.StatusSeeOther)
	if got, _ := st.ListTasks(ctx, 2); got[6].ClosedOn() != date(t, testToday) {
		t.Errorf("close: %+v", got[6])
	}
	wantBody(t, get(h, "/roadmap?sel=2"), "closed "+testToday, `class="feed-text done-text">Write runbook</span>`)
	wantStatus(t, post(h, target, next), http.StatusSeeOther)
	if got, _ := st.ListTasks(ctx, 2); !got[6].ClosedOn().IsZero() {
		t.Errorf("reopen: %+v", got[6])
	}
	wantStatus(t, post(h, "/tasks/999/toggle", next), http.StatusNotFound)
	notes, _ := st.ListJournal(ctx, 2)
	for _, j := range notes {
		if j.Kind == domain.KindUpdate {
			wantStatus(t, post(h, "/tasks/"+strconv.FormatInt(j.ID, 10)+"/toggle", next), http.StatusNotFound)
			break
		}
	}

	// edit changes text and opened date; the kind stays task
	wantStatus(t, post(h, "/journal/"+id, form("kind", "task", "text", "Runbook v2", "date", "2026-11-10")), http.StatusSeeOther)
	got, _ := st.ListTasks(ctx, 2)
	if got[6].Text != "Runbook v2" || got[6].CreatedOn != date(t, "2026-11-10") || !got[6].ClosedOn().IsZero() {
		t.Errorf("edit: %+v", got[6])
	}
	if rec := post(h, "/journal/"+id, form("kind", "update", "text", "x", "date", "2026-11-10")); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("task turned into an update: %d", rec.Code)
	}
	// moving the opened date after an existing closed date is refused
	closed := strconv.FormatInt(subs[0].ID, 10) // seeded and closed
	if rec := post(h, "/journal/"+closed, form("kind", "task", "text", "x", "date", "2026-11-30")); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("opened after closed: %d", rec.Code)
	}

	// delete, including the last remaining task (SPEC §10 item 23)
	for _, s := range got {
		wantStatus(t, post(h, "/journal/"+strconv.FormatInt(s.ID, 10)+"/delete", next), http.StatusSeeOther)
	}
	if left, _ := st.ListTasks(ctx, 2); len(left) != 0 {
		t.Errorf("left: %+v", left)
	}
	if a, _ := st.GetActivity(ctx, 2); a.Status != domain.StatusBlocked {
		t.Errorf("activity changed: %+v", a)
	}
}
