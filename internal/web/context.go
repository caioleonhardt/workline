package web

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"workline/internal/domain"
)

// The context (SPEC §10 item 121) is where new entries go until it changes:
// a project or an activity, set by `:p` or `:a` in the command line and kept
// in a cookie, so the server applies it with or without scripting.
const contextCookie = "workline_ctx"

// entryContext is a resolved, live context.
type entryContext struct {
	Link          string // an entryLink
	Name, Project string // Project is set for an activity
}

// Label is how the statusline names the context.
func (c entryContext) Label() string {
	if c.Project != "" {
		return c.Project + " › " + c.Name
	}
	return c.Name
}

// readContext returns the request's context, if it names a live project or activity.
func (s *Server) readContext(ctx context.Context, r *http.Request) (entryContext, bool) {
	ck, err := r.Cookie(contextCookie)
	if err != nil || ck.Value == "" {
		return entryContext{}, false
	}
	activity, project, err := parseLink(ck.Value)
	if err != nil || activity == 0 && project == 0 {
		return entryContext{}, false
	}
	projects, err := s.store.ListProjects(ctx, false)
	if err != nil {
		return entryContext{}, false
	}
	acts, err := s.store.ListActivities(ctx)
	if err != nil {
		return entryContext{}, false
	}
	projectName, actOf, _ := liveIndex(projects, acts)
	p, a, live := where(projectName, actOf, activity, project)
	if !live {
		return entryContext{}, false
	}
	if a == "" {
		return entryContext{Link: ck.Value, Name: p}, true
	}
	return entryContext{Link: ck.Value, Name: a, Project: p}, true
}

// applyContext makes the context the bar's default: selected in its select,
// or its target on the roadmap when no activity is selected there.
func (s *Server) applyContext(r *http.Request, b *quickBar) {
	c, ok := s.readContext(r.Context(), r)
	if !ok {
		return
	}
	b.Context = c.Label()
	if b.QuickFile != nil {
		selectLink(b.QuickFile, c.Link)
		return
	}
	if b.QuickAdd.ID == 0 && b.QuickAdd.Link == "" {
		b.QuickAdd.Link, b.QuickAdd.Name, b.QuickAdd.Project = c.Link, c.Name, c.Project
	}
}

// selectLink selects link in the groups' options.
func selectLink(groups []optGroup, link string) {
	for i := range groups {
		for j := range groups[i].Options {
			groups[i].Options[j].Selected = groups[i].Options[j].Value == link
		}
	}
}

// setContext runs a command line command: `p name|id` or `a name|id` sets the
// context, `p` or `a` alone clears it. It answers 303 to `next`, or 422 with
// the message the statusline shows.
func (s *Server) setContext(w http.ResponseWriter, r *http.Request) {
	cmd, arg, _ := strings.Cut(strings.TrimSpace(r.PostFormValue("q")), " ")
	arg = strings.TrimSpace(arg)
	if cmd != "p" && cmd != "a" {
		http.Error(w, fmt.Sprintf("E492: Not a command: %s (try :p name or :a name)", cmd), http.StatusUnprocessableEntity)
		return
	}
	if arg == "" {
		http.SetCookie(w, &http.Cookie{Name: contextCookie, Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteLaxMode})
		redirect(w, r, nextURL(r))
		return
	}
	ctx := r.Context()
	projects, err := s.store.ListProjects(ctx, false)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	acts, err := s.store.ListActivities(ctx)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	_, actOf, _ := liveIndex(projects, acts)
	var cs []domain.Candidate
	what := "project"
	if cmd == "p" {
		for _, p := range projects {
			cs = append(cs, domain.Candidate{ID: p.ID, Name: p.Name})
		}
	} else {
		what = "activity"
		for _, a := range acts {
			if _, live := actOf[a.ID]; live {
				cs = append(cs, domain.Candidate{ID: a.ID, Name: a.Name})
			}
		}
	}
	c, several, ok := domain.Pick(arg, cs)
	switch {
	case len(several) > 0:
		http.Error(w, fmt.Sprintf("%d %s names match %q: %s", len(several), what, arg, strings.Join(several, ", ")), http.StatusUnprocessableEntity)
		return
	case !ok:
		http.Error(w, fmt.Sprintf("No %s matches %q", what, arg), http.StatusUnprocessableEntity)
		return
	}
	link := entryLink(0, c.ID)
	if cmd == "a" {
		link = entryLink(c.ID, 0)
	}
	http.SetCookie(w, &http.Cookie{Name: contextCookie, Value: link, Path: "/", MaxAge: 365 * 24 * 3600, HttpOnly: true, SameSite: http.SameSiteLaxMode})
	redirect(w, r, nextURL(r))
}
