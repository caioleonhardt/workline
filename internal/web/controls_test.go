package web

import (
	"net/url"
	"slices"
	"strings"
	"testing"
)

func filterFor(t *testing.T, query string) Filter {
	t.Helper()
	q, err := url.ParseQuery(query)
	if err != nil {
		t.Fatal(err)
	}
	return parseFilter(q, date(t, testToday))
}

func hasPair(ps []pair, k, v string) bool { return slices.Contains(ps, pair{k, v}) }

func findToggle(t *testing.T, ts []toggle, label string) toggle {
	t.Helper()
	for _, tg := range ts {
		if tg.Label == label {
			return tg
		}
	}
	t.Fatalf("no toggle %q", label)
	return toggle{}
}

func TestControlToggleStates(t *testing.T) {
	f := filterFor(t, "period=q4&month=11&project=3&sel=2&status=active")
	v := buildControls(f, nil, nil, 0)

	// a period button clears the month and keeps the rest
	q1 := findToggle(t, v.Periods, "Q1")
	if hasPair(q1.Pairs, "month", "11") || !hasPair(q1.Pairs, "period", "q1") ||
		!hasPair(q1.Pairs, "project", "3") || !hasPair(q1.Pairs, "sel", "2") {
		t.Errorf("Q1 pairs: %v", q1.Pairs)
	}
	if q4 := findToggle(t, v.Periods, "Q4"); !q4.Pressed || findToggle(t, v.Periods, "Year").Pressed {
		t.Error("period pressed state")
	}
	if len(v.Scales) != 0 {
		t.Error("scale shown outside the Year view")
	}
	// a pill flips one status and keeps the others
	done := findToggle(t, v.Pills, "Done")
	if done.Pressed || !hasPair(done.Pairs, "status", "done") || !hasPair(done.Pairs, "status", "active") {
		t.Errorf("Done pill: %+v", done)
	}
	if active := findToggle(t, v.Pills, "Active"); !active.Pressed || hasPair(active.Pairs, "status", "active") ||
		!hasPair(active.Pairs, "status", "") {
		t.Errorf("Active pill (last one off leaves status=): %+v", active)
	}
	if !hasPair(v.Creep.Pairs, "creep", "1") || v.Creep.Pressed {
		t.Errorf("creep: %+v", v.Creep)
	}
	// the selects' form keeps everything but project and month
	if hasPair(v.SelectPairs, "project", "3") || hasPair(v.SelectPairs, "month", "11") || !hasPair(v.SelectPairs, "sel", "2") {
		t.Errorf("select pairs: %v", v.SelectPairs)
	}
	var months []string
	for _, o := range v.Months {
		months = append(months, o.Label)
	}
	if got := strings.Join(months, ","); got != "Whole Q4,Oct 2026,Nov 2026,Dec 2026" {
		t.Errorf("months: %s", got)
	}
	if v.ClearHref != "/roadmap?period=q4&sel=2" {
		t.Errorf("clear: %q", v.ClearHref)
	}
	if v.Eyebrow != "Q4 2026 · OCT–DEC" {
		t.Errorf("eyebrow: %q", v.Eyebrow)
	}
}

func TestControlsYearView(t *testing.T) {
	v := buildControls(filterFor(t, ""), nil, nil, 0)
	if v.Eyebrow != "2026 · ROADMAP" || v.ClearHref != "" || len(v.Scales) != 2 || len(v.Months) != 13 {
		t.Errorf("year view: %+v", v)
	}
	if v.PrevYear != "/roadmap?year=2025" || v.NextYear != "/roadmap?year=2027" {
		t.Errorf("year links: %q %q", v.PrevYear, v.NextYear)
	}
	if months := findToggle(t, v.Scales, "Months"); months.Pressed || !hasPair(months.Pairs, "scale", "month") {
		t.Errorf("Months: %+v", months)
	}
	if v := buildControls(filterFor(t, "year=2025&month=3&sel=4"), nil, nil, 0); v.PrevYear != "/roadmap?month=3&sel=4&year=2024" {
		t.Errorf("year nav keeps filters and selection: %q", v.PrevYear)
	}
}

func TestDashboardControlsMarkup(t *testing.T) {
	h, _ := newApp(t)
	body := get(h, "/roadmap").Body.String()
	for _, want := range []string{
		`aria-label="Period"`, `aria-label="Filter by status"`,
		`Planned <span class="mono">1</span>`, `Active <span class="mono">4</span>`,
		`Blocked <span class="mono">1</span>`, `Done <span class="mono">3</span>`,
		`Scope creep <span class="mono">2</span>`, `2026 · ROADMAP`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q", want)
		}
	}
	if strings.Contains(body, "Clear filters") {
		t.Error("Clear filters shown with default filters")
	}
	if !strings.Contains(get(h, "/roadmap?creep=1").Body.String(), "Clear filters") {
		t.Error("Clear filters missing when filtered")
	}
	// counts ignore the status filter but follow the period and project filters
	body = get(h, "/roadmap?period=q1&status=done").Body.String()
	for _, want := range []string{`Active <span class="mono">1</span>`, `Done <span class="mono">2</span>`, `Planned <span class="mono">0</span>`} {
		if !strings.Contains(body, want) {
			t.Errorf("q1: missing %q", want)
		}
	}
	if !strings.Contains(get(h, "/roadmap?project=3").Body.String(), `Active <span class="mono">3</span>`) {
		t.Error("project filter ignored by pill counts")
	}
}
