package web

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"time"

	"workline/internal/domain"
)

// The journal page shows two weeks, Monday to Sunday: the week named by
// `week` (any day in it; default this week) and the week before (SPEC §10 item 95).
const journalWeeks = 2

// stripView is the journal page's one-row burndown of all tasks.
type stripView struct {
	Columns   []columnView
	Lines     []lineView
	ShowToday bool
	TodayLeft pct
	Bins      []binView
	Done      []doneDot
	OpenLabel string
}

// openTaskRow is a task in the journal page's priority list.
type openTaskRow struct {
	ID                      int64
	Text, Created           string
	Priority, PriorityLabel string
	Project, Activity       string
	ActivityHref            string // empty for Unassigned
}

type journalPageView struct {
	quickBar
	Title, Next                      string
	RangeLabel                       string
	PrevHref, NextHref, ThisWeekHref string // ThisWeekHref is empty when this week is shown
	ExportHref                       string
	Tiles                            []tileView
	Strip                            stripView
	Tasks                            []openTaskRow
	Journal                          journalView
}

// journalRange returns the first and last day shown for the query's `week`.
func journalRange(q url.Values, today domain.Date) (from, to domain.Date) {
	week := domain.WeekStart(today)
	if d, err := domain.ParseDate(q.Get("week")); err == nil {
		week = domain.WeekStart(d)
	}
	return week.AddDays(-7 * (journalWeeks - 1)), week.AddDays(6)
}

// journalPageURL is the journal page showing the weeks up to the one starting on week.
func journalPageURL(week, today domain.Date) string {
	if week == domain.WeekStart(today) {
		return "/"
	}
	return "/?week=" + week.String()
}

// isJournalPage reports whether the address is the journal page: / with at
// most a `week` parameter. Any other query on / is timeline state.
func isJournalPage(u *url.URL) bool {
	if u.Path != "/" {
		return false
	}
	for k := range u.Query() {
		if k != "week" {
			return false
		}
	}
	return true
}

// home serves the journal page, or sends an old timeline address on / to /timeline.
func (s *Server) home(w http.ResponseWriter, r *http.Request) {
	if !isJournalPage(r.URL) {
		http.Redirect(w, r, timelineAddress(r.URL.RequestURI()), http.StatusFound)
		return
	}
	s.renderJournalPage(w, r, r.URL.RequestURI())
}

func (s *Server) renderJournalPage(w http.ResponseWriter, r *http.Request, uri string, mods ...func(*quickBar)) {
	v, err := s.buildJournalPage(r.Context(), uri)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	for _, mod := range mods {
		mod(&v.quickBar)
	}
	w.Header().Set("Vary", "HX-Request")
	name := "journalpage.html"
	if isHX(r) {
		name = "jdash-fragment"
	}
	s.render(w, http.StatusOK, name, v)
}

func (s *Server) buildJournalPage(ctx context.Context, uri string) (journalPageView, error) {
	today := s.clock.Today()
	u, err := url.Parse(uri)
	if err != nil {
		return journalPageView{}, domain.Invalid("bad journal address")
	}
	from, to := journalRange(u.Query(), today)
	projects, err := s.store.ListProjects(ctx, false)
	if err != nil {
		return journalPageView{}, err
	}
	acts, err := s.store.ListActivities(ctx)
	if err != nil {
		return journalPageView{}, err
	}
	allTasks, err := s.store.ListAllTasks(ctx)
	if err != nil {
		return journalPageView{}, err
	}
	entries, err := s.store.ListJournalBetween(ctx, from, to)
	if err != nil {
		return journalPageView{}, err
	}

	// Archived projects are hidden everywhere (SPEC §10 item 33).
	projectName := map[int64]string{}
	for _, p := range projects {
		projectName[p.ID] = p.Name
	}
	actOf := map[int64]domain.Activity{}
	liveOf := map[int64][]domain.Activity{}
	for _, a := range acts {
		if _, live := projectName[a.ProjectID]; live {
			actOf[a.ID] = a
			liveOf[a.ProjectID] = append(liveOf[a.ProjectID], a)
		}
	}
	var filing []domain.Group
	for _, p := range projects {
		if as := liveOf[p.ID]; len(as) > 0 {
			filing = append(filing, domain.Group{Project: p, Activities: as})
		}
	}
	shown := func(activityID int64) bool {
		_, ok := actOf[activityID]
		return activityID == 0 || ok
	}
	var tasks []domain.Task
	for _, t := range allTasks {
		if shown(t.ActivityID) {
			tasks = append(tasks, t)
		}
	}

	week := domain.WeekStart(to)
	v := journalPageView{
		Title: "Journal · Workline", Next: uri,
		RangeLabel: fmt.Sprintf("%s – %s", from, to),
		PrevHref:   journalPageURL(week.AddDays(-7), today),
		NextHref:   journalPageURL(week.AddDays(7), today),
		ExportHref: "/export/journal.md?" + url.Values{"from": {from.String()}, "to": {to.String()}}.Encode(),
		Tiles:      journalTiles(tasks, from, to, today),
		Strip:      buildStrip(tasks, from, to, today),
		quickBar:   newQuickBar(quickAddView{Pick: true, Next: uri}),
	}
	v.QuickFile = fileChoices(filing, 0)
	if week != domain.WeekStart(today) {
		v.ThisWeekHref = "/"
	}

	open := slices.DeleteFunc(slices.Clone(tasks), func(t domain.Task) bool { return !t.ClosedOn.IsZero() })
	domain.SortTasks(open)
	for _, t := range open {
		row := openTaskRow{
			ID: t.ID, Text: t.Text, Created: t.CreatedOn.String(),
			Priority: string(t.Priority), PriorityLabel: priorityLabel(t.Priority),
		}
		if a, ok := actOf[t.ActivityID]; ok {
			row.Project, row.Activity = projectName[a.ProjectID], a.Name
			row.ActivityHref = "/timeline?sel=" + strconv.FormatInt(a.ID, 10)
		}
		v.Tasks = append(v.Tasks, row)
	}

	// The feed: entries dated in the range, newest first.
	entries = slices.DeleteFunc(entries, func(j domain.JournalEntry) bool { return !shown(j.ActivityID) })
	slices.Reverse(entries)
	v.Journal = journalView{Next: uri, Title: "Journal", FullLoadLinks: true}
	for i, row := range journalRows(entries) {
		a := actOf[entries[i].ActivityID]
		fr := feedRow{journalRow: row, Project: projectName[a.ProjectID], Activity: a.Name, Unassigned: entries[i].ActivityID == 0}
		if fr.Unassigned {
			fr.FileTo = fileChoices(filing, 0)
		} else {
			fr.ActivityHref = "/timeline?sel=" + strconv.FormatInt(a.ID, 10)
		}
		v.Journal.Entries = append(v.Journal.Entries, fr)
	}
	return v, nil
}

func journalTiles(tasks []domain.Task, from, to, today domain.Date) []tileView {
	var opened, closed, high int
	in := func(d domain.Date) bool { return !d.IsZero() && !d.Before(from) && !d.After(to) }
	for _, t := range tasks {
		if in(t.CreatedOn) {
			opened++
		}
		if in(t.ClosedOn) {
			closed++
		}
		if t.ClosedOn.IsZero() && t.Priority == domain.PriorityHigh {
			high++
		}
	}
	return []tileView{
		{"Tasks opened", "", opened}, {"Tasks closed", "done", closed},
		{"Open tasks now", "", domain.OpenAt(tasks, today)}, {"High priority open", "blocked", high},
	}
}

// buildStrip draws all tasks as one timeline row over from..to: daily bins of
// open tasks up to today, and a dot per day with completions.
func buildStrip(tasks []domain.Task, from, to, today domain.Date) stripView {
	ax := newAxis(from, to)
	v := stripView{OpenLabel: fmt.Sprintf("%d open / %d", domain.OpenAt(tasks, today), domain.ScopeAt(tasks, today))}
	for d := from; !d.After(to); d = d.AddDays(1) {
		left, width := ax.span(d, d)
		c := columnView{Left: left, Width: width, Label: d.Weekday().String()[:3], Sub: strconv.Itoa(d.Day())}
		if d == today {
			c.Class = "now"
		}
		v.Columns = append(v.Columns, c)
		if d != from {
			v.Lines = append(v.Lines, lineView{Left: left, Strong: d.Weekday() == time.Monday})
		}
	}
	if !today.Before(from) && !today.After(to) {
		left, width := ax.span(today, today)
		v.ShowToday, v.TodayLeft = true, left+width/2
	}
	if !from.After(today) {
		bins := domain.BurndownRange(tasks, from, min(to, today), 1)
		peak := 0
		for _, b := range bins {
			peak = max(peak, b.Open)
		}
		for _, b := range bins {
			left, width := ax.span(b.Start, b.End)
			bv := binView{Left: left, Width: width}
			if peak > 0 {
				bv.Height = pct(float64(b.Open) / float64(peak) * 100)
			}
			v.Bins = append(v.Bins, bv)
		}
	}
	v.Done = completionDots(ax, from, to, "all", tasks, true)
	_, day := ax.span(from, from)
	for i := range v.Done {
		v.Done[i].Left += day / 2 // a day is a whole column here: centre the dot in it
	}
	return v
}
