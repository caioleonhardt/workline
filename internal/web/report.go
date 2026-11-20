package web

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"workline/internal/domain"
	"workline/internal/store"
)

// The 1:1 report (SPEC §10 items 112–113): one model, shown as a page and
// written as Markdown that reads well as plain text in an email.

// reportLine is a bullet; Sub are its indented bullets. StarID is set on a
// highlight, which the page lets the user unstar.
type reportLine struct {
	Text   string
	Sub    []string
	StarID int64
}

type reportSection struct {
	Title string
	Lines []reportLine
}

type reportView struct {
	quickBar
	Title, Next      string
	From, To         string
	Projects         []option
	OnlyStarred      bool
	Heading          string
	Sections         []reportSection
	Markdown, MdHref string
}

// reportQuery reads from, to, project and only from the query. From defaults
// to To − 6, To to today; project is 0 for all.
func reportQuery(q url.Values, today domain.Date) (from, to domain.Date, project int64, onlyStarred bool, err error) {
	parse := func(key string, def domain.Date) (domain.Date, error) {
		v := q.Get(key)
		if v == "" {
			return def, nil
		}
		d, err := domain.ParseDate(v)
		if err != nil {
			return 0, domain.Invalid("%s must be a date (YYYY-MM-DD), not %q", key, v)
		}
		return d, nil
	}
	if to, err = parse("to", today); err != nil {
		return
	}
	if from, err = parse("from", to.AddDays(-6)); err != nil {
		return
	}
	if from.After(to) {
		err = domain.Invalid("from (%s) is after to (%s)", from, to)
		return
	}
	if project, err = queryProjectID(q); err != nil {
		return
	}
	return from, to, project, q.Get("only") == "starred", nil
}

// queryProjectID reads the optional `project` parameter.
func queryProjectID(q url.Values) (int64, error) {
	v := q.Get("project")
	if v == "" {
		return 0, nil
	}
	id, err := strconv.ParseInt(v, 10, 64)
	if err != nil || id <= 0 {
		return 0, domain.Invalid("project must be a project id, not %q", v)
	}
	return id, nil
}

// reportData is a built report with what the page needs around it.
type reportData struct {
	domain.Report
	Heading     string
	Projects    []domain.Project // live ones, for the project select
	Project     int64            // 0: all
	OnlyStarred bool
	Filing      []domain.Group // the quick-add bar's choices
}

// query is the report's resolved parameters, so a download matches the page
// even after "today" moves on.
func (d reportData) query() url.Values {
	q := url.Values{"from": {d.From.String()}, "to": {d.To.String()}}
	if d.Project != 0 {
		q.Set("project", strconv.FormatInt(d.Project, 10))
	}
	if d.OnlyStarred {
		q.Set("only", "starred")
	}
	return q
}

// loadReport builds the report for the query: live projects only (item 33),
// one of them when `project` is set, and Unassigned work only for all projects.
func (s *Server) loadReport(ctx context.Context, q url.Values) (reportData, error) {
	from, to, project, only, err := reportQuery(q, s.clock.Today())
	if err != nil {
		return reportData{}, err
	}
	projects, err := s.store.ListProjects(ctx, false)
	if err != nil {
		return reportData{}, err
	}
	d := reportData{Projects: projects, Project: project, OnlyStarred: only}
	shownProjects := projects
	if project != 0 {
		i := slices.IndexFunc(projects, func(p domain.Project) bool { return p.ID == project })
		if i < 0 {
			return reportData{}, fmt.Errorf("project %d: %w", project, store.ErrNotFound)
		}
		shownProjects = projects[i : i+1]
	}
	acts, err := s.store.ListActivities(ctx)
	if err != nil {
		return reportData{}, err
	}
	tasks, err := s.store.ListAllTasks(ctx)
	if err != nil {
		return reportData{}, err
	}
	entries, err := s.store.ListJournalBetween(ctx, from, to)
	if err != nil {
		return reportData{}, err
	}
	_, _, d.Filing = liveIndex(projects, acts)
	_, actOf, _ := liveIndex(shownProjects, acts)
	shown := func(activityID int64) bool {
		_, ok := actOf[activityID]
		return ok || activityID == 0 && project == 0
	}
	in := domain.ReportInput{From: from, To: to, Projects: shownProjects, OnlyStarred: only}
	for _, a := range acts {
		if _, ok := actOf[a.ID]; ok {
			in.Activities = append(in.Activities, a)
		}
	}
	for _, t := range tasks {
		if shown(t.ActivityID) {
			in.Tasks = append(in.Tasks, t)
		}
	}
	for _, e := range entries {
		if shown(e.ActivityID) {
			in.Entries = append(in.Entries, e)
		}
	}
	d.Report = domain.BuildReport(in)
	d.Heading = fmt.Sprintf("1:1 update · %s to %s", from, to)
	if project != 0 {
		d.Heading += " · " + shownProjects[0].Name
	}
	return d, nil
}

func reportTag(project, activity string) string {
	if activity == "" {
		return "Unassigned"
	}
	return project + " / " + activity
}

// firstLine is an entry's text with its later lines dropped, for a sub-bullet.
func firstLine(text string) string {
	first, rest, _ := strings.Cut(strings.TrimSpace(text), "\n")
	if strings.TrimSpace(rest) != "" {
		return first + " …"
	}
	return first
}

func entryLine(it domain.ReportItem, kind bool) reportLine {
	lead := it.Entry.LogDate().String()
	if kind {
		lead += " · " + kindLabel(it.Entry.Kind)
	}
	return reportLine{Text: lead + " · " + reportTag(it.Project, it.Activity) + ": " + strings.TrimSpace(it.Entry.Text)}
}

func taskLines(groups []domain.ReportTasks, label string, sub func(domain.JournalEntry) string) []reportLine {
	var out []reportLine
	for _, g := range groups {
		l := reportLine{Text: label + reportTag(g.Project, g.Activity)}
		for _, e := range g.Tasks {
			l.Sub = append(l.Sub, sub(e))
		}
		out = append(out, l)
	}
	return out
}

func change(before, after int) string {
	switch d := after - before; {
	case d > 0:
		return fmt.Sprintf("+%d", d)
	case d < 0:
		return fmt.Sprintf("−%d", -d)
	}
	return "no change"
}

// reportSections lays the report out in the order it is read; empty sections are left out.
func reportSections(r domain.Report) []reportSection {
	var out []reportSection
	add := func(title string, lines []reportLine) {
		if len(lines) > 0 {
			out = append(out, reportSection{title, lines})
		}
	}

	var lines []reportLine
	for _, it := range r.Highlights {
		l := entryLine(it, true)
		l.StarID = it.Entry.ID
		lines = append(lines, l)
	}
	add("Highlights", lines)

	lines = taskLines(r.Closed, "", func(e domain.JournalEntry) string { return firstLine(e.Text) })
	for _, a := range r.Finished {
		lines = append(lines, reportLine{Text: "Finished: " + reportTag(a.Project, a.Activity.Name)})
	}
	add("Done", lines)

	lines = nil
	for _, p := range r.Progress {
		text := fmt.Sprintf("%s: %d%% → %d%% (%s) · %d open · due %s",
			reportTag(p.Project, p.Activity.Name), p.Before, p.After, change(p.Before, p.After), p.Open, p.Activity.Due)
		if p.Activity.Status != domain.StatusActive {
			text += " · " + string(p.Activity.Status)
		}
		lines = append(lines, reportLine{Text: text})
	}
	add("Progress", lines)

	lines = nil
	for _, a := range r.Created {
		text := "New activity: " + reportTag(a.Project, a.Activity.Name)
		if a.Activity.RequestedBy != "" {
			text += ", requested by " + a.Activity.RequestedBy
		}
		lines = append(lines, reportLine{Text: text + ", due " + a.Activity.Due.String()})
	}
	lines = append(lines, taskLines(r.Opened, "Tasks added · ", func(e domain.JournalEntry) string {
		text := firstLine(e.Text)
		if e.ClosedAt != "" {
			text += " (closed)"
		}
		return text
	})...)
	for _, it := range r.Acks {
		lines = append(lines, entryLine(it, false))
	}
	add("New", lines)

	lines = nil
	for _, it := range r.Decisions {
		lines = append(lines, entryLine(it, false))
	}
	add("Decisions", lines)

	lines = nil
	for _, a := range r.BlockedNow {
		lines = append(lines, reportLine{Text: "Blocked now: " + reportTag(a.Project, a.Activity.Name)})
	}
	for _, it := range r.Blockers {
		lines = append(lines, entryLine(it, false))
	}
	add("Blockers", lines)
	return out
}

// reportMarkdown writes the report for an email body: a title, ## sections,
// - bullets with two-space indented sub-bullets and continuation lines.
func reportMarkdown(heading string, sections []reportSection) string {
	var b strings.Builder
	b.WriteString("# " + heading + "\n")
	if len(sections) == 0 {
		b.WriteString("\nNothing to report in this range.\n")
	}
	for _, s := range sections {
		b.WriteString("\n## " + s.Title + "\n\n")
		for _, l := range s.Lines {
			b.WriteString("- " + strings.ReplaceAll(l.Text, "\n", "\n  ") + "\n")
			for _, sub := range l.Sub {
				b.WriteString("  - " + sub + "\n")
			}
		}
	}
	return b.String()
}

func (s *Server) reportMD(w http.ResponseWriter, r *http.Request) {
	d, err := s.loadReport(r.Context(), r.URL.Query())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="workline-1on1-%s.md"`, d.To))
	fmt.Fprint(w, reportMarkdown(d.Heading, reportSections(d.Report)))
}

func (s *Server) report(w http.ResponseWriter, r *http.Request) {
	s.renderReport(w, r, r.URL.RequestURI())
}

func (s *Server) renderReport(w http.ResponseWriter, r *http.Request, uri string, mods ...func(*quickBar)) {
	u, err := url.Parse(uri)
	if err != nil {
		s.fail(w, r, domain.Invalid("bad report address"))
		return
	}
	d, err := s.loadReport(r.Context(), u.Query())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	v := reportView{
		Title: "1:1 report · Workline", Next: uri,
		From: d.From.String(), To: d.To.String(), OnlyStarred: d.OnlyStarred,
		Projects: []option{{"", "All projects", d.Project == 0}},
		Heading:  d.Heading,
		Sections: reportSections(d.Report),
		MdHref:   "/report.md?" + d.query().Encode(),
		quickBar: newQuickBar(quickAddView{Pick: true, Next: uri}),
	}
	for _, p := range d.Projects {
		v.Projects = append(v.Projects, option{strconv.FormatInt(p.ID, 10), p.Name, p.ID == d.Project})
	}
	v.Markdown = reportMarkdown(v.Heading, v.Sections)
	v.QuickFile = fileChoices(d.Filing, 0)
	for _, mod := range mods {
		mod(&v.quickBar)
	}
	w.Header().Set("Vary", "HX-Request")
	name := "report.html"
	if isHX(r) {
		name = "rdash-fragment"
	}
	s.render(w, http.StatusOK, name, v)
}
