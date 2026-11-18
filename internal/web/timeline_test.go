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

func TestTimelineJournalDotsAndLegend(t *testing.T) {
	h, st := newApp(t)
	body := get(h, "/").Body.String()
	kafka := rowTrack(t, body, "Kafka upgrade readiness")
	if n := strings.Count(kafka, `class="dot-mark`); n != 4 {
		t.Errorf("dots = %d, want 4", n)
	}
	if !strings.Contains(kafka, `class="dot-mark kind-blocker" role="img" aria-label="2026-11-03, blocker: Waiting on platform team for a maintenance window" title="2026-11-03, blocker: Waiting on platform team for a maintenance window" style="left:83.84%;bottom:10px"`) {
		t.Errorf("blocker dot missing: %s", kafka)
	}
	// dots outside the period are dropped
	q4 := rowTrack(t, get(h, "/?period=q4").Body.String(), "Kafka upgrade readiness")
	if n := strings.Count(q4, `class="dot-mark`); n != 3 {
		t.Errorf("q4 dots = %d, want 3 (Sep 7 is before the quarter)", n)
	}
	// an acknowledgement adds a scope_ack dot
	if _, err := st.AckScope(ctx, 2, domain.FixedClock{D: date(t, testToday)}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(get(h, "/").Body.String(), "kind-scope_ack") {
		t.Error("scope_ack dot missing")
	}
	wantBody(t, get(h, "/"), "Activity: open subtasks (taller = more open)", "Until due date", "Journal entry")
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
	if script < strings.Index(body, `class="legend`) || strings.Count(body, "<script") != 1 {
		t.Error("app.js must be the only script, loaded right after the timeline")
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
	if _, err := st.ToggleSubtask(ctx, subs[0].ID, date(t, "2026-11-02")); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 6; i++ {
		s, err := st.AddSubtask(ctx, a.ID, "tue-fri", date(t, "2026-11-03")) // Tuesday
		if err != nil {
			t.Fatal(err)
		}
		if _, err := st.ToggleSubtask(ctx, s.ID, date(t, "2026-11-06")); err != nil { // Friday
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

func TestJournalDotsStackByDay(t *testing.T) {
	h, st := newApp(t)
	add := func(day, at, kind, text string) {
		t.Helper()
		if _, err := st.AddJournal(ctx, domain.JournalEntry{ActivityID: 7, Date: date(t, day), Kind: domain.Kind(kind), Text: text, LoggedAt: at}); err != nil {
			t.Fatal(err)
		}
	}
	// Team onboarding guide (activity 7): two seeded entries on Oct 5 and Oct 29
	add("2026-10-29", "2026-10-29 15:00", "decision", "second")
	add("2026-10-29", "2026-10-29 09:30", "blocker", "first")
	row := rowTrack(t, get(h, "/").Body.String(), "Team onboarding guide refresh")
	dots := regexp.MustCompile(`class="dot-mark kind-(\w+)" role="img" aria-label="([^"]*)" title="[^"]*" style="left:([\d.]+)%;bottom:(\d+)px"`).FindAllStringSubmatch(row, -1)
	type dot struct{ kind, label, left, bottom string }
	var got []dot
	for _, d := range dots {
		got = append(got, dot{d[1], d[2], d[3], d[4]})
	}
	want := []dot{
		{"update", "2026-10-05, update: Started", "75.89", "10"},
		{"update", "2026-10-29, update: Local environment section rewritten", "82.47", "10"}, // no logged time: bottom of the stack
		{"blocker", "2026-10-29 09:30, blocker: first", "82.47", "19"},                       // oldest logged time lowest
		{"decision", "2026-10-29 15:00, decision: second", "82.47", "28"},
	}
	if len(got) != len(want) {
		t.Fatalf("dots: %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("dot %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestJournalDotStackStopsAtTheRowTop(t *testing.T) {
	h, st := newApp(t)
	for i := 0; i < 9; i++ {
		if _, err := st.AddJournal(ctx, domain.JournalEntry{ActivityID: 7, Date: date(t, "2026-11-02"), Kind: domain.KindUpdate, Text: "n"}); err != nil {
			t.Fatal(err)
		}
	}
	row := rowTrack(t, get(h, "/").Body.String(), "Team onboarding guide refresh")
	if n := strings.Count(row, `style="left:83.56%;bottom:`); n != 9 {
		t.Errorf("every entry keeps its dot: %d", n)
	}
	for _, bottom := range []string{"bottom:10px", "bottom:19px", "bottom:28px", "bottom:37px", "bottom:46px", "bottom:55px"} {
		if !strings.Contains(row, bottom) {
			t.Errorf("missing slot %s", bottom)
		}
	}
	if strings.Contains(row, "bottom:64px") {
		t.Error("stack grew past its cap")
	}
	if got := strings.Count(row, "left:83.56%;bottom:55px"); got != 4 { // slots 5..8 share the top slot
		t.Errorf("overflowing dots on the top slot = %d, want 4", got)
	}
}
