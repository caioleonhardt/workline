package web

import (
	"net/http"
	"strings"
	"testing"

	"workline/internal/domain"
)

func TestExportMarkdownFormat(t *testing.T) {
	act := exportActivity{
		Activity: domain.Activity{Name: "Plan", Status: domain.StatusActive, RequestedBy: "Manager", Start: date(t, "2026-03-02"), Due: date(t, "2026-12-18")},
		Entries: []domain.JournalEntry{
			{Date: date(t, "2026-03-02"), Kind: domain.KindMeeting, Text: "Kick-off"},
			{Date: date(t, "2026-04-20"), Kind: domain.KindDecision, Text: "first line\nsecond line\n", LoggedAt: "2026-04-20 14:05"},
			{Date: date(t, "2026-11-18"), Kind: domain.KindScopeAck, Text: "Scope change acknowledged", LoggedAt: "2026-11-18 16:02"},
		},
	}
	got := exportMarkdown("Alpha", date(t, "2026-01-01"), date(t, "2026-11-19"), date(t, "2026-11-19"),
		[]exportProject{{Name: "Alpha", Activities: []exportActivity{act}}})
	want := "# Workline journal\n" +
		"2026-01-01 to 2026-11-19 · Alpha · generated 2026-11-19\n" +
		"\n## Alpha\n" +
		"\n### Plan\nActive · requested by Manager · 2026-03-02 → 2026-12-18\n\n" +
		"- 2026-03-02 **Meeting** — Kick-off\n" +
		"- 2026-04-20 14:05 **Decision** — first line\n  second line\n" +
		"- 2026-11-18 16:02 **Scope acknowledged** — Scope change acknowledged\n"
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}

	empty := exportMarkdown("all projects", 0, 0, date(t, "2026-11-19"), nil)
	if empty != "# Workline journal\nall dates · all projects · generated 2026-11-19\n\nNo journal entries in this range.\n" {
		t.Errorf("empty: %q", empty)
	}
	for _, c := range []struct {
		from, to domain.Date
		want     string
	}{{date(t, "2026-01-01"), 0, "from 2026-01-01"}, {0, date(t, "2026-02-01"), "until 2026-02-01"}} {
		if got := exportMarkdown("x", c.from, c.to, 1+date(t, "2026-01-01"), nil); !strings.Contains(got, c.want+" · x") {
			t.Errorf("range label: %q", got)
		}
	}
}

func TestExportJournalEndpoint(t *testing.T) {
	h, st := newApp(t)
	rec := get(h, "/export/journal.md")
	wantStatus(t, rec, http.StatusOK)
	if got := rec.Header().Get("Content-Type"); got != "text/markdown; charset=utf-8" {
		t.Errorf("Content-Type = %q", got)
	}
	if got := rec.Header().Get("Content-Disposition"); got != `attachment; filename="workline-journal-2026-11-18.md"` {
		t.Errorf("Content-Disposition = %q", got)
	}
	body := rec.Body.String()
	wantBody(t, rec, "# Workline journal\nall dates · all projects · generated 2026-11-18\n",
		"## Order platform modernisation", "### Consolidate order services into one Go service",
		"Active · requested by Manager · 2026-03-02 → 2026-12-18",
		"- 2026-03-02 **Meeting** — Kick-off with manager, scope agreed")
	// projects by name, entries oldest first
	if !bodyOrderIn(body, "## Operations and planning", "## Order platform modernisation", "## Platform and tooling", "## Team and people") {
		t.Error("projects out of order")
	}
	if !bodyOrderIn(body, "Kick-off with manager", "Strangler-fig over big-bang", "Pricing migrated", "Kafka dependencies removed") {
		t.Error("entries not oldest first")
	}

	// project and date range
	rec = get(h, "/export/journal.md?project=3&from=2026-09-01&to=2026-10-31")
	body = rec.Body.String()
	wantBody(t, rec, "2026-09-01 to 2026-10-31 · Team and people ·", "Rubric favours system design", "Calibration with interview panel")
	for _, no := range []string{"Order platform", "Headcount approved", "Plan agreed with both"} {
		if strings.Contains(body, no) {
			t.Errorf("unexpected %q in filtered export", no)
		}
	}

	// archived projects are left out unless named
	if err := st.SetProjectArchived(ctx, 3, true); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(get(h, "/export/journal.md").Body.String(), "Team and people") {
		t.Error("archived project exported without being named")
	}
	wantBody(t, get(h, "/export/journal.md?project=3"), "## Team and people")

	// errors
	wantStatus(t, get(h, "/export/journal.md?from=yesterday"), http.StatusUnprocessableEntity)
	wantStatus(t, get(h, "/export/journal.md?to=2026-13-01"), http.StatusUnprocessableEntity)
	wantStatus(t, get(h, "/export/journal.md?project=abc"), http.StatusUnprocessableEntity)
	wantStatus(t, get(h, "/export/journal.md?project=999"), http.StatusNotFound)
	// nothing in range
	wantBody(t, get(h, "/export/journal.md?from=2030-01-01"), "No journal entries in this range.")
}

func TestExportLinkFollowsTheDashboard(t *testing.T) {
	h, _ := newApp(t)
	for target, want := range map[string]string{
		"/":                               `href="/export/journal.md?from=2026-01-01&amp;to=2026-12-31" hx-boost="false"`,
		"/?period=q4":                     `href="/export/journal.md?from=2026-10-01&amp;to=2026-12-31" hx-boost="false"`,
		"/?period=q4&month=11&project=3":  `href="/export/journal.md?from=2026-11-01&amp;project=3&amp;to=2026-11-30" hx-boost="false"`,
		"/?status=done&creep=1&year=2025": `href="/export/journal.md?from=2025-01-01&amp;to=2025-12-31" hx-boost="false"`,
	} {
		wantBody(t, get(h, target), want, ">Export journal</a>")
	}
}
