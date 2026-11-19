package web

import (
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"workline/internal/domain"
	"workline/internal/store"
)

type exportActivity struct {
	domain.Activity
	Entries []domain.JournalEntry
}

type exportProject struct {
	Name       string
	Activities []exportActivity
}

// exportMarkdown renders the journal export (SPEC §10 items 66 and 91). from
// and to are zero when unbounded; scope names the project or "all projects".
// The Unassigned entries come last.
func exportMarkdown(scope string, from, to, generated domain.Date, projects []exportProject, unassigned []domain.JournalEntry) string {
	var b strings.Builder
	rng := "all dates"
	switch {
	case !from.IsZero() && !to.IsZero():
		rng = from.String() + " to " + to.String()
	case !from.IsZero():
		rng = "from " + from.String()
	case !to.IsZero():
		rng = "until " + to.String()
	}
	fmt.Fprintf(&b, "# Workline journal\n%s · %s · generated %s\n", rng, scope, generated)
	if len(projects) == 0 && len(unassigned) == 0 {
		b.WriteString("\nNo journal entries in this range.\n")
	}
	for _, p := range projects {
		fmt.Fprintf(&b, "\n## %s\n", p.Name)
		for _, a := range p.Activities {
			fmt.Fprintf(&b, "\n### %s\n%s · requested by %s · %s → %s\n\n",
				a.Name, capitalize(string(a.Status)), orDash(a.RequestedBy), a.Start, a.Due)
			writeEntries(&b, a.Entries)
		}
	}
	if len(unassigned) > 0 {
		b.WriteString("\n## Unassigned\n\n")
		writeEntries(&b, unassigned)
	}
	return b.String()
}

func writeEntries(b *strings.Builder, entries []domain.JournalEntry) {
	for _, e := range entries {
		when := e.Date.String()
		if t := e.Time(); t != "" {
			when += " " + t
		}
		text := strings.ReplaceAll(strings.TrimRight(e.Text, "\n"), "\n", "\n  ")
		if !e.ClosedOn.IsZero() {
			text += " (closed " + e.ClosedOn.String() + ")"
		}
		fmt.Fprintf(b, "- %s **%s** — %s\n", when, kindLabel(e.Kind), text)
	}
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

func queryDate(r *http.Request, key string) (domain.Date, error) {
	v := r.URL.Query().Get(key)
	if v == "" {
		return 0, nil
	}
	d, err := domain.ParseDate(v)
	if err != nil {
		return 0, domain.Invalid("%s must be a date (YYYY-MM-DD), not %q", key, v)
	}
	return d, nil
}

func (s *Server) exportJournal(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	from, err := queryDate(r, "from")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	to, err := queryDate(r, "to")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	projects, err := s.store.ListProjects(ctx, true)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	scope, allProjects := "all projects", true
	if v := r.URL.Query().Get("project"); v != "" {
		id, perr := strconv.ParseInt(v, 10, 64)
		if perr != nil {
			s.fail(w, r, domain.Invalid("project must be a project id, not %q", v))
			return
		}
		i := slices.IndexFunc(projects, func(p domain.Project) bool { return p.ID == id })
		if i < 0 {
			s.fail(w, r, fmt.Errorf("project %d: %w", id, store.ErrNotFound))
			return
		}
		projects, scope, allProjects = projects[i:i+1], projects[i].Name, false
	} else {
		projects = slices.DeleteFunc(projects, func(p domain.Project) bool { return p.Archived })
	}
	acts, err := s.store.ListActivities(ctx)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	entries, err := s.store.ListJournalBetween(ctx, from, to)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	byActivity := map[int64][]domain.JournalEntry{}
	for _, e := range entries {
		byActivity[e.ActivityID] = append(byActivity[e.ActivityID], e)
	}
	slices.SortStableFunc(projects, func(a, b domain.Project) int {
		return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	})
	slices.SortStableFunc(acts, func(a, b domain.Activity) int {
		if a.Start != b.Start {
			return int(a.Start - b.Start)
		}
		return int(a.ID - b.ID)
	})
	var out []exportProject
	for _, p := range projects {
		ep := exportProject{Name: p.Name}
		for _, a := range acts {
			if a.ProjectID == p.ID && len(byActivity[a.ID]) > 0 {
				ep.Activities = append(ep.Activities, exportActivity{Activity: a, Entries: byActivity[a.ID]})
			}
		}
		if len(ep.Activities) > 0 {
			out = append(out, ep)
		}
	}
	today := s.clock.Today()
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="workline-journal-%s.md"`, today))
	var unassigned []domain.JournalEntry
	if allProjects {
		unassigned = byActivity[0]
	}
	fmt.Fprint(w, exportMarkdown(scope, from, to, today, out, unassigned))
}
