package domain

// Bin is one burndown bucket covering the inclusive days Start..End.
// Open and Scope are measured at the bin's last counted day. Ahead bins lie
// entirely after today and carry no values.
type Bin struct {
	Start, End  Date
	Open, Scope int
	Ahead       bool
}

// WeekStep is the bin size, in days, of weekly burndowns.
const WeekStep = 7

// maxSpanDays bounds the work done for one activity.
const maxSpanDays = 3660

func checkActivity(a Activity) {
	assert(!a.Start.IsZero() && !a.Due.IsZero() && !a.Due.Before(a.Start), "activity needs start <= due")
	assert(a.Due.Sub(a.Start) <= maxSpanDays, "activity span too long")
}

// Burndown returns the timeline series: bins of step days from a.Start to
// min(a.Due, today, a.DoneOn) (§4.3, SPEC §10 item 45). It is empty when the
// activity starts after that end.
func Burndown(a Activity, subs []Task, step int, today Date) []Bin {
	assert(step > 0, "step must be positive")
	checkActivity(a)
	end := a.Due
	if today.Before(end) {
		end = today
	}
	if !a.DoneOn.IsZero() && a.DoneOn.Before(end) {
		end = a.DoneOn
	}
	var bins []Bin
	for start := a.Start; !start.After(end); start = start.AddDays(step) {
		last := start.AddDays(step - 1)
		if last.After(end) {
			last = end
		}
		bins = append(bins, Bin{Start: start, End: last, Open: OpenAt(subs, last), Scope: ScopeAt(subs, last)})
	}
	return bins
}

// BurndownDetail returns the detail-panel series: weekly bins over the full
// a.Start..a.Due span. Bins starting after today are Ahead. A bin that
// straddles today is measured at today.
func BurndownDetail(a Activity, subs []Task, today Date) []Bin {
	checkActivity(a)
	var bins []Bin
	for start := a.Start; !start.After(a.Due); start = start.AddDays(WeekStep) {
		last := start.AddDays(WeekStep - 1)
		if last.After(a.Due) {
			last = a.Due
		}
		b := Bin{Start: start, End: last}
		if start.After(today) {
			b.Ahead = true
		} else {
			at := last
			if today.Before(at) {
				at = today
			}
			b.Open, b.Scope = OpenAt(subs, at), ScopeAt(subs, at)
		}
		bins = append(bins, b)
	}
	return bins
}

// ClipBins drops bins outside from..to and trims the ones crossing an edge.
// Values are kept as measured.
func ClipBins(bins []Bin, from, to Date) []Bin {
	assert(!to.Before(from), "clip window is inverted")
	var out []Bin
	for _, b := range bins {
		if b.End.Before(from) || b.Start.After(to) {
			continue
		}
		if b.Start.Before(from) {
			b.Start = from
		}
		if b.End.After(to) {
			b.End = to
		}
		out = append(out, b)
	}
	return out
}

// GlobalMax is the highest Open reached by any non-ahead bin of any series.
// It is 0 for no data; callers must guard the division when scaling.
func GlobalMax(weekly [][]Bin) int {
	m := 0
	for _, bins := range weekly {
		for _, b := range bins {
			if !b.Ahead && b.Open > m {
				m = b.Open
			}
		}
	}
	return m
}
