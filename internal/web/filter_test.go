package web

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"workline/internal/domain"
)

func TestParseFilterCanonicalURL(t *testing.T) {
	today := date(t, testToday)
	for query, want := range map[string]string{
		"":                          "/",
		"year=2026":                 "/",
		"year=2025":                 "/?year=2025",
		"year=1999":                 "/",
		"year=abc":                  "/",
		"period=q4&month=11":        "/?month=11&period=q4",
		"period=q4&month=2":         "/?period=q4",
		"period=q9":                 "/",
		"month=13":                  "/",
		"month=2":                   "/?month=2",
		"scale=month":               "/?scale=month",
		"period=q1&scale=month":     "/?period=q1",
		"project=3&creep=1":         "/?creep=1&project=3",
		"creep=0":                   "/",
		"sel=7&new=1":               "/?new=1&sel=7",
		"sel=abc&new=0":             "/",
		"status=done&status=active": "/?status=active&status=done",
		"status=active&status=blocked&status=planned": "/",
		"status=":      "/?status=",
		"status=bogus": "/?status=",
	} {
		q, _ := url.ParseQuery(query)
		if got := parseFilter(q, today).URL(); got != want {
			t.Errorf("parseFilter(%q).URL() = %q, want %q", query, got, want)
		}
	}
}

// bodyOrder reports whether the names occur in the page's sections in the given order.
func bodyOrder(body string, names ...string) bool {
	return bodyOrderIn(body[strings.Index(body, "<section"):], names...) // skip the controls, which list project names
}

func TestDashboardHidesDoneByDefault(t *testing.T) {
	h, _ := newApp(t)
	body := get(h, "/").Body.String()
	for _, name := range []string{"Kafka upgrade readiness", "Hire senior ML engineer", "2027 capacity"} {
		if !strings.Contains(body, name) {
			t.Errorf("%s missing", name)
		}
	}
	for _, name := range []string{"Observability vendor review", "Postmortem", "Move build logic"} {
		if strings.Contains(body, name) {
			t.Errorf("done activity %s shown by default", name)
		}
	}
	wantBody(t, get(h, "/?status=done"), "Observability vendor review")
	if strings.Contains(get(h, "/?status=").Body.String(), "Kafka") {
		t.Error("status= shows activities")
	}
}

func TestDashboardOrdering(t *testing.T) {
	h, st := newApp(t)
	for id, p := range map[int64]domain.Priority{5: domain.PriorityLow, 6: domain.PriorityHigh} {
		a, _ := st.GetActivity(ctx, id)
		a.Priority = p
		if err := st.UpdateActivity(ctx, a); err != nil {
			t.Fatal(err)
		}
	}
	if err := st.SetActivityStatus(ctx, 7, domain.StatusDone, date(t, testToday)); err != nil {
		t.Fatal(err)
	}
	body := get(h, "/?status=active&status=done").Body.String()
	// within the project: priority, and done activities last
	if !bodyOrder(body, "Mentoring plan", "Hire senior ML", "Team onboarding guide") {
		t.Error("activities out of order")
	}
	// the project holding the high-priority activity leads; only-done projects come last, by name
	if !bodyOrder(body, "Team and people", "Order platform modernisation", "Operations and planning", "Platform and tooling") {
		t.Error("projects out of order")
	}
}

func TestDashboardFilters(t *testing.T) {
	h, _ := newApp(t)
	all := "status=planned&status=active&status=blocked&status=done"
	for _, c := range []struct {
		query         string
		shown, hidden []string
	}{
		{"period=q1&" + all, []string{"Consolidate order", "Move build logic", "Observability vendor"}, []string{"Kafka", "Hire senior", "2027 capacity"}},
		{"period=q4&month=10&" + all, []string{"Kafka", "Mentoring plan"}, []string{"2027 capacity", "Postmortem"}},
		{"period=q4&month=11&" + all, []string{"2027 capacity"}, []string{"Postmortem"}},
		{"project=3", []string{"Hire senior", "Team onboarding"}, []string{"Kafka", "Consolidate"}},
		{"creep=1", []string{"Consolidate order", "Kafka"}, []string{"Hire senior", "Mentoring"}},
		{"year=2025&" + all, nil, []string{"Consolidate", "Kafka"}},
	} {
		body := get(h, "/?"+c.query).Body.String()
		for _, n := range c.shown {
			if !strings.Contains(body, n) {
				t.Errorf("%s: %q missing", c.query, n)
			}
		}
		for _, n := range c.hidden {
			if strings.Contains(body, n) {
				t.Errorf("%s: %q shown", c.query, n)
			}
		}
	}
}

func TestSelectedActivityIgnoresFiltersButNotArchive(t *testing.T) {
	h, st := newApp(t)
	// activity 3 is done, hidden from the list by default, but its panel shows
	rec := get(h, "/?sel=3")
	wantBody(t, rec, `id="panel"`, "Move build logic")
	if strings.Contains(rec.Body.String(), `aria-current`) {
		t.Error("hidden activity has a row")
	}
	if err := st.SetProjectArchived(ctx, 2, true); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(get(h, "/?sel=3&status=done").Body.String(), `id="panel"`) {
		t.Error("panel of an archived project's activity shown")
	}
	if err := st.SetProjectArchived(ctx, 2, false); err != nil {
		t.Fatal(err)
	}
	wantStatus(t, get(h, "/?sel=3"), http.StatusOK)
}
