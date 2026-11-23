package web

import (
	"context"
	"strconv"
	"strings"

	"workline/internal/domain"
)

// feedRow is a journal entry with the activity or project it belongs to.
type feedRow struct {
	journalRow
	Project, Activity string     // Activity is empty for a project's own entry
	ActivityHref      string     // its activity or project on the roadmap; empty when the feed is narrowed to it
	Unassigned        bool       // the entry has neither activity nor project
	FileTo            []optGroup // an entry outside any activity can be filed under these
	DayHeading        string     // with DaySections, set on the first row of each day
}

// optGroup is a labelled group of select options.
type optGroup struct {
	Label   string
	Options []option
}

// fileChoices lists where an entry can be filed: Unassigned, then each live
// project followed by its activities, indented under it, with cur (an
// entryLink) selected. A project is itself a choice, so the list has no
// optgroups: their labels cannot be picked (SPEC §10 item 120).
func fileChoices(groups []domain.Group, cur string) []optGroup {
	opts := []option{{"", "Unassigned", cur == ""}}
	for _, g := range groups {
		p := entryLink(0, g.Project.ID)
		opts = append(opts, option{p, g.Project.Name, p == cur})
		for _, a := range g.Activities {
			v := entryLink(a.ID, 0)
			opts = append(opts, option{v, activityIndent + a.Name, v == cur})
		}
	}
	return []optGroup{{Options: opts}}
}

// activityIndent sets an activity under its project in a select; options
// cannot be styled, so it is non-breaking spaces.
const activityIndent = "\u00a0\u00a0\u00a0"

// entryLink is the form value naming where an entry belongs: an activity's
// id, "p" and a project's id, or "" for Unassigned (SPEC §10 item 120).
func entryLink(activityID, projectID int64) string {
	switch {
	case activityID != 0:
		return strconv.FormatInt(activityID, 10)
	case projectID != 0:
		return "p" + strconv.FormatInt(projectID, 10)
	}
	return ""
}

// parseLink reads an entryLink back.
func parseLink(v string) (activityID, projectID int64, err error) {
	if v == "" {
		return 0, 0, nil
	}
	n, err := strconv.ParseInt(strings.TrimPrefix(v, "p"), 10, 64)
	if err != nil || n <= 0 {
		return 0, 0, domain.Invalid("choose an activity, a project, or none")
	}
	if strings.HasPrefix(v, "p") {
		return 0, n, nil
	}
	return n, 0, nil
}

type journalView struct {
	Next          string
	FullLoadLinks bool // activity links lead to another page, so htmx must not boost them
	DaySections   bool // rows are grouped under day headings and show only their time
	Narrowed      bool // only the selected activity's entries
	Title         string
	ShowAllHref   string
	Entries       []feedRow
	MoreHref      string
	MoreCount     int
}

// buildJournal loads the feed for the given activities and projects, plus the
// Unassigned entries when unassigned is set: newest first, feedStep entries at
// a time (SPEC §10 items 73, 77, 91 and 120). filing lists where an entry can
// be filed.
func (s *Server) buildJournal(ctx context.Context, f Filter, uri string, ids, projectIDs []int64, unassigned bool, selected *panelView, itemOf map[int64]domain.ActivityWithTasks, projects []domain.Project, filing []domain.Group) (journalView, error) {
	entries, more, err := s.store.ListJournalFeed(ctx, ids, projectIDs, unassigned, f.Feed)
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
		e := entries[i]
		fr := feedRow{journalRow: row, Unassigned: e.Unassigned()}
		switch {
		case e.ActivityID != 0:
			a := itemOf[e.ActivityID]
			fr.Project, fr.Activity = projectName[a.ProjectID], a.Name
			if selected == nil {
				fr.ActivityHref = f.WithSel(a.ID).URL()
			}
		case e.ProjectID != 0:
			fr.Project = projectName[e.ProjectID]
			if f.Project != e.ProjectID {
				fr.ActivityHref = f.WithProject(e.ProjectID).URL()
			}
		}
		if e.ActivityID == 0 {
			fr.FileTo = fileChoices(filing, entryLink(0, e.ProjectID))
		}
		v.Entries = append(v.Entries, fr)
	}
	if more && f.Feed < feedMax {
		v.MoreHref, v.MoreCount = f.WithMoreFeed().URL(), feedStep
	}
	return v, nil
}
