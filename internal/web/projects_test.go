package web

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestProjectsPage(t *testing.T) {
	h, _ := newApp(t)
	rec := get(h, "/projects")
	wantStatus(t, rec, http.StatusOK)
	wantBody(t, rec, "Team and people", "Confirm delete", `name="next" value="/projects"`)
}

func TestProjectRenameArchiveDelete(t *testing.T) {
	h, st := newApp(t)
	f := url.Values{"name": {"Renamed"}, "next": {"/projects?x=1"}}
	rec := post(h, "/projects/2", f)
	wantStatus(t, rec, http.StatusSeeOther)
	if loc := rec.Header().Get("Location"); loc != "/projects?x=1" {
		t.Errorf("Location = %q", loc)
	}
	if p, _ := st.GetProject(ctx, 2); p.Name != "Renamed" {
		t.Errorf("rename: %+v", p)
	}

	wantStatus(t, post(h, "/projects/2", url.Values{"name": {"team AND people"}}), http.StatusUnprocessableEntity)
	wantStatus(t, post(h, "/projects/2", url.Values{"name": {"x"}}), http.StatusSeeOther)

	wantStatus(t, post(h, "/projects/2", url.Values{"archived": {"1"}}), http.StatusSeeOther)
	if p, _ := st.GetProject(ctx, 2); !p.Archived {
		t.Error("not archived")
	}
	if body := get(h, "/?status=done").Body.String(); strings.Contains(body, "Observability vendor review") {
		t.Error("archived project's activities are still on the dashboard")
	}
	wantStatus(t, post(h, "/projects/2", url.Values{"archived": {"0"}}), http.StatusSeeOther)
	wantBody(t, get(h, "/?status=done"), "Observability vendor review")

	wantStatus(t, post(h, "/projects/2/delete", url.Values{"next": {"/projects"}}), http.StatusSeeOther)
	if body := get(h, "/?status=done").Body.String(); strings.Contains(body, "Observability vendor review") {
		t.Error("deleted project's activities are still listed")
	}
	for _, target := range []string{"/projects/99", "/projects/99/delete", "/projects/abc", "/projects/0/delete"} {
		wantStatus(t, post(h, target, url.Values{"name": {"x"}}), http.StatusNotFound)
	}
}
