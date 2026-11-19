package web

import (
	"net/url"
	"slices"
	"strconv"
	"time"

	"workline/internal/domain"
)

// The journal feed shows feedStep entries and grows by that much, up to feedMax.
const (
	feedStep = 30
	feedMax  = 600
)

var allStatuses = []domain.Status{domain.StatusPlanned, domain.StatusActive, domain.StatusBlocked, domain.StatusDone}

// Filter is the dashboard state carried in the URL query string (SPEC §5.1).
type Filter struct {
	Year     int
	Period   domain.Period
	Scale    domain.Scale
	Project  int64      // 0 = all projects
	Month    time.Month // 0 = whole period
	Statuses []domain.Status
	Creep    bool
	Sel      int64
	New      bool
	Feed     int // journal entries shown

	defaultYear int
}

func defaultStatuses() []domain.Status { return allStatuses[:3] }

var periodNames = []string{"year", "q1", "q2", "q3", "q4"}

// parseFilter reads the query string; invalid values fall back to defaults.
func parseFilter(q url.Values, today domain.Date) Filter {
	f := Filter{Year: today.Year(), defaultYear: today.Year(), Statuses: defaultStatuses(), Feed: feedStep}
	if y, err := strconv.Atoi(q.Get("year")); err == nil && y >= 2000 && y <= 2100 {
		f.Year = y
	}
	if i := slices.Index(periodNames, q.Get("period")); i > 0 {
		f.Period = domain.Period(i)
	}
	if f.Period == domain.PeriodYear && q.Get("scale") == "month" {
		f.Scale = domain.ScaleMonth
	}
	f.Project, _ = strconv.ParseInt(q.Get("project"), 10, 64)
	if m, err := strconv.Atoi(q.Get("month")); err == nil && f.monthAllowed(time.Month(m)) {
		f.Month = time.Month(m)
	}
	if q.Has("status") {
		f.Statuses = nil
		for _, s := range allStatuses {
			if slices.Contains(q["status"], string(s)) {
				f.Statuses = append(f.Statuses, s)
			}
		}
	}
	f.Creep = q.Get("creep") == "1"
	f.Sel, _ = strconv.ParseInt(q.Get("sel"), 10, 64)
	f.New = q.Get("new") == "1"
	if n, err := strconv.Atoi(q.Get("feed")); err == nil && n >= feedStep && n <= feedMax {
		f.Feed = n
	}
	return f
}

// monthAllowed reports whether m lies in the selected period.
func (f Filter) monthAllowed(m time.Month) bool {
	if m < time.January || m > time.December {
		return false
	}
	if f.Period == domain.PeriodYear {
		return true
	}
	first := time.Month(3*(int(f.Period)-1) + 1)
	return m >= first && m < first+3
}

// Values returns the canonical query: defaults are omitted.
func (f Filter) Values() url.Values {
	q := url.Values{}
	if f.Year != f.defaultYear {
		q.Set("year", strconv.Itoa(f.Year))
	}
	if f.Period != domain.PeriodYear {
		q.Set("period", periodNames[f.Period])
	}
	if f.Scale == domain.ScaleMonth {
		q.Set("scale", "month")
	}
	if f.Project != 0 {
		q.Set("project", strconv.FormatInt(f.Project, 10))
	}
	if f.Month != 0 {
		q.Set("month", strconv.Itoa(int(f.Month)))
	}
	if !slices.Equal(f.Statuses, defaultStatuses()) {
		q["status"] = []string{}
		for _, s := range f.Statuses {
			q.Add("status", string(s))
		}
		if len(f.Statuses) == 0 {
			q.Set("status", "")
		}
	}
	if f.Creep {
		q.Set("creep", "1")
	}
	if f.Sel != 0 {
		q.Set("sel", strconv.FormatInt(f.Sel, 10))
	}
	if f.New {
		q.Set("new", "1")
	}
	if f.Feed != feedStep {
		q.Set("feed", strconv.Itoa(f.Feed))
	}
	return q
}

// URL returns the timeline path with the canonical query.
func (f Filter) URL() string {
	if enc := f.Values().Encode(); enc != "" {
		return "/timeline?" + enc
	}
	return "/timeline"
}

// WithSel returns f with the selected activity set (0 clears it).
func (f Filter) WithSel(id int64) Filter {
	f.Sel = id
	return f
}

// inScope applies the period, project and month filters (not status or creep).
func (f Filter) inScope(a domain.Activity) bool {
	if f.Project != 0 && a.ProjectID != f.Project {
		return false
	}
	from, to := domain.PeriodRange(f.Year, f.Period)
	if !a.Overlaps(from, to) {
		return false
	}
	if f.Month != 0 {
		ms, me := domain.MonthRange(f.Year, f.Month)
		return a.Overlaps(ms, me)
	}
	return true
}

// shows applies the status and creep filters.
func (f Filter) shows(st domain.Status, flagged bool) bool {
	return slices.Contains(f.Statuses, st) && (!f.Creep || flagged)
}

// WithYear returns f for another year; a month stays, the selection stays.
func (f Filter) WithYear(y int) Filter {
	f.Year = y
	return f
}

// WithPeriod returns f for another period; the month is cleared (SPEC §5.1).
func (f Filter) WithPeriod(p domain.Period) Filter {
	f.Period, f.Month = p, 0
	return f
}

// WithScale returns f with the Year view's column scale changed.
func (f Filter) WithScale(sc domain.Scale) Filter {
	f.Scale = sc
	return f
}

// ToggleStatus returns f with one status pill flipped.
func (f Filter) ToggleStatus(st domain.Status) Filter {
	var next []domain.Status
	for _, s := range allStatuses {
		if slices.Contains(f.Statuses, s) != (s == st) {
			next = append(next, s)
		}
	}
	f.Statuses = next
	return f
}

// ToggleCreep returns f with the scope-creep filter flipped.
func (f Filter) ToggleCreep() Filter {
	f.Creep = !f.Creep
	return f
}

// Filtered reports whether any filter that "Clear filters" resets is active.
func (f Filter) Filtered() bool {
	return f.Project != 0 || f.Month != 0 || f.Creep || !slices.Equal(f.Statuses, defaultStatuses())
}

// Cleared resets project, month, statuses and creep; year, period, scale and
// selection stay (SPEC §10 item 42).
func (f Filter) Cleared() Filter {
	f.Project, f.Month, f.Creep, f.Statuses = 0, 0, false, defaultStatuses()
	return f
}

// ExportURL is the journal export for the dashboard's project and the selected
// month, or else the period (SPEC §10 item 66).
func (f Filter) ExportURL() string {
	from, to := domain.PeriodRange(f.Year, f.Period)
	if f.Month != 0 {
		from, to = domain.MonthRange(f.Year, f.Month)
	}
	q := url.Values{"from": {from.String()}, "to": {to.String()}}
	if f.Project != 0 {
		q.Set("project", strconv.FormatInt(f.Project, 10))
	}
	return "/export/journal.md?" + q.Encode()
}

// WithMoreFeed returns f showing feedStep more journal entries.
func (f Filter) WithMoreFeed() Filter {
	f.Feed = min(f.Feed+feedStep, feedMax)
	return f
}
