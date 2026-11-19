package web

import (
	"context"

	"workline/internal/domain"
)

// feedRow is a journal entry with the activity it belongs to.
type feedRow struct {
	journalRow
	Project, Activity string
	ActivityHref      string // empty when the feed is already narrowed to the activity
}

type journalView struct {
	Next        string
	Narrowed    bool // only the selected activity's entries
	Title       string
	ShowAllHref string
	Entries     []feedRow
	MoreHref    string
	MoreCount   int
}

// buildJournal loads the feed for the given activities: newest first, feedStep
// entries at a time (SPEC §10 items 73 and 77).
func (s *Server) buildJournal(ctx context.Context, f Filter, uri string, ids []int64, selected *panelView, itemOf map[int64]domain.ActivityWithSubtasks, projects []domain.Project) (journalView, error) {
	entries, more, err := s.store.ListJournalFeed(ctx, ids, f.Feed)
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
		fr := feedRow{journalRow: row, Project: projectName[a.ProjectID], Activity: a.Name}
		if selected == nil {
			fr.ActivityHref = f.WithSel(a.ID).URL()
		}
		v.Entries = append(v.Entries, fr)
	}
	if more && f.Feed < feedMax {
		v.MoreHref, v.MoreCount = f.WithMoreFeed().URL(), feedStep
	}
	return v, nil
}
