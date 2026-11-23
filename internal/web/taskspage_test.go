package web

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"workline/internal/domain"
)

// /tasks lists every task of live projects, paginated, open ones by priority
// by default (SPEC §10 item 123). The seed has 17 open tasks.
func TestTasksPage(t *testing.T) {
	h, st := newApp(t)
	for i := range tasksPerPage {
		if _, err := st.AddJournal(ctx, domain.JournalEntry{Date: date(t, "2026-11-17"), Kind: domain.KindTask, Text: fmt.Sprintf("Filler %02d", i)}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := st.AddJournal(ctx, domain.JournalEntry{Date: date(t, "2026-11-17"), Kind: domain.KindTask, Text: "Loose urgent task", Priority: domain.PriorityHigh}); err != nil {
		t.Fatal(err)
	}
	rec := get(h, "/tasks")
	wantStatus(t, rec, http.StatusOK)
	wantBody(t, rec, "68 tasks · page 1 of 2", `<a href="/tasks?page=2" aria-label="Next page">`, `<span class="tag small unassigned">Unassigned</span>`)
	body := rec.Body.String()
	if !bodyOrderIn(body, "Loose urgent task", "Filler 00") || strings.Contains(body, "Remove Kafka topic dependencies") || strings.Contains(body, "Previous page") {
		t.Error("page 1 is not by priority, holds closed tasks, or offers a previous page")
	}
	if n := strings.Count(body, "data-entry"); n != tasksPerPage {
		t.Errorf("page 1 has %d tasks, want %d", n, tasksPerPage)
	}
	rec = get(h, "/tasks?page=2")
	wantBody(t, rec, "page 2 of 2", `<a href="/tasks" aria-label="Previous page">`)
	if n := strings.Count(rec.Body.String(), "data-entry"); n != 18 {
		t.Errorf("page 2 has %d tasks, want 18", n)
	}
	// a bad page is page 1, one past the end the last page
	wantBody(t, get(h, "/tasks?page=x"), "page 1 of 2")
	wantBody(t, get(h, "/tasks?page=9"), "page 2 of 2")

	// closed tasks, newest close first, checked
	rec = get(h, "/tasks?state=closed")
	wantBody(t, rec, "Remove Kafka topic dependencies", `<input type="checkbox" checked>`, "Reopen")
	if strings.Contains(rec.Body.String(), "Filler") {
		t.Error("closed tasks list open ones")
	}
	// a project narrows it and leaves Unassigned out
	rec = get(h, "/tasks?project=1&state=all")
	if body := rec.Body.String(); strings.Contains(body, "Unassigned</span>") || strings.Contains(body, "Filler") {
		t.Error("a project filter keeps Unassigned tasks")
	}

	// htmx gets the fragment; toggling from the page answers with it too
	rec = hxGet(h, "/tasks?page=2")
	if body := rec.Body.String(); strings.Contains(body, "<html") || !strings.Contains(body, `id="dash" class="tasks-page"`) {
		t.Error("HX-Request does not get the tasks fragment")
	}
	rec = hxPost(h, "/tasks/1/toggle", url.Values{"next": {"/tasks?state=all"}})
	wantStatus(t, rec, http.StatusOK)
	wantBody(t, rec, `class="tasks-page"`)
}
