package web

import (
	"regexp"
	"strings"
	"testing"

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
	if _, err := st.CreateActivity(ctx, store.NewActivity{ProjectID: 1, Name: "Next year", Start: date(t, "2027-02-01"), Due: date(t, "2027-03-01"), Subtasks: []string{"x"}}, date(t, testToday)); err != nil {
		t.Fatal(err)
	}
	wantBody(t, get(h, "/?year=2027"), "1 planned")
}
