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
	body := get(h, "/").Body.String()
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

	body = get(h, "/?period=q4&month=11").Body.String()
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
	if body := get(h, "/?period=q1").Body.String(); strings.Contains(body, "today-line") {
		t.Error("Today line outside the period")
	}
}

func TestTimelineGroupsAndRows(t *testing.T) {
	h, _ := newApp(t)
	body := get(h, "/?status=active&status=blocked&sel=1").Body.String()
	// project row: count, progress over the visible activities, scope flags
	for _, want := range []string{
		"Order platform modernisation", "· 2 activities", "· 50% done", "· 2 scope flags",
		"· 3 activities", "· 60% done", // Team and people
	} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q", want)
		}
	}
	// the selected row's link carries aria-current; the scope chip names the added subtasks
	row := regexp.MustCompile(`(?s)<a class="tl-label act-label" href="[^"]*" aria-current="true">.*?</a>`).FindString(body)
	for _, want := range []string{"Consolidate order services", "Active", "Scope &#43;4", "3 open / 8", "2026-03-02 → 2026-12-18 · Manager"} {
		if !strings.Contains(row, want) {
			t.Errorf("selected row lacks %q: %s", want, row)
		}
	}
	if strings.Count(body, `aria-current`) != 1 {
		t.Error("exactly one row is current")
	}
	// no activities: the empty state
	wantBody(t, get(h, "/?status="), "No activities match these filters.")
}

func TestTimelineColumnLoadFuture(t *testing.T) {
	h, st := newApp(t)
	if _, err := st.CreateActivity(ctx, store.NewActivity{ProjectID: 1, Name: "Next year", Start: date(t, "2027-02-01"), Due: date(t, "2027-03-01"), Subtasks: []string{"x"}}, domain.FixedClock{D: date(t, testToday)}); err != nil {
		t.Fatal(err)
	}
	wantBody(t, get(h, "/?year=2027"), "1 planned")
}

// rowTrack returns the markup of an activity's track, from its name to its bar link.
func rowTrack(t *testing.T, body, name string) string {
	t.Helper()
	i := strings.Index(body, `<span class="act-name">`+name)
	if i < 0 {
		t.Fatalf("row %q not found", name)
	}
	j := strings.Index(body[i:], `class="bar-link"`)
	return body[i : i+j+250]
}

func TestTimelineRowSpanPlanAndLabel(t *testing.T) {
	h, _ := newApp(t)
	row := rowTrack(t, get(h, "/").Body.String(), "Consolidate order services into one Go service")
	for _, want := range []string{
		`class="span-line active" aria-hidden="true" style="left:16.44%;width:80.00%"`,
		`class="plan active" aria-hidden="true" style="left:87.95%;width:8.49%"`, // today..due
		`style="left:92.00%">63%`, // label sits at the due date, capped at 92%
		`aria-label="Consolidate order services into one Go service: 63% done, 3 open subtasks. Show details"`,
	} {
		if !strings.Contains(row, want) {
			t.Errorf("year row lacks %q:\n%s", want, row)
		}
	}
	// a planned activity's box starts at its start date
	row = rowTrack(t, get(h, "/").Body.String(), "2027 capacity and budget plan")
	if !strings.Contains(row, `class="plan planned"`) || !strings.Contains(row, "left:89.32%") {
		t.Errorf("planned row: %s", row)
	}

	// quarter view: spans are clipped to the quarter, positions use the quarter's 92 days
	q4 := get(h, "/?period=q4").Body.String()
	row = rowTrack(t, q4, "Consolidate order services into one Go service")
	for _, want := range []string{`style="left:0.00%;width:85.87%"`, `class="plan active" aria-hidden="true" style="left:52.17%;width:33.70%"`} {
		if !strings.Contains(row, want) {
			t.Errorf("q4 row lacks %q:\n%s", want, row)
		}
	}
	// done activities have no plan box
	if strings.Contains(get(h, "/?status=done").Body.String(), `class="plan done"`) {
		t.Error("done activity has a plan box")
	}
	// an activity wholly outside the period draws nothing visible
	if body := get(h, "/?period=q1&status=active&status=done").Body.String(); strings.Contains(body, "Hire senior ML") {
		t.Error("activity outside the period listed")
	}
}

func TestTimelineBinsUseGlobalMax(t *testing.T) {
	h, _ := newApp(t)
	body := get(h, "/").Body.String()
	// the tallest weekly open count among visible activities is 5 (activity 1)
	if !strings.Contains(body, "height:100.00%") {
		t.Error("no bin reaches the global maximum")
	}
	for _, m := range regexp.MustCompile(`height:(\d+\.\d+)%`).FindAllStringSubmatch(body, -1) {
		if h, _ := strconv.ParseFloat(m[1], 64); h > 100 {
			t.Errorf("bin taller than the maximum: %s", m[1])
		}
	}
	kafka := rowTrack(t, body, "Kafka upgrade readiness")
	if n := strings.Count(kafka, `class="bin blocked"`); n != 11 {
		t.Errorf("year view: %d weekly bins, want 11", n)
	}
	// the last, partial week (Nov 16-18) has 4 open of 5
	if !strings.Contains(kafka, `style="left:87.40%;width:0.82%;height:80.00%"`) {
		t.Errorf("last bin: %s", kafka)
	}

	// quarter view: 2-day bins clipped to the quarter
	q4 := rowTrack(t, get(h, "/?period=q4").Body.String(), "Kafka upgrade readiness")
	if n := strings.Count(q4, `class="bin blocked"`); n != 25 {
		t.Errorf("quarter view: %d bins, want 25", n)
	}
	// done activities stop at their done date: seeded done_on is the due date, so
	// Postmortem (Jun 8 - Jun 26) has 3 weekly bins
	post := rowTrack(t, get(h, "/?status=done").Body.String(), "Postmortem: inference latency incident")
	if n := strings.Count(post, `class="bin done"`); n != 3 {
		t.Errorf("done activity bins = %d, want 3", n)
	}
}

func TestTimelineHasNoJournalDots(t *testing.T) {
	h, _ := newApp(t)
	body := get(h, "/").Body.String()
	if strings.Contains(body, "dot-mark") || strings.Contains(body, "</i>Journal entry") {
		t.Error("journal entries must not appear on the timeline")
	}
	wantBody(t, get(h, "/"), "Activity: open subtasks (taller = more open)", "Until due date")
}

func TestProjectRowIsAnExpandedToggle(t *testing.T) {
	h, _ := newApp(t)
	body := get(h, "/").Body.String()
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
	if got := strings.Count(js.Body.String(), "localStorage."); got != 2 {
		t.Errorf("localStorage used %d times; each access must sit in a try block", got)
	}
}

func maxBinHeight(t *testing.T, body string) float64 {
	t.Helper()
	top := 0.0
	for _, m := range regexp.MustCompile(`height:(\d+\.\d+)%`).FindAllStringSubmatch(body, -1) {
		h, _ := strconv.ParseFloat(m[1], 64)
		top = max(top, h)
	}
	return top
}

// A subtask open from Tuesday to Friday is invisible in weekly samples but
// shows in 2-day bins; heights must be scaled to the bins drawn (SPEC §10 item 48).
func TestQuarterBinsNeverExceedFullHeight(t *testing.T) {
	h, st := newApp(t)
	a, err := st.CreateActivity(ctx, store.NewActivity{
		NewProjectName: "Short work", Name: "Tue to Fri", Start: date(t, "2026-11-02"), Due: date(t, "2026-12-31"),
		Subtasks: []string{"closed at once"}, // created Mon Nov 2
	}, domain.FixedClock{D: date(t, testToday)})
	if err != nil {
		t.Fatal(err)
	}
	subs, _ := st.ListSubtasks(ctx, a.ID)
	if _, err := st.ToggleSubtask(ctx, subs[0].ID, domain.FixedClock{D: date(t, "2026-11-02")}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 6; i++ {
		s, err := st.AddSubtask(ctx, a.ID, "tue-fri", date(t, "2026-11-03")) // Tuesday
		if err != nil {
			t.Fatal(err)
		}
		if _, err := st.ToggleSubtask(ctx, s.ID, domain.FixedClock{D: date(t, "2026-11-06")}); err != nil { // Friday
			t.Fatal(err)
		}
	}
	// alone in its project, the 2-day bins reach 6 open while every weekly sample is 0
	solo := get(h, "/?period=q4&project=5").Body.String()
	if got := maxBinHeight(t, solo); got != 100 {
		t.Errorf("tallest quarter bin = %.2f%%, want 100", got)
	}
	// next to the seeded activities the bins still never exceed the maximum
	if got := maxBinHeight(t, get(h, "/?period=q4").Body.String()); got != 100 {
		t.Errorf("tallest bin with all activities = %.2f%%, want 100", got)
	}
	// the year view samples weekly: nothing is open at any week end, so its bars stay flat
	if got := maxBinHeight(t, get(h, "/?project=5").Body.String()); got != 0 {
		t.Errorf("year view bar = %.2f%%, want 0", got)
	}
}

func TestCompletionDotsOnePerDay(t *testing.T) {
	h, st := newApp(t)
	const name = "Kafka upgrade readiness"
	row := rowTrack(t, get(h, "/").Body.String(), name)
	// seeded: one subtask closed on Oct 5 and one on Oct 19, neither with a time
	if n := strings.Count(row, `class="done-dot center`); n != 2 {
		t.Fatalf("dots = %d, want 2:\n%s", n, row)
	}
	for _, want := range []string{
		`class="done-dot center down" style="left:75.89%"><button type="button" class="dot" aria-label="1 subtask completed on 2026-10-05" aria-describedby="done-2-2026-10-05">`,
		`<span class="popup" id="done-2-2026-10-05"><strong>2026-10-05</strong><ul><li>Compatibility matrix</li></ul>`,
		`style="left:79.73%"`,
	} {
		if !strings.Contains(row, want) {
			t.Errorf("missing %q in:\n%s", want, row)
		}
	}
	// outside the period nothing is drawn; inside, the same dots
	if n := strings.Count(rowTrack(t, get(h, "/?period=q3").Body.String(), name), "done-dot"); n != 0 {
		t.Errorf("Q3 dots = %d", n)
	}
	if n := strings.Count(rowTrack(t, get(h, "/?period=q4").Body.String(), name), `class="done-dot`); n != 2 {
		t.Errorf("Q4 dots = %d", n)
	}

	// a day with several closures is still one dot; the popup lists them by time, then id
	close := func(text string, created, closedOn string, hour, minute int) {
		t.Helper()
		sub, err := st.AddSubtask(ctx, 2, text, date(t, created))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := st.ToggleSubtask(ctx, sub.ID, domain.FixedClock{D: date(t, closedOn), Hour: hour, Minute: minute}); err != nil {
			t.Fatal(err)
		}
	}
	close("afternoon", "2026-10-01", "2026-10-05", 15, 45)
	close("morning", "2026-10-01", "2026-10-05", 8, 10)
	close("same minute, later id", "2026-10-01", "2026-10-05", 8, 10)
	row = rowTrack(t, get(h, "/").Body.String(), name)
	if n := strings.Count(row, `class="done-dot`); n != 2 {
		t.Errorf("dots = %d, want still 2", n)
	}
	popup := row[strings.Index(row, `id="done-2-2026-10-05"`):]
	popup = popup[:strings.Index(popup, "</ul>")]
	if !bodyOrderIn(popup, "Compatibility matrix", `<span class="mono">08:10</span> morning`, `<span class="mono">08:10</span> same minute, later id`, `<span class="mono">15:45</span> afternoon`) {
		t.Errorf("popup order: %s", popup)
	}
	wantBody(t, get(h, "/"), "4 subtasks completed on 2026-10-05")
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

func TestCompletionPopupsStayInsideTheTrack(t *testing.T) {
	h, st := newApp(t)
	sub, _ := st.AddSubtask(ctx, 2, "early", date(t, "2026-01-02"))
	if _, err := st.ToggleSubtask(ctx, sub.ID, domain.FixedClock{D: date(t, "2026-01-03")}); err != nil {
		t.Fatal(err)
	}
	sub, _ = st.AddSubtask(ctx, 2, "today", date(t, "2026-11-01"))
	if _, err := st.ToggleSubtask(ctx, sub.ID, domain.FixedClock{D: date(t, testToday)}); err != nil {
		t.Fatal(err)
	}
	row := rowTrack(t, get(h, "/").Body.String(), "Kafka upgrade readiness")
	for _, want := range []string{`class="done-dot left down" style="left:0.55%"`, `class="done-dot right down" style="left:87.95%"`} {
		if !strings.Contains(row, want) {
			t.Errorf("missing %q", want)
		}
	}
	// Q4 drops the January dot
	if q4 := rowTrack(t, get(h, "/?period=q4").Body.String(), "Kafka upgrade readiness"); strings.Contains(q4, "2026-01-03") {
		t.Error("dot outside the period")
	}
	wantBody(t, get(h, "/"), "Subtasks completed")
}

// Popups open upward and fit above any row; the first two rows open downward
// instead, but only when two more rows follow to hold them.
func TestCompletionPopupsOpenDownInTheFirstRows(t *testing.T) {
	h, _ := newApp(t)
	downIn := func(body, name string) bool {
		t.Helper()
		row := rowTrack(t, body, name)
		if !strings.Contains(row, "done-dot") {
			t.Fatalf("%s has no dots", name)
		}
		return strings.Contains(row, " down\"")
	}
	// all default activities: six rows, so rows 1 and 2 (Kafka, Consolidate) open down
	body := get(h, "/").Body.String()
	var order []string
	for _, m := range regexp.MustCompile(`<span class="act-name">([^<]*)</span>`).FindAllStringSubmatch(body, -1) {
		order = append(order, m[1])
	}
	if len(order) != 6 {
		t.Fatalf("rows: %v", order)
	}
	for i, name := range order {
		row := rowTrack(t, body, name)
		if got := strings.Contains(row, " down\""); strings.Contains(row, "done-dot") && got != (i < 2) {
			t.Errorf("row %d (%s): down = %v", i, name, got)
		}
	}
	// project 3 has three rows: only the first has two rows below it
	p3 := get(h, "/?project=3").Body.String()
	if !downIn(p3, "Team onboarding guide refresh") || downIn(p3, "Hire senior ML engineer") || downIn(p3, "Mentoring plan for two juniors") {
		t.Error("project 3: only its first row opens downward")
	}
	// a lone row cannot hold a downward popup
	if downIn(get(h, "/?project=4&status=done").Body.String(), "Postmortem: inference latency incident") {
		t.Error("a single row opens downward")
	}
}

// Bins sit inside .bins, which is itself positioned, so each bin must position
// itself against it; otherwise it collapses to nothing (found in a real browser).
func TestBinsArePositionedAgainstTheirContainer(t *testing.T) {
	h, _ := newApp(t)
	css := get(h, "/static/app.css").Body.String()
	for _, want := range []string{".bins { left: 0; right: 0; bottom: 16px; height: 44px; }", ".bin { position: absolute; bottom: 0;"} {
		if !strings.Contains(css, want) {
			t.Errorf("app.css lacks %q", want)
		}
	}
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
