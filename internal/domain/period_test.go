package domain

import (
	"reflect"
	"testing"
	"time"
)

func colStr(cols []Column) []string {
	var out []string
	for _, c := range cols {
		out = append(out, c.Label+" "+c.Start.String()+".."+c.End.String())
	}
	return out
}

func TestColumns(t *testing.T) {
	tests := []struct {
		name  string
		year  int
		p     Period
		scale Scale
		want  []string
	}{
		{"year quarters", 2026, PeriodYear, ScaleQuarter, []string{
			"Q1 2026-01-01..2026-03-31", "Q2 2026-04-01..2026-06-30",
			"Q3 2026-07-01..2026-09-30", "Q4 2026-10-01..2026-12-31"}},
		{"year months leap", 2024, PeriodYear, ScaleMonth, nil},
		{"q4 months", 2026, PeriodQ4, ScaleQuarter, []string{
			"Oct 2026-10-01..2026-10-31", "Nov 2026-11-01..2026-11-30", "Dec 2026-12-01..2026-12-31"}},
		{"q1 ignores scale", 2026, PeriodQ1, ScaleMonth, []string{
			"Jan 2026-01-01..2026-01-31", "Feb 2026-02-01..2026-02-28", "Mar 2026-03-01..2026-03-31"}},
		{"q2", 2026, PeriodQ2, ScaleQuarter, []string{
			"Apr 2026-04-01..2026-04-30", "May 2026-05-01..2026-05-31", "Jun 2026-06-01..2026-06-30"}},
		{"q3 leap year q1", 2024, PeriodQ1, ScaleQuarter, []string{
			"Jan 2024-01-01..2024-01-31", "Feb 2024-02-01..2024-02-29", "Mar 2024-03-01..2024-03-31"}},
	}
	for _, c := range tests {
		got := colStr(Columns(c.year, c.p, c.scale))
		if c.want == nil {
			if len(got) != 12 || got[0] != "Jan 2024-01-01..2024-01-31" || got[1] != "Feb 2024-02-01..2024-02-29" || got[11] != "Dec 2024-12-01..2024-12-31" {
				t.Errorf("%s: %v", c.name, got)
			}
			continue
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: %v", c.name, got)
		}
	}
	mustPanic(t, "bad period", func() { Columns(2026, Period(9), ScaleQuarter) })
	mustPanic(t, "bad period range", func() { PeriodRange(2026, Period(-1)) })
}

func TestGridlines(t *testing.T) {
	if Gridlines(2026, PeriodYear) != nil {
		t.Error("year has no gridlines")
	}
	q4 := Gridlines(2026, PeriodQ4)
	if len(q4) != 13 || q4[0] != d(t, "2026-10-05") || q4[12] != d(t, "2026-12-28") {
		t.Errorf("Q4 2026: %d lines, first %v", len(q4), q4[0])
	}
	q1 := Gridlines(2026, PeriodQ1)
	if q1[0] != d(t, "2026-01-05") || q1[0].Day() != 5 {
		t.Errorf("Q1 2026 first = %v", q1[0])
	}
	// quarter starting on a Monday: that day is included (2025-12... use 2024-01-01, a Monday)
	if g := Gridlines(2024, PeriodQ1); g[0] != d(t, "2024-01-01") {
		t.Errorf("Q1 2024 first = %v", g[0])
	}
	for _, g := range q4 {
		if g.Weekday() != time.Monday {
			t.Errorf("%v is not a Monday", g)
		}
	}
}

func TestColumnStateAndLoad(t *testing.T) {
	today := d(t, "2026-11-18")
	cols := Columns(2026, PeriodYear, ScaleQuarter)
	if cols[2].IsToday(today) || !cols[3].IsToday(today) {
		t.Error("IsToday")
	}
	if !cols[3].IsToday(d(t, "2026-10-01")) || !cols[3].IsToday(d(t, "2026-12-31")) || cols[3].IsToday(d(t, "2027-01-01")) {
		t.Error("IsToday edges")
	}
	if !cols[3].OverlapsMonth(2026, time.November) || cols[2].OverlapsMonth(2026, time.November) {
		t.Error("quarter OverlapsMonth")
	}
	months := Columns(2026, PeriodYear, ScaleMonth)
	if !months[10].OverlapsMonth(2026, time.November) || months[9].OverlapsMonth(2026, time.November) || months[11].OverlapsMonth(2026, time.November) {
		t.Error("month OverlapsMonth")
	}
	if !months[11].OverlapsMonth(2026, time.December) {
		t.Error("december")
	}

	mk := func(start, due string, subs ...Subtask) ActivityWithSubtasks {
		return ActivityWithSubtasks{Activity: act(t, start, due), Subtasks: subs}
	}
	acts := []ActivityWithSubtasks{
		mk("2026-01-05", "2026-02-27", sub(t, 1, "2026-01-05", "2026-02-01"), sub(t, 2, "2026-01-06", "")), // Q1 only, 1 open leftover
		mk("2026-03-02", "2026-12-18", sub(t, 3, "2026-03-02", ""), sub(t, 4, "2026-11-01", "")),           // Q1..Q4
		mk("2026-11-23", "2026-12-31", sub(t, 5, "2026-11-23", "")),                                        // starts after today
	}
	tests := []struct {
		col   int
		label string
	}{
		{0, "2 open · 2 act."}, // ref = Mar 31: a0 open 1 (sub2), a1 open 1 (sub3)
		{1, "1 open · 1 act."}, // Q2: only a1
		{2, "1 open · 1 act."},
		{3, "2 open · 1 act."}, // Q4 current: ref today; a1 open 2; a2 not started
	}
	for _, c := range tests {
		l := ColumnLoad(cols[c.col], acts, today)
		if l.Planned || l.Label() != c.label {
			t.Errorf("column %d: %q (planned=%v)", c.col, l.Label(), l.Planned)
		}
	}

	future := Column{Label: "Q1", Start: d(t, "2027-01-01"), End: d(t, "2027-03-31")}
	fa := []ActivityWithSubtasks{mk("2026-12-01", "2027-01-15"), mk("2027-02-01", "2027-02-10"), mk("2026-01-01", "2026-12-31")}
	if l := ColumnLoad(future, fa, today); !l.Planned || l.Label() != "2 planned" {
		t.Errorf("future: %q", l.Label())
	}
	if l := ColumnLoad(cols[0], nil, today); l.Label() != "0 open · 0 act." {
		t.Errorf("empty: %q", l.Label())
	}
}

func TestPeriodRange(t *testing.T) {
	s, e := PeriodRange(2026, PeriodYear)
	if s != d(t, "2026-01-01") || e != d(t, "2026-12-31") {
		t.Errorf("year range %v..%v", s, e)
	}
}
