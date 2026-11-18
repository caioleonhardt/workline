package web

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"workline/internal/domain"
	"workline/internal/store"
)

func formDate(r *http.Request, field string) (domain.Date, error) {
	d, err := domain.ParseDate(r.PostFormValue(field))
	if err != nil {
		return 0, fmt.Errorf("%s: %w", field, err)
	}
	return d, nil
}

// formLines returns the non-blank lines of a textarea value.
func formLines(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			out = append(out, line)
		}
	}
	return out
}

// createActivity handles the dashboard's inline new-activity form.
func (s *Server) createActivity(w http.ResponseWriter, r *http.Request) {
	in := store.NewActivity{
		Name:        r.PostFormValue("name"),
		RequestedBy: r.PostFormValue("requested_by"),
		Priority:    domain.Priority(r.PostFormValue("priority")),
		Subtasks:    formLines(r.PostFormValue("subtasks")),
	}
	var err error
	if r.PostFormValue("project") == "new" {
		in.NewProjectName = r.PostFormValue("new_project")
	} else if in.ProjectID, err = strconv.ParseInt(r.PostFormValue("project"), 10, 64); err != nil {
		err = fmt.Errorf("%w: choose a project", domain.ErrInvalid)
	}
	if err == nil {
		in.Start, err = formDate(r, "start")
	}
	if err == nil {
		in.Due, err = formDate(r, "due")
	}
	var a domain.Activity
	if err == nil {
		a, err = s.store.CreateActivity(r.Context(), in, s.clock)
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	to := withParam(nextURL(r), "new", "")
	redirect(w, r, withParam(to, "sel", strconv.FormatInt(a.ID, 10)))
}
