package web

import (
	"cmp"
	"fmt"
	"slices"
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

// barView is an activity on the roadmap: a bar in its project's lane.
type barView struct {
	ID                  int64
	Name, Href          string
	Selected            bool
	Status, StatusLabel string
	ScopeChip           string // empty unless flagged
	Tip                 string
	Left, Width         pct
	Top                 int  // px from the top of the lane
	ClipStart, ClipEnd  bool // the span runs past the period's start or end
	Progress            int  // % of tasks done, drawn as the bar's fill
	PctLabel            string
}

// The bars of a project are packed into as few lines as they fit in.
const (
	barLine = 34 // px per line of bars
	barPad  = 8  // px above the first line and below the last
)

// timelineData is what the dashboard hands to buildTimeline.
type timelineData struct {
	Groups []domain.Group
	ItemOf map[int64]domain.ActivityWithTasks
	Creeps map[int64]domain.Creep
}

type groupView struct {
	ID                                    int64
	Name, CountLabel, PctLabel, FlagLabel string
	Height                                int // px: all its lines of bars
	Bars                                  []barView
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

// buildTimeline lays out the visible groups on the period's track: one bar
// per activity from start to due, clipped to the period (SPEC §10 item 125).
func buildTimeline(f Filter, today domain.Date, d timelineData) timelineView {
	from, to := domain.PeriodRange(f.Year, f.Period)
	ax := newAxis(from, to)
	var visible []domain.ActivityWithTasks
	for _, g := range d.Groups {
		for _, a := range g.Activities {
			visible = append(visible, d.ItemOf[a.ID])
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

	for _, g := range d.Groups {
		gv := groupView{ID: g.Project.ID, Name: g.Project.Name, CountLabel: plural(len(g.Activities), "activity", "activities")}
		// Bars are packed by start, so the lane needs as few lines as possible.
		acts := slices.Clone(g.Activities)
		slices.SortStableFunc(acts, func(a, b domain.Activity) int { return cmp.Compare(max(a.Start, from), max(b.Start, from)) })
		spans := make([]domain.Span, len(acts))
		for i, a := range acts {
			spans[i] = domain.Span{From: max(a.Start, from), To: min(a.Due, to)}
		}
		lanes, n := domain.Lanes(spans)
		gv.Height = max(n, 1)*barLine + 2*barPad
		var subs []domain.Task
		flags := 0
		for i, a := range acts {
			it := d.ItemOf[a.ID]
			subs = append(subs, it.Tasks...)
			c := d.Creeps[a.ID]
			if c.Flagged {
				flags++
			}
			b := buildBar(f, ax, from, to, today, it, c)
			b.Top = barPad + lanes[i]*barLine
			gv.Bars = append(gv.Bars, b)
		}
		gv.PctLabel = strconv.Itoa(domain.ProjectProgress(subs)) + "% done"
		if flags > 0 {
			gv.FlagLabel = plural(flags, "scope flag", "scope flags")
		}
		v.Groups = append(v.Groups, gv)
	}
	return v
}

func buildBar(f Filter, ax axis, from, to, today domain.Date, it domain.ActivityWithTasks, c domain.Creep) barView {
	b := barView{
		ID: it.ID, Name: it.Name, Href: f.WithSel(it.ID).URL(), Selected: it.ID == f.Sel,
		Status: string(it.Status), StatusLabel: capitalize(string(it.Status)),
		Progress:  domain.Progress(it.Status, it.Tasks),
		ClipStart: it.Start.Before(from), ClipEnd: it.Due.After(to),
	}
	b.PctLabel = strconv.Itoa(b.Progress) + "%"
	b.Left, b.Width = ax.span(max(it.Start, from), min(it.Due, to))
	b.Width = max(b.Width, 0.6)
	if c.Flagged {
		b.ScopeChip = "Scope"
		if c.Added > 0 {
			b.ScopeChip = fmt.Sprintf("Scope +%d", c.Added)
		}
	}
	b.Tip = fmt.Sprintf("%s · %s · %s → %s", it.Name, b.StatusLabel, it.Start, it.Due)
	if it.RequestedBy != "" {
		b.Tip += " · requested by " + it.RequestedBy
	}
	b.Tip += fmt.Sprintf(" · %s done, %d open of %d tasks", b.PctLabel, domain.OpenAt(it.Tasks, today), len(it.Tasks))
	if b.ScopeChip != "" {
		b.Tip += " · " + b.ScopeChip
	}
	return b
}
