package web

import (
	"reflect"
	"strings"
	"testing"

	"workline/internal/domain"
)

func TestBuildChart(t *testing.T) {
	sub := func(id int64, created, closed string) domain.Subtask {
		s := domain.Subtask{ID: id, CreatedOn: date(t, created)}
		if closed != "" {
			s.ClosedOn = date(t, closed)
		}
		return s
	}
	a := domain.Activity{Status: domain.StatusActive, Start: date(t, "2026-11-02"), Due: date(t, "2026-12-06")}
	subs := []domain.Subtask{
		sub(1, "2026-11-02", "2026-11-10"), sub(2, "2026-11-02", ""), sub(3, "2026-11-09", ""), sub(4, "2026-11-17", ""),
	}
	c := buildChart(a, subs, date(t, testToday)) // Wednesday of the third week

	type bin struct {
		scope, open  string
		ahead, today bool
		tip          string
	}
	var got []bin
	for _, b := range c.Bins {
		got = append(got, bin{b.Scope.String(), b.Open.String(), b.Ahead, b.Today, b.Tip})
	}
	want := []bin{
		{"50.00", "50.00", false, false, "Week of 2026-11-02: 2 open of 2"},
		{"75.00", "50.00", false, false, "Week of 2026-11-09: 2 open of 3"}, // sub 1 closed on the 10th
		{"100.00", "75.00", false, true, "Week of 2026-11-16: 3 open of 4"}, // measured at today
		{"100.00", "0.00", true, false, "Week of 2026-11-23: ahead"},
		{"100.00", "0.00", true, false, "Week of 2026-11-30: ahead"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("bins:\n got %+v\nwant %+v", got, want)
	}
	if c.Peak != 3 || c.StartLabel != "2026-11-02" || c.EndLabel != "2026-12-06" || len(c.Rows) != 5 ||
		c.Summary != "Burndown by week: peak 3 open of 4 subtasks" {
		t.Errorf("chart: %+v", c)
	}
	if c.Rows[2] != (chartRow{"2026-11-16", 3, 4, false}) || !c.Rows[3].Ahead {
		t.Errorf("rows: %+v", c.Rows)
	}
	// no subtasks: nothing to divide by, nothing drawn
	empty := buildChart(a, nil, date(t, testToday))
	if empty.Peak != 0 || empty.Bins[0].Open != 0 || empty.Bins[0].Scope != 0 {
		t.Errorf("empty chart: %+v", empty.Bins[0])
	}
}

func TestPanelShowsBurndown(t *testing.T) {
	h, _ := newApp(t)
	body := get(h, "/?sel=2").Body.String() // Kafka: 12 weeks, the last one ahead; 6 subtasks
	for _, want := range []string{
		`peak 4 open · weekly`,
		`<div class="chart" role="img" aria-label="Burndown by week: peak 4 open of 6 subtasks">`,
		`title="Week of 2026-10-05: 4 open of 5"`,
		`<span class="scope blocked" style="height:83.33%"></span><span class="open blocked" style="height:66.67%"></span>`, // week of Oct 5
		`<span class="scope blocked ahead" style="height:100.00%"></span>`,
		`<span>2026-09-07</span><span>2026-11-27</span>`,
		`<tr><td>2026-11-23</td><td colspan="2">ahead</td></tr>`,
		`Scope (all subtasks)`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q", want)
		}
	}
	if n := strings.Count(body, `class="cbin"`); n != 12 {
		t.Errorf("bins = %d, want 12", n)
	}
	if n := strings.Count(body, `class="today-mark"`); n != 1 {
		t.Errorf("today markers = %d, want 1", n)
	}
}
