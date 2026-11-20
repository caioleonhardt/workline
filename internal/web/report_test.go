package web

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"workline/internal/domain"
)

func TestReportMarkdown(t *testing.T) {
	got := reportMarkdown("1:1 update · 2026-11-12 to 2026-11-18", []reportSection{
		{"Highlights", []reportLine{{Text: "2026-11-17 · Decision · Atlas / Migration: Cut over on Dec 2\nafter the freeze", StarID: 9}}},
		{"Done", []reportLine{{Text: "Atlas / Migration", Sub: []string{"Write runbook", "Dry run"}}, {Text: "Finished: Atlas / Old API"}}},
	})
	want := `# 1:1 update · 2026-11-12 to 2026-11-18

## Highlights

- 2026-11-17 · Decision · Atlas / Migration: Cut over on Dec 2
  after the freeze

## Done

- Atlas / Migration
  - Write runbook
  - Dry run
- Finished: Atlas / Old API
`
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
	if got := reportMarkdown("x", nil); !strings.Contains(got, "Nothing to report in this range.") {
		t.Errorf("empty: %q", got)
	}
}

func TestReportPage(t *testing.T) {
	h, st := newApp(t)
	// a decision today, starred; a task opened and closed today
	dec, err := st.AddJournal(ctx, domain.JournalEntry{ActivityID: 1, Date: date(t, testToday), Kind: domain.KindDecision, Text: "Cut over on Dec 2"})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.ToggleStar(ctx, dec.ID); err != nil {
		t.Fatal(err)
	}
	wantStatus(t, post(h, "/journal", url.Values{"activity": {"1"}, "kind": {"task"}, "text": {"Write runbook"}, "next": {"/"}}), http.StatusSeeOther)

	rec := get(h, "/report")
	wantStatus(t, rec, http.StatusOK)
	wantBody(t, rec, `<div id="dash" class="report-page"`, `value="2026-11-12"`, `value="2026-11-18"`,
		"<h1>1:1 update · 2026-11-12 to 2026-11-18</h1>", "<h2>Highlights</h2>", "<h2>New</h2>", "<h2>Decisions</h2>", "<h2>Progress</h2>",
		"Cut over on Dec 2", "Write runbook", `action="/journal/`+strconv.FormatInt(dec.ID, 10)+`/star"`,
		`href="/report.md?from=2026-11-12&amp;to=2026-11-18"`, `id="report-md"`)
	if rec.Header().Get("Vary") != "HX-Request" {
		t.Error("no Vary")
	}

	rec = hxGet(h, "/report?only=starred")
	wantStatus(t, rec, http.StatusOK)
	body := rec.Body.String()
	if strings.Contains(body, "<html") || !strings.Contains(body, "<h2>Highlights</h2>") || strings.Contains(body, "<h2>Decisions</h2>") {
		t.Errorf("highlights-only fragment:\n%s", body)
	}

	// unstarring from the report answers with the report
	rec = hxPost(h, "/journal/"+strconv.FormatInt(dec.ID, 10)+"/star", url.Values{"next": {"/report?only=starred"}})
	wantStatus(t, rec, http.StatusOK)
	wantBody(t, rec, `class="report-page"`, "Nothing to report in this range.")

	rec = get(h, "/report.md?from=2026-11-18&to=2026-11-18")
	wantStatus(t, rec, http.StatusOK)
	if cd := rec.Header().Get("Content-Disposition"); cd != `attachment; filename="workline-1on1-2026-11-18.md"` {
		t.Errorf("Content-Disposition = %q", cd)
	}
	if !strings.HasPrefix(rec.Body.String(), "# 1:1 update · 2026-11-18 to 2026-11-18\n") || !strings.Contains(rec.Body.String(), "\n## Decisions\n\n- 2026-11-18 · ") {
		t.Errorf("markdown:\n%s", rec.Body)
	}

	// a project narrows it and names it
	rec = get(h, "/report.md?project=1")
	wantStatus(t, rec, http.StatusOK)
	if first, _, _ := strings.Cut(rec.Body.String(), "\n"); !strings.HasPrefix(first, "# 1:1 update · 2026-11-12 to 2026-11-18 · ") {
		t.Errorf("heading %q", first)
	}

	for target, status := range map[string]int{
		"/report?from=nope":                       http.StatusUnprocessableEntity,
		"/report?from=2026-11-19&to=2026-11-18":   http.StatusUnprocessableEntity,
		"/report.md?project=x":                    http.StatusUnprocessableEntity,
		"/report?project=999":                     http.StatusNotFound,
		"/report.md?from=2026-11-19&to=2026-11-1": http.StatusUnprocessableEntity,
	} {
		if rec := get(h, target); rec.Code != status {
			t.Errorf("%s: %d, want %d", target, rec.Code, status)
		}
	}
	wantBody(t, get(h, "/"), `href="/report?from=2026-11-09&amp;to=2026-11-18"`)
}
