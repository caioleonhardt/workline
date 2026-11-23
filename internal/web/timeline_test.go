package web

import (
	"net/http"
	"regexp"
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

func TestTimelineGroupsAndRows(t *testing.T) {
	h, _ := newApp(t)
	body := get(h, "/roadmap?status=active&status=blocked&sel=1").Body.String()
	// project row: count, progress over the visible activities, scope flags
	for _, want := range []string{
		"Order platform modernisation", "· 2 activities", "· 50% done", "· 2 scope flags",
		"· 3 activities", "· 60% done", // Team and people
	} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q", want)
		}
	}
	// the selected row's link carries aria-current; the scope chip names the added tasks
	row := regexp.MustCompile(`(?s)<div class="tl-label act-label">\s*<a class="act-name" href="[^"]*" aria-current="true">.*?</div>`).FindString(body)
	for _, want := range []string{"Consolidate order services", "Active", "Scope &#43;4", "3 open / 8", "2026-03-02 → 2026-12-18 · Manager"} {
		if !strings.Contains(row, want) {
			t.Errorf("selected row lacks %q: %s", want, row)
		}
	}
	if strings.Count(body, `aria-current`) != 1 {
		t.Error("exactly one row is current")
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

// rowTrack returns the markup of an activity's track, from its name to its bar link.
func rowTrack(t *testing.T, body, name string) string {
	t.Helper()
	i := strings.Index(body, `>`+name+`</a>`)
	if i < 0 {
		t.Fatalf("row %q not found", name)
	}
	j := strings.Index(body[i:], `class="row-link"`)
	return body[i : i+j+250]
}

// One bar per activity from start to due, clipped to the period, filled by
// the share of tasks done (SPEC §10 item 125).
func TestRoadmapBars(t *testing.T) {
	h, _ := newApp(t)
	row := rowTrack(t, get(h, "/roadmap").Body.String(), "Consolidate order services into one Go service")
	for _, want := range []string{
		`<span class="bar active" aria-hidden="true" style="left:16.44%;width:80.00%"><span class="bar-fill" style="width:63%"></span><span class="bar-text">Consolidate order services into one Go service · 63%</span></span>`,
		`title="Consolidate order services into one Go service: 2026-03-02 → 2026-12-18, 63% done, 3 open tasks. Show details" tabindex="-1" aria-hidden="true"`,
	} {
		if !strings.Contains(row, want) {
			t.Errorf("year row lacks %q:\n%s", want, row)
		}
	}
	// a planned activity's bar starts at its start date
	row = rowTrack(t, get(h, "/roadmap").Body.String(), "2027 capacity and budget plan")
	if !strings.Contains(row, `class="bar planned"`) || !strings.Contains(row, "left:89.32%") {
		t.Errorf("planned row: %s", row)
	}
	// quarter view: a bar that started earlier is clipped at the quarter's start
	row = rowTrack(t, get(h, "/roadmap?period=q4").Body.String(), "Consolidate order services into one Go service")
	if !strings.Contains(row, `<span class="bar active clip-start" aria-hidden="true" style="left:0.00%;width:85.87%">`) {
		t.Errorf("q4 row: %s", row)
	}
	// one running through the whole quarter is clipped at both ends
	row = rowTrack(t, get(h, "/roadmap?period=q3").Body.String(), "Consolidate order services into one Go service")
	if !strings.Contains(row, `<span class="bar active clip-start clip-end" aria-hidden="true" style="left:0.00%;width:100.00%">`) {
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

func TestProjectRowIsAnExpandedToggle(t *testing.T) {
	h, _ := newApp(t)
	body := get(h, "/roadmap").Body.String()
	for _, want := range []string{
		`<div class="grp" data-project="1">`,
		`<button type="button" class="proj-row proj-toggle" aria-expanded="true">`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q", want)
		}
	}
	// the script follows the timeline so the stored state applies before first paint
	script := strings.Index(body, `<script src="/static/app.js"></script>`)
	if script < strings.Index(body, `class="legend`) || strings.Count(body, `<script src="/static/app.js">`) != 1 {
		t.Error("app.js must load once, right after the timeline")
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
		t.Error("project collapse state is not stored")
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

// Whole-row selection (SPEC §10 item 80): the name is the one link in the
// accessibility tree and the tab order; a pointer-only overlay covers the row.
func TestRowSelectionOverlayIsPointerOnly(t *testing.T) {
	h, _ := newApp(t)
	body := get(h, "/roadmap").Body.String()
	rows := strings.Count(body, `class="tl-row act-row`)
	if rows != 6 || strings.Count(body, `class="act-name" href="`) != rows || strings.Count(body, `class="row-link"`) != rows {
		t.Fatalf("rows=%d names=%d overlays=%d", rows, strings.Count(body, `class="act-name" href="`), strings.Count(body, `class="row-link"`))
	}
	for _, m := range regexp.MustCompile(`<a class="row-link"[^>]*>`).FindAllString(body, -1) {
		if !strings.Contains(m, `tabindex="-1"`) || !strings.Contains(m, `aria-hidden="true"`) {
			t.Errorf("overlay is reachable by keyboard or assistive technology: %s", m)
		}
	}
	if strings.Contains(body, "bar-link") {
		t.Error("the duplicate bar link is gone")
	}
	css := get(h, "/static/app.css").Body.String()
	for _, want := range []string{".row-link { position: absolute; inset: 0; z-index: 3;"} {
		if !strings.Contains(css, want) {
			t.Errorf("app.css lacks %q", want)
		}
	}
}
