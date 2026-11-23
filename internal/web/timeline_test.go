package web

import (
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"workline/internal/domain"
	"workline/internal/store"
)

func TestAxis(t *testing.T) {
	ax := newAxis(date(t, "2026-01-01"), date(t, "2026-12-31")) // 365 days
	for _, c := range []struct {
		day  string
		want string
	}{
		{"2025-06-01", "0.00"}, {"2026-01-01", "0.00"}, {"2026-11-18", "87.95"},
		{"2026-12-31", "99.73"}, {"2027-01-01", "100.00"}, {"2030-01-01", "100.00"},
	} {
		if got := ax.pos(date(t, c.day)).String(); got != c.want {
			t.Errorf("pos(%s) = %s, want %s", c.day, got, c.want)
		}
	}
	left, width := ax.span(date(t, "2026-03-02"), date(t, "2026-12-18"))
	if left.String() != "16.44" || width.String() != "80.00" {
		t.Errorf("span = %s, %s", left, width)
	}
	// spans crossing the edges are clipped
	if left, width := ax.span(date(t, "2025-12-01"), date(t, "2026-01-31")); left != 0 || width.String() != "8.49" {
		t.Errorf("clipped left: %s %s", left, width)
	}
	if left, width := ax.span(date(t, "2026-12-01"), date(t, "2027-02-01")); left.String() != "91.51" || width.String() != "8.49" {
		t.Errorf("clipped right: %s %s", left, width)
	}
	if _, width := ax.span(date(t, "2027-02-01"), date(t, "2027-03-01")); width != 0 {
		t.Errorf("outside span has width %s", width)
	}
}

func TestTimelineHeaderAndOverlay(t *testing.T) {
	h, _ := newApp(t)
	body := get(h, "/roadmap").Body.String()
	for _, want := range []string{
		`class="today-line" style="left:87.95%"`, `class="today-flag" style="left:87.95%">Today`,
		"Q1 <span", "Jan–Mar", "3 open · 1 act.", "13 open · 5 act.", "Project · activity", "6 activities shown",
		`class="col now" style="left:74.79%;width:25.21%"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("year view lacks %q", want)
		}
	}
	if strings.Contains(body, `class="gl"`) || strings.Contains(body, "tick mono") {
		t.Error("weekly gridlines only belong in the quarter view")
	}

	body = get(h, "/roadmap?period=q4&month=11").Body.String()
	for _, want := range []string{
		`class="today-line" style="left:52.17%"`, "Oct", "Nov", "Dec",
		`class="col sel" style="left:33.70%;width:32.61%"`, `class="band" style="left:33.70%;width:32.61%"`,
		`class="tick mono" aria-hidden="true" style="left:4.35%">5<`, // first Monday: Oct 5
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Q4 view lacks %q", want)
		}
	}
	if got := strings.Count(body, `class="gl`); got != 2+13 { // Nov and Dec starts, 13 Mondays
		t.Errorf("gridlines = %d", got)
	}
	// today is outside the period: no Today line
	if body := get(h, "/roadmap?period=q1").Body.String(); strings.Contains(body, "today-line") {
		t.Error("Today line outside the period")
	}
}

// Each project is one lane, its activities bars in it (SPEC §10 item 126).
func TestRoadmapLanes(t *testing.T) {
	h, st := newApp(t)
	body := get(h, "/roadmap?status=active&status=blocked&sel=1").Body.String()
	// lane label: count, progress over the visible activities, scope flags
	for _, want := range []string{
		`<div class="tl-row lane" data-project="1">`, `<span class="proj-name">Order platform modernisation</span>`,
		"2 activities · <span class=\"mono\">50% done</span>", "2 scope flags",
		"3 activities · <span class=\"mono\">60% done</span>", // Team and people
	} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q", want)
		}
	}
	if strings.Contains(body, "act-row") || strings.Contains(body, "act-name") {
		t.Error("activities still have rows of their own")
	}
	// the selected bar is current; its tooltip says what the row used to
	bar := barOf(t, body, "Consolidate order services into one Go service")
	for _, want := range []string{`aria-current="true"`, " sel", " creep", "Active", "Scope &#43;4", "3 open of 8 tasks", "2026-03-02 → 2026-12-18", "requested by Manager"} {
		if !strings.Contains(bar, want) {
			t.Errorf("selected bar lacks %q: %s", want, bar)
		}
	}
	if strings.Count(body, `aria-current`) != 1 {
		t.Error("exactly one bar is current")
	}
	// bars share a line unless they overlap: Jan–Mar and May–Jun on line 1, Feb–Apr on line 2
	p, err := st.CreateProject(ctx, "Lanes")
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range [][3]string{{"A", "2026-01-05", "2026-03-31"}, {"B", "2026-02-02", "2026-04-30"}, {"C", "2026-05-04", "2026-06-30"}} {
		if _, err := st.CreateActivity(ctx, store.NewActivity{ProjectID: p.ID, Name: a[0], Start: date(t, a[1]), Due: date(t, a[2]), Tasks: []string{"x"}}, domain.FixedClock{D: date(t, testToday)}); err != nil {
			t.Fatal(err)
		}
	}
	lane := get(h, "/roadmap?status=active&status=done&status=planned&project="+strconv.FormatInt(p.ID, 10)).Body.String()
	if !strings.Contains(lane, `<div class="tl-track" style="height:84px">`) {
		t.Error("lane is not two lines high")
	}
	for name, top := range map[string]string{"A": "top:8px", "B": "top:42px", "C": "top:8px"} {
		if b := barOf(t, lane, name); !strings.Contains(b, top) {
			t.Errorf("bar %s not at %s: %s", name, top, b)
		}
	}
	// no activities: the empty state
	wantBody(t, get(h, "/roadmap?status="), "No activities match these filters.")
}
func TestTimelineColumnLoadFuture(t *testing.T) {
	h, st := newApp(t)
	if _, err := st.CreateActivity(ctx, store.NewActivity{ProjectID: 1, Name: "Next year", Start: date(t, "2027-02-01"), Due: date(t, "2027-03-01"), Tasks: []string{"x"}}, domain.FixedClock{D: date(t, testToday)}); err != nil {
		t.Fatal(err)
	}
	wantBody(t, get(h, "/roadmap?year=2027"), "1 planned")
}

// barOf returns an activity's bar link, from its opening tag to its end.
func barOf(t *testing.T, body, name string) string {
	t.Helper()
	m := regexp.MustCompile(`<a class="bar [^>]*><span class="bar-fill"[^>]*></span><span class="bar-text">` + regexp.QuoteMeta(name) + ` · [^<]*</span></a>`).FindString(body)
	if m == "" {
		t.Fatalf("bar %q not found", name)
	}
	return m
}

// One bar per activity from start to due, clipped to the period, filled by
// the share of tasks done (SPEC §10 item 125).
func TestRoadmapBars(t *testing.T) {
	h, _ := newApp(t)
	row := barOf(t, get(h, "/roadmap").Body.String(), "Consolidate order services into one Go service")
	for _, want := range []string{
		`href="/roadmap?sel=1"`, `style="left:16.44%;width:80.00%;top:`,
		`<span class="bar-fill" style="width:63%"></span><span class="bar-text">Consolidate order services into one Go service · 63%</span></a>`,
	} {
		if !strings.Contains(row, want) {
			t.Errorf("year row lacks %q:\n%s", want, row)
		}
	}
	// a planned activity's bar starts at its start date
	row = barOf(t, get(h, "/roadmap").Body.String(), "2027 capacity and budget plan")
	if !strings.Contains(row, `class="bar planned"`) || !strings.Contains(row, "left:89.32%") {
		t.Errorf("planned row: %s", row)
	}
	// quarter view: a bar that started earlier is clipped at the quarter's start
	row = barOf(t, get(h, "/roadmap?period=q4").Body.String(), "Consolidate order services into one Go service")
	if !strings.Contains(row, `<a class="bar active clip-start creep"`) || !strings.Contains(row, `style="left:0.00%;width:85.87%;`) {
		t.Errorf("q4 row: %s", row)
	}
	// one running through the whole quarter is clipped at both ends
	row = barOf(t, get(h, "/roadmap?period=q3").Body.String(), "Consolidate order services into one Go service")
	if !strings.Contains(row, `<a class="bar active clip-start clip-end creep"`) || !strings.Contains(row, `style="left:0.00%;width:100.00%;`) {
		t.Errorf("q3 row: %s", row)
	}
	// an activity wholly outside the period is not listed
	if body := get(h, "/roadmap?period=q1&status=active&status=done").Body.String(); strings.Contains(body, "Hire senior ML") {
		t.Error("activity outside the period listed")
	}
	// no burndown is left
	if body := get(h, "/roadmap").Body.String(); strings.Contains(body, `class="bin`) || strings.Contains(body, "done-dot") || strings.Contains(body, `class="tile"`) {
		t.Error("the roadmap still draws bins, completion dots or tiles")
	}
	wantBody(t, get(h, "/roadmap"), "Activity: start to due, filled by tasks done")
}

func TestRoadmapScriptAndStorage(t *testing.T) {
	h, _ := newApp(t)
	body := get(h, "/roadmap").Body.String()
	script := strings.Index(body, `<script src="/static/app.js"></script>`)
	if script < strings.Index(body, `class="legend`) || strings.Count(body, `<script src="/static/app.js">`) != 1 {
		t.Error("app.js must load once, right after the roadmap")
	}
	js := get(h, "/static/app.js")
	wantStatus(t, js, http.StatusOK)
	lines := strings.Split(js.Body.String(), "\n")
	uses := 0
	for i, line := range lines {
		if strings.Contains(line, "localStorage.") {
			uses++
			if i == 0 || !strings.HasSuffix(strings.TrimSpace(lines[i-1]), "try {") {
				t.Errorf("localStorage outside a try block: %s", strings.TrimSpace(line))
			}
		}
	}
	if uses == 0 {
		t.Error("search history is not stored")
	}
}

func bodyOrderIn(s string, parts ...string) bool {
	last := -1
	for _, p := range parts {
		i := strings.Index(s, p)
		if i <= last {
			return false
		}
		last = i
	}
	return true
}

// The page is wide enough for the timeline (min 720px) and the panel (360px) to
// sit side by side (found in a real browser: at 1100px the panel wrapped below).
func TestPageIsWideEnoughForTheTimelineBesidePanel(t *testing.T) {
	h, _ := newApp(t)
	css := get(h, "/static/app.css").Body.String()
	for _, want := range []string{"main, .top { max-width: 1400px;", "grid-template-columns: minmax(0, 1fr) 360px;", `grid-template-areas: "timeline panel" "journal panel";`, ".detail { grid-area: panel; align-self: start;"} {
		if !strings.Contains(css, want) {
			t.Errorf("app.css lacks %q", want)
		}
	}
}

// The Today chip needs its own strip under the column's load text (found in a
// real browser: at 72px the chip covered the "13 open · …" line).
func TestTodayChipHasItsOwnStrip(t *testing.T) {
	h, _ := newApp(t)
	css := get(h, "/static/app.css").Body.String()
	for _, want := range []string{".head-track { height: 88px; }", "padding: 6px 10px;", ".today-flag { position: absolute; bottom: 20px;"} {
		if !strings.Contains(css, want) {
			t.Errorf("app.css lacks %q", want)
		}
	}
}

// Narrow screens (SPEC §10 item 70): sticky labels, a 160px label column on
// phones, wrapping stat boxes and 44px disclosure summaries. Checked in Firefox
// at phone, tablet and wide sizes; this keeps the rules from being dropped.
func TestNarrowScreenRules(t *testing.T) {
	h, _ := newApp(t)
	css := get(h, "/static/app.css").Body.String()
	for _, want := range []string{
		"--label-w: 300px;", ".tl-label { position: sticky; left: 0;", "@media (max-width: 640px) { :root { --label-w: 160px; }",
		"grid-template-columns: repeat(auto-fit, minmax(96px, 1fr))", "summary { display: flex; align-items: center; min-height: 44px;",
	} {
		if !strings.Contains(css, want) {
			t.Errorf("app.css lacks %q", want)
		}
	}
	if strings.Contains(css, "left: 300px") {
		t.Error("the overlay must follow --label-w")
	}
}

// Every bar is a real link naming its activity, so it is in the tab order and
// the accessibility tree (SPEC §10 item 126).
func TestBarsAreLinks(t *testing.T) {
	h, _ := newApp(t)
	body := get(h, "/roadmap").Body.String()
	if n := strings.Count(body, `<a class="bar `); n != 6 {
		t.Fatalf("bars = %d, want 6", n)
	}
	for _, m := range regexp.MustCompile(`<a class="bar [^>]*>`).FindAllString(body, -1) {
		if strings.Contains(m, `tabindex="-1"`) || strings.Contains(m, `aria-hidden`) || !strings.Contains(m, `href="/roadmap?sel=`) {
			t.Errorf("bar is not a plain link: %s", m)
		}
	}
}

// A done activity closed after its due date runs to its close: green within
// schedule, orange through the tolerance, red after (SPEC §10 item 128).
func TestLateCloseGradient(t *testing.T) {
	h, st := newApp(t)
	clock := domain.FixedClock{D: date(t, testToday)}
	a, err := st.CreateActivity(ctx, store.NewActivity{ProjectID: 2, Name: "Late one", Start: date(t, "2026-10-01"), Due: date(t, "2026-10-10"), Tasks: []string{"x"}}, clock)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetActivityStatus(ctx, a.ID, domain.StatusDone, date(t, testToday)); err != nil {
		t.Fatal(err)
	}
	// 2026-10-01..11-18 is 49 days; due ends day 10, the 2-day tolerance day 12
	bar := barOf(t, get(h, "/roadmap?status=done").Body.String(), "Late one")
	for _, want := range []string{`class="bar done late"`, "--due:20.41%;--tolerated:24.49%", "closed 2026-11-18, 39 days late"} {
		if !strings.Contains(bar, want) {
			t.Errorf("bar lacks %q: %s", want, bar)
		}
	}
	// on time: a plain bar
	if bar := barOf(t, get(h, "/roadmap?status=done").Body.String(), "Postmortem: inference latency incident"); strings.Contains(bar, " late\"") || strings.Contains(bar, "--due") {
		t.Errorf("on-time bar drawn late: %s", bar)
	}
	css := get(h, "/static/app.css").Body.String()
	if !strings.Contains(css, ".bar.late {") || !strings.Contains(css, "var(--warn) calc(var(--tolerated) - 3%), var(--over) calc(var(--tolerated) + 3%)") {
		t.Error("the late gradient is missing")
	}
}
