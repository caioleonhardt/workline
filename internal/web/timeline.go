package web

import (
	"fmt"
	"strconv"
	"strings"

	"workline/internal/domain"
)

// pct is a percentage of the track width; it prints as "12.34" so templates
// can emit it straight into an inline style.
type pct float64

func (p pct) String() string { return strconv.FormatFloat(float64(p), 'f', 2, 64) }

// axis maps dates to horizontal positions on the track.
type axis struct {
	start domain.Date
	days  int
}

func newAxis(from, to domain.Date) axis { return axis{start: from, days: to.Sub(from) + 1} }

// pos is the position of the start of day d, clamped to the track.
func (a axis) pos(d domain.Date) pct {
	p := float64(d.Sub(a.start)) / float64(a.days) * 100
	return pct(min(max(p, 0), 100))
}

// span is the left edge and width of the inclusive days from..to.
func (a axis) span(from, to domain.Date) (left, width pct) {
	left = a.pos(from)
	return left, a.pos(to.AddDays(1)) - left
}

type columnView struct {
	Left, Width pct
	Label, Sub  string
	Load        string
	Class       string // "sel" (selected month), "now" (today), or empty
}

type lineView struct {
	Left   pct
	Strong bool
}

type tickView struct {
	Left  pct
	Label string
}

type rowView struct {
	ID                  int64
	Name, Href          string
	Selected            bool
	Status, StatusLabel string
	ScopeChip           string // empty unless flagged
	OpenLabel, Dates    string
}

type groupView struct {
	Name, CountLabel, PctLabel, FlagLabel string
	Rows                                  []rowView
}

type timelineView struct {
	ShownLabel          string
	Columns             []columnView
	Lines               []lineView
	Ticks               []tickView
	ShowToday           bool
	TodayLeft           pct
	HasBand             bool
	BandLeft, BandWidth pct
	Groups              []groupView
}

func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}

func capitalize(s string) string { return strings.ToUpper(s[:1]) + s[1:] }

// buildTimeline lays out the visible groups on the period's track.
func buildTimeline(f Filter, today domain.Date, groups []domain.Group, itemOf map[int64]domain.ActivityWithSubtasks, creeps map[int64]domain.Creep) timelineView {
	from, to := domain.PeriodRange(f.Year, f.Period)
	ax := newAxis(from, to)
	var visible []domain.ActivityWithSubtasks
	for _, g := range groups {
		for _, a := range g.Activities {
			visible = append(visible, itemOf[a.ID])
		}
	}
	v := timelineView{ShownLabel: plural(len(visible), "activity", "activities") + " shown"}

	for _, c := range domain.Columns(f.Year, f.Period, f.Scale) {
		left, width := ax.span(c.Start, c.End)
		cv := columnView{Left: left, Width: width, Label: c.Label, Load: domain.ColumnLoad(c, visible, today).Label()}
		if f.Period == domain.PeriodYear && f.Scale == domain.ScaleQuarter {
			first := c.Start.Month()
			cv.Sub = first.String()[:3] + "–" + (first + 2).String()[:3]
		}
		switch {
		case f.Month != 0 && c.OverlapsMonth(f.Year, f.Month):
			cv.Class = "sel"
		case c.IsToday(today):
			cv.Class = "now"
		}
		v.Columns = append(v.Columns, cv)
		if c.Start != from {
			v.Lines = append(v.Lines, lineView{Left: ax.pos(c.Start), Strong: f.Period != domain.PeriodYear || (c.Start.Month()-1)%3 == 0})
		}
	}
	for _, d := range domain.Gridlines(f.Year, f.Period) {
		v.Lines = append(v.Lines, lineView{Left: ax.pos(d)})
		v.Ticks = append(v.Ticks, tickView{Left: ax.pos(d), Label: strconv.Itoa(d.Day())})
	}
	if !today.Before(from) && !today.After(to) {
		v.ShowToday, v.TodayLeft = true, ax.pos(today)
	}
	if f.Month != 0 {
		ms, me := domain.MonthRange(f.Year, f.Month)
		v.HasBand = true
		v.BandLeft, v.BandWidth = ax.span(ms, me)
	}

	for _, g := range groups {
		gv := groupView{Name: g.Project.Name, CountLabel: plural(len(g.Activities), "activity", "activities")}
		var subs []domain.Subtask
		flags := 0
		for _, a := range g.Activities {
			it := itemOf[a.ID]
			subs = append(subs, it.Subtasks...)
			c := creeps[a.ID]
			if c.Flagged {
				flags++
			}
			gv.Rows = append(gv.Rows, buildRow(f, today, it, c))
		}
		gv.PctLabel = strconv.Itoa(domain.ProjectProgress(subs)) + "% done"
		if flags > 0 {
			gv.FlagLabel = plural(flags, "scope flag", "scope flags")
		}
		v.Groups = append(v.Groups, gv)
	}
	return v
}

func buildRow(f Filter, today domain.Date, it domain.ActivityWithSubtasks, c domain.Creep) rowView {
	r := rowView{
		ID: it.ID, Name: it.Name, Href: f.WithSel(it.ID).URL(), Selected: it.ID == f.Sel,
		Status: string(it.Status), StatusLabel: capitalize(string(it.Status)),
		OpenLabel: fmt.Sprintf("%d open / %d", domain.OpenAt(it.Subtasks, today), len(it.Subtasks)),
		Dates:     fmt.Sprintf("%s → %s · %s", it.Start, it.Due, it.RequestedBy),
	}
	if c.Flagged {
		r.ScopeChip = "Scope"
		if c.Added > 0 {
			r.ScopeChip = fmt.Sprintf("Scope +%d", c.Added)
		}
	}
	return r
}
