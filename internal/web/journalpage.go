package web

import (
	"context"
	"net/http"
	"net/url"
	"slices"
	"strconv"

	"workline/internal/domain"
)

type journalPageView struct {
	quickBar
	Title, Next                   string
	Day                           string
	PrevHref, NextHref, TodayHref string // TodayHref is empty when today is shown
	ExportHref, ReportHref        string
	Journal                       journalView
}

// journalDay is the day named by the query's `date`; today when it is missing or bad.
func journalDay(q url.Values, today domain.Date) domain.Date {
	if d, err := domain.ParseDate(q.Get("date")); err == nil {
		return d
	}
	return today
}

// journalDayURL is the journal page of day.
func journalDayURL(day, today domain.Date) string {
	if day == today {
		return "/"
	}
	return "/?date=" + day.String()
}

// isJournalPage reports whether the address is the journal page: / with at
// most a `date` parameter (or an old `week`, which is ignored). Any other
// query on / is timeline state.
func isJournalPage(u *url.URL) bool {
	if u.Path != "/" {
		return false
	}
	for k := range u.Query() {
		if k != "date" && k != "week" {
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

// buildJournalPage builds the home page: the journal of one day (SPEC §10 item 124).
func (s *Server) buildJournalPage(ctx context.Context, uri string) (journalPageView, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return journalPageView{}, domain.Invalid("bad journal address")
	}
	today := s.clock.Today()
	day := journalDay(u.Query(), today)
	projects, err := s.store.ListProjects(ctx, false)
	if err != nil {
		return journalPageView{}, err
	}
	acts, err := s.store.ListActivities(ctx)
	if err != nil {
		return journalPageView{}, err
	}
	entries, err := s.store.ListJournalBetween(ctx, day, day)
	if err != nil {
		return journalPageView{}, err
	}
	projectName, actOf, filing := liveIndex(projects, acts)
	v := journalPageView{
		Title: "Journal · Workline", Next: uri, Day: day.String(),
		ExportHref: "/export/journal.md?" + url.Values{"from": {day.String()}, "to": {day.String()}}.Encode(),
		ReportHref: "/report",
		PrevHref:   journalDayURL(day.AddDays(-1), today),
		NextHref:   journalDayURL(day.AddDays(1), today),
		quickBar:   newQuickBar(quickAddView{Pick: true, Next: uri}),
	}
	if day != today {
		v.TodayHref = "/"
		v.QuickAdd.Date = day.String()
	}
	v.QuickFile = fileChoices(filing, 0)

	// The feed: the day's entries of live projects and Unassigned ones, newest first.
	entries = slices.DeleteFunc(entries, func(j domain.JournalEntry) bool {
		_, ok := actOf[j.ActivityID]
		return j.ActivityID != 0 && !ok
	})
	slices.Reverse(entries)
	v.Journal = journalView{Next: uri, Title: "Journal", FullLoadLinks: true, Entries: pageFeedRows(entries, projectName, actOf, filing)}
	return v, nil
}

// pageFeedRows builds feed rows for a page other than the timeline: each
// activity tag is a link to the timeline with it selected.
func pageFeedRows(entries []domain.JournalEntry, projectName map[int64]string, actOf map[int64]domain.Activity, filing []domain.Group) []feedRow {
	var out []feedRow
	for i, row := range journalRows(entries) {
		a := actOf[entries[i].ActivityID]
		fr := feedRow{journalRow: row, Project: projectName[a.ProjectID], Activity: a.Name, Unassigned: entries[i].ActivityID == 0}
		if fr.Unassigned {
			fr.FileTo = fileChoices(filing, 0)
		} else {
			fr.ActivityHref = "/timeline?sel=" + strconv.FormatInt(a.ID, 10)
		}
		out = append(out, fr)
	}
	return out
}

// liveIndex indexes the activities of live projects (archived ones are hidden
// everywhere, SPEC §10 item 33): project names by id, activities by id, and the
// groups an Unassigned entry can be filed under. projects must exclude archived ones.
func liveIndex(projects []domain.Project, acts []domain.Activity) (map[int64]string, map[int64]domain.Activity, []domain.Group) {
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
	return projectName, actOf, filing
}
