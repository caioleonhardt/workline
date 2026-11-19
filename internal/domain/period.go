package domain

import (
	"strconv"
	"time"
)

// Period selects what the dashboard shows.
type Period int

// Periods: the whole year or one quarter.
const (
	PeriodYear Period = iota
	PeriodQ1
	PeriodQ2
	PeriodQ3
	PeriodQ4
)

// Scale is the column granularity of the Year view.
type Scale int

// Scales.
const (
	ScaleQuarter Scale = iota
	ScaleMonth
)

// Column is one labelled slice of the period, inclusive of both ends.
type Column struct {
	Label      string
	Start, End Date
}

// Load is a column's workload summary.
type Load struct {
	Planned     bool // column lies fully in the future
	Open, Count int
}

// MonthRange returns the first and last day of a month.
func MonthRange(year int, m time.Month) (Date, Date) {
	start := DateOf(year, m, 1)
	var next Date
	if m == time.December {
		next = DateOf(year+1, time.January, 1)
	} else {
		next = DateOf(year, m+1, 1)
	}
	return start, next.AddDays(-1)
}

// PeriodRange returns the first and last day of the period.
func PeriodRange(year int, p Period) (Date, Date) {
	assert(p >= PeriodYear && p <= PeriodQ4, "unknown period")
	if p == PeriodYear {
		return DateOf(year, time.January, 1), DateOf(year, time.December, 31)
	}
	first := time.Month(3*(int(p)-1) + 1)
	start, _ := MonthRange(year, first)
	_, end := MonthRange(year, first+2)
	return start, end
}

// Columns returns the timeline columns (§4.5). scale only matters for PeriodYear.
func Columns(year int, p Period, s Scale) []Column {
	assert(p >= PeriodYear && p <= PeriodQ4, "unknown period")
	var cols []Column
	month := func(m time.Month) {
		st, en := MonthRange(year, m)
		cols = append(cols, Column{Label: m.String()[:3], Start: st, End: en})
	}
	switch {
	case p != PeriodYear:
		first := time.Month(3*(int(p)-1) + 1)
		for m := first; m < first+3; m++ {
			month(m)
		}
	case s == ScaleMonth:
		for m := time.January; m <= time.December; m++ {
			month(m)
		}
	default:
		labels := [...]string{"Q1", "Q2", "Q3", "Q4"}
		for q := PeriodQ1; q <= PeriodQ4; q++ {
			st, en := PeriodRange(year, q)
			cols = append(cols, Column{Label: labels[q-1], Start: st, End: en})
		}
	}
	return cols
}

// Gridlines returns the Mondays inside a quarter (§4.5). The Year period has none.
func Gridlines(year int, p Period) []Date {
	if p == PeriodYear {
		return nil
	}
	start, end := PeriodRange(year, p)
	d := start
	for d.Weekday() != time.Monday {
		d = d.AddDays(1)
	}
	var out []Date
	for ; !d.After(end); d = d.AddDays(7) {
		out = append(out, d)
	}
	return out
}

// Overlaps reports whether the activity's start..due span intersects from..to.
func (a Activity) Overlaps(from, to Date) bool {
	return !a.Start.After(to) && !a.Due.Before(from)
}

// IsToday reports whether today falls inside the column.
func (c Column) IsToday(today Date) bool {
	return !today.Before(c.Start) && !today.After(c.End)
}

// OverlapsMonth reports whether the column overlaps the given month of year.
func (c Column) OverlapsMonth(year int, m time.Month) bool {
	ms, me := MonthRange(year, m)
	return !c.End.Before(ms) && !c.Start.After(me)
}

// ColumnLoad summarises the visible activities for one column (§4.5, SPEC §10
// decision 8): both Count and Open consider activities overlapping the column
// that had started by min(column end, today).
func ColumnLoad(c Column, acts []ActivityWithTasks, today Date) Load {
	l := Load{Planned: c.Start.After(today)}
	ref := c.End
	if today.Before(ref) {
		ref = today
	}
	for _, a := range acts {
		if !a.Overlaps(c.Start, c.End) {
			continue
		}
		if l.Planned {
			l.Count++
			continue
		}
		if a.Start.After(ref) {
			continue
		}
		l.Count++
		l.Open += OpenAt(a.Tasks, ref)
	}
	return l
}

// Label formats the load for display.
func (l Load) Label() string {
	if l.Planned {
		return strconv.Itoa(l.Count) + " planned"
	}
	return strconv.Itoa(l.Open) + " open · " + strconv.Itoa(l.Count) + " act."
}
