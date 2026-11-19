package web

import (
	"context"
	"strconv"

	"workline/internal/domain"
)

// feedRow is a journal entry with the activity it belongs to.
type feedRow struct {
	journalRow
	Project, Activity string
	ActivityHref      string     // empty when the feed is already narrowed to the activity
	Unassigned        bool       // the entry has no activity yet
	FileTo            []optGroup // an Unassigned entry's Edit form offers these activities
}

// optGroup is a labelled group of select options.
type optGroup struct {
	Label   string
	Options []option
}

// fileChoices lists where an entry can be filed: Unassigned, then the live
// activities by project, with cur selected.
func fileChoices(groups []domain.Group, cur int64) []optGroup {
	out := []optGroup{{Options: []option{{"", "Unassigned", cur == 0}}}}
	for _, g := range groups {
		og := optGroup{Label: g.Project.Name}
		for _, a := range g.Activities {
			og.Options = append(og.Options, option{strconv.FormatInt(a.ID, 10), a.Name, a.ID == cur})
		}
		out = append(out, og)
	}
	return out
}

type journalView struct {
	Next          string
	FullLoadLinks bool // activity links lead to another page, so htmx must not boost them
	Narrowed      bool // only the selected activity's entries
	Title         string
	ShowAllHref   string
	Entries       []feedRow
	MoreHref      string
	MoreCount     int
}

// buildJournal loads the feed for the given activities, plus the Unassigned
// entries when unassigned is set: newest first, feedStep entries at a time
// (SPEC §10 items 73, 77 and 91). filing lists the activities an entry can be
// filed under.
func (s *Server) buildJournal(ctx context.Context, f Filter, uri string, ids []int64, unassigned bool, selected *panelView, itemOf map[int64]domain.ActivityWithTasks, projects []domain.Project, filing []domain.Group) (journalView, error) {
	entries, more, err := s.store.ListJournalFeed(ctx, ids, unassigned, f.Feed)
	if err != nil {
		return journalView{}, err
	}
	projectName := map[int64]string{}
	for _, p := range projects {
		projectName[p.ID] = p.Name
	}
	v := journalView{Next: uri, Narrowed: selected != nil}
	if selected != nil {
		v.Title, v.ShowAllHref = selected.Name, f.WithSel(0).URL()
	}
	for i, row := range journalRows(entries) {
		a := itemOf[entries[i].ActivityID]
		fr := feedRow{journalRow: row, Project: projectName[a.ProjectID], Activity: a.Name, Unassigned: entries[i].ActivityID == 0}
		if selected == nil && !fr.Unassigned {
			fr.ActivityHref = f.WithSel(a.ID).URL()
		}
		if fr.Unassigned {
			fr.FileTo = fileChoices(filing, 0)
		}
		v.Entries = append(v.Entries, fr)
	}
	if more && f.Feed < feedMax {
		v.MoreHref, v.MoreCount = f.WithMoreFeed().URL(), feedStep
	}
	return v, nil
}
