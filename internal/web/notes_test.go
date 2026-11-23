package web

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"workline/internal/domain"
)

// An entry can belong to a project instead of an activity, and `note` is a
// kind with its /n prefix (SPEC §10 item 120). Project 1 is "Order platform
// modernisation"; activity 1 is in it.
func TestProjectNotes(t *testing.T) {
	h, st := newApp(t)
	// the bar files to a project with a "p" value; /n makes it a note
	wantStatus(t, post(h, "/journal", url.Values{"activity": {"p1"}, "kind": {"update"}, "text": {"/n Vendor contract renews in March"}, "next": {"/"}}), http.StatusSeeOther)
	all, err := st.ListAllJournal(ctx)
	if err != nil {
		t.Fatal(err)
	}
	note := all[0]
	if note.ProjectID != 1 || note.ActivityID != 0 || note.Kind != domain.KindNote || note.Text != "Vendor contract renews in March" {
		t.Fatalf("logged: %+v", note)
	}

	// home: tagged with the project, linking to it on the roadmap, re-fileable
	home := get(h, "/")
	wantBody(t, home, `<span class="chip kind-note">Note</span>`,
		`<a class="tag small" href="/roadmap?project=1" hx-boost="false">Order platform modernisation</a>`,
		`<option value="p1" selected>Order platform modernisation (project)</option>`)

	// roadmap: shown for all projects and for its own, not for another or under a selection
	for target, want := range map[string]bool{"/roadmap": true, "/roadmap?project=1": true, "/roadmap?project=2": false, "/roadmap?sel=1": false} {
		if got := strings.Contains(get(h, target).Body.String(), "Vendor contract renews"); got != want {
			t.Errorf("%s shows the note: %v", target, got)
		}
	}
	// search: proj: matches it, is:unassigned does not
	if !strings.Contains(get(h, searchURL(`proj:"Order platform"`)).Body.String(), "Vendor contract renews") {
		t.Error("proj: misses the project's note")
	}
	if strings.Contains(get(h, searchURL("is:unassigned")).Body.String(), "Vendor contract renews") {
		t.Error("a project note is Unassigned")
	}
	// export: under its project, before the activities
	md := get(h, "/export/journal.md").Body.String()
	if !bodyOrderIn(md, "## Order platform modernisation\n\n- "+testToday, "**Note** — Vendor contract renews in March", "### Consolidate order services") {
		t.Errorf("export:\n%s", md)
	}
	// report: tagged with the project alone
	wantBody(t, get(h, "/report?only=starred"), "Highlights") // the page renders with project items around
	if err := st.ToggleStar(ctx, note.ID); err != nil {
		t.Fatal(err)
	}
	wantBody(t, get(h, "/report"), "Order platform modernisation: Vendor contract renews in March")

	// editing files it under an activity, and back to Unassigned
	edit := func(link string) {
		t.Helper()
		wantStatus(t, post(h, "/journal/"+strconv.FormatInt(note.ID, 10), url.Values{"activity": {link}, "kind": {"note"}, "text": {"Vendor contract renews in March"}, "date": {testToday}, "next": {"/"}}), http.StatusSeeOther)
	}
	edit("1")
	if es, _ := st.ListJournal(ctx, 1); es[0].ID != note.ID {
		t.Error("not filed under activity 1")
	}
	edit("")
	if all, _ := st.ListAllJournal(ctx); !all[0].Unassigned() {
		t.Error("not Unassigned again")
	}
	// bad links are refused
	for _, link := range []string{"p", "px", "p-1", "x"} {
		if rec := post(h, "/journal", url.Values{"activity": {link}, "kind": {"note"}, "text": {"x"}, "next": {"/"}}); rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("link %q: %d", link, rec.Code)
		}
	}
	if rec := post(h, "/journal", url.Values{"activity": {"p999"}, "kind": {"note"}, "text": {"x"}, "next": {"/"}}); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("missing project: %d", rec.Code)
	}
}

// A project's task is listed on the tasks page under its project.
func TestProjectTaskOnTasksPage(t *testing.T) {
	h, st := newApp(t)
	if _, err := st.AddJournal(ctx, domain.JournalEntry{ProjectID: 2, Date: date(t, testToday), Kind: domain.KindTask, Text: "Renew the CI licence"}); err != nil {
		t.Fatal(err)
	}
	wantBody(t, get(h, "/tasks?project=2"), "Renew the CI licence", `<a class="tag small" href="/roadmap?project=2" hx-boost="false">Platform and tooling</a>`)
	if strings.Contains(get(h, "/tasks?project=1").Body.String(), "Renew the CI licence") {
		t.Error("another project lists it")
	}
}
