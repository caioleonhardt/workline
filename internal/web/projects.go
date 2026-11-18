package web

import (
	"fmt"
	"net/http"
	"strconv"

	"workline/internal/domain"
	"workline/internal/store"
)

// pathID parses the {id} path value; anything but a positive integer is a 404.
func pathID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("id %q: %w", r.PathValue("id"), store.ErrNotFound)
	}
	return id, nil
}

type projectsView struct {
	Title, Next string
	Projects    []domain.Project
}

func (s *Server) projects(w http.ResponseWriter, r *http.Request) {
	ps, err := s.store.ListProjects(r.Context(), true)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, http.StatusOK, "projects.html", projectsView{Title: "Projects", Next: r.URL.RequestURI(), Projects: ps})
}

// updateProject renames (form has name) and/or archives (form has archived).
func (s *Server) updateProject(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err == nil && r.PostFormValue("name") != "" {
		err = s.store.RenameProject(r.Context(), id, r.PostFormValue("name"))
	}
	if err == nil && r.PostForm.Has("archived") {
		err = s.store.SetProjectArchived(r.Context(), id, r.PostFormValue("archived") == "1")
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	redirect(w, r, nextURL(r))
}

func (s *Server) deleteProject(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err == nil {
		err = s.store.DeleteProject(r.Context(), id)
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	redirect(w, r, nextURL(r))
}
