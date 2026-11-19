package web

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"workline/internal/domain"
)

func newForm(over url.Values) url.Values {
	f := url.Values{
		"next": {"/timeline?new=1&status=active&status=blocked"}, "project": {"1"}, "name": {"Pilot"},
		"requested_by": {"Manager"}, "priority": {"high"}, "start": {"2026-11-01"}, "due": {"2026-12-01"},
		"tasks": {"first\n\n  second  \n"},
	}
	for k, v := range over {
		f[k] = v
	}
	return f
}

func TestNewActivityFormShownOnlyWithNew(t *testing.T) {
	h, _ := newApp(t)
	wantBody(t, get(h, "/timeline?new=1"), `action="/activities"`, "+ New project…", "First tasks")
	rec := get(h, "/timeline")
	wantStatus(t, rec, http.StatusOK)
	if body := rec.Body.String(); strings.Contains(body, `action="/activities"`) {
		t.Error("form shown without new=1")
	}
	wantBody(t, rec, `href="/timeline?new=1"`)
}

func TestCreateActivity(t *testing.T) {
	h, st := newApp(t)
	rec := post(h, "/activities", newForm(nil))
	wantStatus(t, rec, http.StatusSeeOther)
	loc, _ := url.Parse(rec.Header().Get("Location"))
	q := loc.Query()
	if q.Get("sel") == "" || q.Has("new") || len(q["status"]) != 2 {
		t.Errorf("Location = %s", loc)
	}
	id, _ := strconv.ParseInt(q.Get("sel"), 10, 64)
	a, err := st.GetActivity(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if a.Name != "Pilot" || a.Priority != domain.PriorityHigh || a.Status != domain.StatusActive || a.ProjectID != 1 {
		t.Errorf("activity: %+v", a)
	}
	subs, _ := st.ListTasks(ctx, a.ID)
	if len(subs) != 2 || subs[1].Text != "second" || subs[0].CreatedOn != date(t, "2026-11-01") {
		t.Errorf("tasks: %+v", subs)
	}
	if j, _ := st.ListJournal(ctx, a.ID); len(j) != 3 || j[0].Text != "Activity created" { // dated today, after the tasks' start date
		t.Errorf("journal: %+v", j)
	}
}

func TestCreateActivityWithNewProject(t *testing.T) {
	h, st := newApp(t)
	wantStatus(t, post(h, "/activities", newForm(url.Values{"project": {"new"}, "new_project": {"Fresh"}})), http.StatusSeeOther)
	ps, _ := st.ListProjects(ctx, true)
	if len(ps) != 5 {
		t.Errorf("projects: %+v", ps)
	}
	// a future start makes it planned
	wantStatus(t, post(h, "/activities", newForm(url.Values{"name": {"Later"}, "start": {"2026-12-01"}, "due": {"2026-12-31"}})), http.StatusSeeOther)
	acts, _ := st.ListActivities(ctx)
	for _, a := range acts {
		if a.Name == "Later" && a.Status != domain.StatusPlanned {
			t.Errorf("status: %+v", a)
		}
	}
}

func TestCreateActivityRejections(t *testing.T) {
	h, st := newApp(t)
	for name, over := range map[string]url.Values{
		"no tasks":         {"tasks": {" \n \n"}},
		"blank name":       {"name": {" "}},
		"due before start": {"due": {"2026-10-01"}},
		"bad date":         {"start": {"11/01/2026"}},
		"no project":       {"project": {""}},
		"missing project":  {"project": {"999"}},
		"blank new name":   {"project": {"new"}, "new_project": {" "}},
		"duplicate name":   {"project": {"new"}, "new_project": {"team and people"}},
		"bad priority":     {"priority": {"urgent"}},
	} {
		if rec := post(h, "/activities", newForm(over)); rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("%s: status %d", name, rec.Code)
		}
	}
	if acts, _ := st.ListActivities(ctx); len(acts) != 9 {
		t.Errorf("rejected forms wrote %d activities", len(acts)-9)
	}
	if ps, _ := st.ListProjects(ctx, true); len(ps) != 4 {
		t.Errorf("rejected forms wrote projects: %+v", ps)
	}
}

func TestPanelShownForSel(t *testing.T) {
	h, _ := newApp(t)
	rec := get(h, "/timeline?sel=2")
	wantStatus(t, rec, http.StatusOK)
	wantBody(t, rec, `id="panel"`, "Kafka upgrade readiness", "2 of 6 tasks done", "33%", `aria-current="true"`)
	wantBody(t, get(h, "/timeline"), `<aside class="detail" aria-label="Activity detail">`, "Select an activity to see its burndown, scope, tasks and journal.")
	for _, target := range []string{"/timeline?sel=999", "/timeline?sel=abc", "/timeline?sel="} {
		if body := get(h, target).Body.String(); strings.Contains(body, `id="panel"`) {
			t.Errorf("%s shows a panel", target)
		}
	}
}

func TestUpdateActivityFields(t *testing.T) {
	h, st := newApp(t)
	f := url.Values{
		"next": {"/timeline?sel=2&status=blocked&status=done"}, "project": {"3"}, "name": {"Kafka 4 readiness"},
		"requested_by": {"CTO"}, "priority": {"low"}, "start": {"2026-09-01"}, "due": {"2026-12-15"},
	}
	rec := post(h, "/activities/2", f)
	wantStatus(t, rec, http.StatusSeeOther)
	if loc := rec.Header().Get("Location"); loc != "/timeline?sel=2&status=blocked&status=done" {
		t.Errorf("Location = %q", loc)
	}
	a, _ := st.GetActivity(ctx, 2)
	if a.ProjectID != 3 || a.Name != "Kafka 4 readiness" || a.Priority != domain.PriorityLow || a.Status != domain.StatusBlocked || a.Due != date(t, "2026-12-15") {
		t.Errorf("activity: %+v", a)
	}
	for name, over := range map[string]string{"due": "2026-08-01", "start": "x", "project": "999", "priority": "urgent"} {
		bad := url.Values{}
		for k, v := range f {
			bad[k] = v
		}
		bad.Set(name, over)
		if rec := post(h, "/activities/2", bad); rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("bad %s: status %d", name, rec.Code)
		}
	}
	wantStatus(t, post(h, "/activities/999", f), http.StatusNotFound)
}

func TestUpdateActivityStatusSetsDoneDate(t *testing.T) {
	h, st := newApp(t)
	wantStatus(t, post(h, "/activities/2", url.Values{"status": {"done"}}), http.StatusSeeOther)
	if a, _ := st.GetActivity(ctx, 2); a.Status != domain.StatusDone || a.DoneOn != date(t, testToday) {
		t.Errorf("done: %+v", a)
	}
	// a done activity with open tasks shows its task ratio
	wantBody(t, get(h, "/timeline?sel=2"), "33%", "2 of 6 tasks done")
	wantStatus(t, post(h, "/activities/2", url.Values{"status": {"active"}}), http.StatusSeeOther)
	if a, _ := st.GetActivity(ctx, 2); !a.DoneOn.IsZero() {
		t.Errorf("reopened: %+v", a)
	}
	wantStatus(t, post(h, "/activities/2", url.Values{"status": {"weird"}}), http.StatusUnprocessableEntity)
}

func TestDeleteActivity(t *testing.T) {
	h, st := newApp(t)
	rec := post(h, "/activities/2/delete", url.Values{"next": {"/timeline?sel=2&new=1"}})
	wantStatus(t, rec, http.StatusSeeOther)
	if loc := rec.Header().Get("Location"); loc != "/timeline?new=1" {
		t.Errorf("Location = %q", loc)
	}
	if _, err := st.GetActivity(ctx, 2); err == nil {
		t.Error("still there")
	}
	if subs, _ := st.ListTasks(ctx, 2); len(subs) != 0 {
		t.Error("tasks survived")
	}
	wantStatus(t, post(h, "/activities/2/delete", nil), http.StatusNotFound)
}

func TestRejectedFormShowsOnlyTheMessage(t *testing.T) {
	h, _ := newApp(t)
	rec := post(h, "/activities", newForm(url.Values{"start": {"11/01/2026"}}))
	wantStatus(t, rec, http.StatusUnprocessableEntity)
	wantBody(t, rec, `Start must be a date (YYYY-MM-DD), not &#34;11/01/2026&#34;.`)
	if strings.Contains(rec.Body.String(), "create activity") || strings.Contains(rec.Body.String(), "invalid value") {
		t.Error("error page shows wrapping context")
	}
}
