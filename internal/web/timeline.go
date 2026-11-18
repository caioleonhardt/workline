package web

import (
	"fmt"
	"strconv"
	"strings"

	"workline/internal/domain"
	"workline/internal/store"
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
	Tip                 string
	SpanLeft, SpanWidth pct
	PctLabel            string
	PctLeft             pct
	HasPlan             bool
	PlanLeft, PlanWidth pct
	Bins                []binView
	Dots                []dotView
}

type binView struct{ Left, Width, Height pct }

type dotView struct {
	Left pct
	Kind string
}

// timelineData is what the dashboard hands to buildTimeline.
type timelineData struct {
	Groups []domain.Group
	ItemOf map[int64]domain.ActivityWithSubtasks
	Creeps map[int64]domain.Creep
	Marks  map[int64][]store.JournalMark
}

// layout is the geometry shared by all rows of one timeline.
type layout struct {
	ax            axis
	from, to      domain.Date
	today         domain.Date
	step, maxOpen int // bin size in days; the tallest weekly open count of any visible activity
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
func buildTimeline(f Filter, today domain.Date, d timelineData) timelineView {
	from, to := domain.PeriodRange(f.Year, f.Period)
	ax := newAxis(from, to)
	l := layout{ax: ax, from: from, to: to, today: today, step: domain.WeekStep}
	if f.Period != domain.PeriodYear {
		l.step = 2
	}
	var visible []domain.ActivityWithSubtasks
	var weekly [][]domain.Bin
	for _, g := range d.Groups {
		for _, a := range g.Activities {
			it := d.ItemOf[a.ID]
			visible = append(visible, it)
			weekly = append(weekly, domain.Burndown(it.Activity, it.Subtasks, domain.WeekStep, today))
		}
	}
	l.maxOpen = domain.GlobalMax(weekly)
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

	for _, g := range d.Groups {
		gv := groupView{Name: g.Project.Name, CountLabel: plural(len(g.Activities), "activity", "activities")}
		var subs []domain.Subtask
		flags := 0
		for _, a := range g.Activities {
			it := d.ItemOf[a.ID]
			subs = append(subs, it.Subtasks...)
			c := d.Creeps[a.ID]
			if c.Flagged {
				flags++
			}
			gv.Rows = append(gv.Rows, buildRow(f, l, it, c, d.Marks[a.ID]))
		}
		gv.PctLabel = strconv.Itoa(domain.ProjectProgress(subs)) + "% done"
		if flags > 0 {
			gv.FlagLabel = plural(flags, "scope flag", "scope flags")
		}
		v.Groups = append(v.Groups, gv)
	}
	return v
}

func buildRow(f Filter, l layout, it domain.ActivityWithSubtasks, c domain.Creep, marks []store.JournalMark) rowView {
	ax, today := l.ax, l.today
	r := rowView{
		ID: it.ID, Name: it.Name, Href: f.WithSel(it.ID).URL(), Selected: it.ID == f.Sel,
		Status: string(it.Status), StatusLabel: capitalize(string(it.Status)),
		OpenLabel: fmt.Sprintf("%d open / %d", domain.OpenAt(it.Subtasks, today), len(it.Subtasks)),
		Dates:     fmt.Sprintf("%s → %s · %s", it.Start, it.Due, it.RequestedBy),
	}
	progress := domain.Progress(it.Status, it.Subtasks)
	r.PctLabel = strconv.Itoa(progress) + "%"
	r.Tip = fmt.Sprintf("%s: %s done, %d open subtasks. Show details", it.Name, r.PctLabel, domain.OpenAt(it.Subtasks, today))
	r.SpanLeft, r.SpanWidth = ax.span(it.Start, it.Due)
	r.SpanWidth = max(r.SpanWidth, 0.6)
	r.PctLeft = min(92, ax.pos(it.Due.AddDays(1)))
	if planFrom := max(it.Start, today); it.Status != domain.StatusDone && !planFrom.After(it.Due) {
		r.PlanLeft, r.PlanWidth = ax.span(planFrom, it.Due)
		r.HasPlan = r.PlanWidth > 0
	}
	for _, b := range domain.ClipBins(domain.Burndown(it.Activity, it.Subtasks, l.step, today), l.from, l.to) {
		left, width := ax.span(b.Start, b.End)
		bv := binView{Left: left, Width: max(width, 0.15)}
		if l.maxOpen > 0 {
			bv.Height = pct(float64(b.Open) / float64(l.maxOpen) * 100)
		}
		r.Bins = append(r.Bins, bv)
	}
	for _, m := range marks {
		if !m.Date.Before(l.from) && !m.Date.After(l.to) {
			r.Dots = append(r.Dots, dotView{Left: ax.pos(m.Date), Kind: string(m.Kind)})
		}
	}
	if c.Flagged {
		r.ScopeChip = "Scope"
		if c.Added > 0 {
			r.ScopeChip = fmt.Sprintf("Scope +%d", c.Added)
		}
	}
	return r
}
