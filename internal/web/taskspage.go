package web

import (
	"cmp"
	"context"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"

	"workline/internal/domain"
)

// tasksPerPage is the length of a page of /tasks (SPEC §10 item 123).
const tasksPerPage = 50

// taskListRow is a task on the tasks page.
type taskListRow struct {
	ID                      int64
	Text, Created, Closed   string // Closed is empty while open
	Priority, PriorityLabel string
	Project, Activity       string // Activity is empty for a project's own task
	ActivityHref            string // empty for Unassigned
}

type tasksPageView struct {
	quickBar
	Title, Next        string
	States, Projects   []option
	State, Project     string
	Count              string
	Tasks              []taskListRow
	PrevHref, NextHref string // empty on the first or last page
}

// tasksQuery reads the tasks page's state (open, closed or all; default
// open), project (0 for all) and page (from 1; anything else is 1).
func tasksQuery(q url.Values) (state string, project int64, page int) {
	state = q.Get("state")
	if state != "closed" && state != "all" {
		state = "open"
	}
	project, _ = strconv.ParseInt(q.Get("project"), 10, 64)
	page, err := strconv.Atoi(q.Get("page"))
	if err != nil || page < 1 {
		page = 1
	}
	return state, project, page
}

// tasksURL is the tasks page with the given state, project and page; defaults are left out.
func tasksURL(state string, project int64, page int) string {
	q := url.Values{}
	if state != "open" {
		q.Set("state", state)
	}
	if project != 0 {
		q.Set("project", strconv.FormatInt(project, 10))
	}
	if page > 1 {
		q.Set("page", strconv.Itoa(page))
	}
	if len(q) == 0 {
		return "/tasks"
	}
	return "/tasks?" + q.Encode()
}

func (s *Server) tasksPage(w http.ResponseWriter, r *http.Request) {
	s.renderTasks(w, r, r.URL.RequestURI())
}

func (s *Server) renderTasks(w http.ResponseWriter, r *http.Request, uri string, mods ...func(*quickBar)) {
	v, err := s.buildTasksPage(r.Context(), uri)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.applyContext(r, &v.quickBar)
	for _, mod := range mods {
		mod(&v.quickBar)
	}
	w.Header().Set("Vary", "HX-Request")
	name := "tasks.html"
	if isHX(r) {
		name = "tdash-fragment"
	}
	s.render(w, http.StatusOK, name, v)
}

func (s *Server) buildTasksPage(ctx context.Context, uri string) (tasksPageView, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return tasksPageView{}, domain.Invalid("bad tasks address")
	}
	state, project, page := tasksQuery(u.Query())
	projects, err := s.store.ListProjects(ctx, false)
	if err != nil {
		return tasksPageView{}, err
	}
	acts, err := s.store.ListActivities(ctx)
	if err != nil {
		return tasksPageView{}, err
	}
	all, err := s.store.ListAllTasks(ctx)
	if err != nil {
		return tasksPageView{}, err
	}
	projectName, actOf, filing := liveIndex(projects, acts)
	if _, ok := projectName[project]; !ok {
		project = 0
	}

	// Archived projects are left out (item 33); Unassigned tasks only show for all projects (item 91).
	tasks := slices.DeleteFunc(all, func(t domain.Task) bool {
		open := t.ClosedOn().IsZero()
		if state == "open" && !open || state == "closed" && open {
			return true
		}
		if t.ActivityID == 0 && t.ProjectID == 0 {
			return project != 0
		}
		owner := t.ProjectID
		if a, ok := actOf[t.ActivityID]; ok {
			owner = a.ProjectID
		}
		_, live := projectName[owner]
		return !live || project != 0 && owner != project
	})
	if state == "closed" {
		slices.SortFunc(tasks, func(a, b domain.Task) int { // newest close first
			return cmp.Or(cmp.Compare(b.ClosedAt, a.ClosedAt), cmp.Compare(b.ID, a.ID))
		})
	} else {
		domain.SortTasks(tasks)
	}

	pages := max(1, (len(tasks)+tasksPerPage-1)/tasksPerPage)
	page = min(page, pages)
	v := tasksPageView{
		Title: "Tasks · Workline", Next: uri,
		State: state, Project: strconv.FormatInt(project, 10),
		Count:    fmt.Sprintf("%d tasks · page %d of %d", len(tasks), page, pages),
		quickBar: newQuickBar(quickAddView{Pick: true, Next: uri}),
	}
	if len(tasks) == 1 {
		v.Count = fmt.Sprintf("1 task · page %d of %d", page, pages)
	}
	v.QuickFile = fileChoices(filing, "")
	for _, st := range []string{"open", "closed", "all"} {
		v.States = append(v.States, option{st, st, st == state})
	}
	v.Projects = []option{{"0", "All projects", project == 0}}
	for _, p := range projects {
		v.Projects = append(v.Projects, option{strconv.FormatInt(p.ID, 10), p.Name, p.ID == project})
	}
	if page > 1 {
		v.PrevHref = tasksURL(state, project, page-1)
	}
	if page < pages {
		v.NextHref = tasksURL(state, project, page+1)
	}
	for _, t := range tasks[(page-1)*tasksPerPage : min(len(tasks), page*tasksPerPage)] {
		row := taskListRow{
			ID: t.ID, Text: t.Text, Created: t.CreatedOn.String(), Closed: t.ClosedOn().String(),
			Priority: string(t.Priority), PriorityLabel: priorityLabel(t.Priority),
		}
		row.Project, row.Activity, _ = where(projectName, actOf, t.ActivityID, t.ProjectID)
		row.ActivityHref = entryHref(t.ActivityID, t.ProjectID)
		v.Tasks = append(v.Tasks, row)
	}
	return v, nil
}
