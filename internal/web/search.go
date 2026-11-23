package web

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"

	"workline/internal/domain"
)

// searchView is the search page (SPEC §10 item 103).
type searchView struct {
	quickBar
	Title, Next string
	Query       string
	Error       string // a refused query's message, shown instead of results
	Count       int    // every match, not only those shown
	CountLabel  string
	ClearHref   string
	Journal     journalView
}

// searchIndex is what a search reads: the live activities and every entry.
type searchIndex struct {
	entries     []domain.JournalEntry
	projectName map[int64]string
	actOf       map[int64]domain.Activity
	filing      []domain.Group
}

func (s *Server) loadSearchIndex(ctx context.Context) (searchIndex, error) {
	projects, err := s.store.ListProjects(ctx, false)
	if err != nil {
		return searchIndex{}, err
	}
	acts, err := s.store.ListActivities(ctx)
	if err != nil {
		return searchIndex{}, err
	}
	entries, err := s.store.ListAllJournal(ctx)
	if err != nil {
		return searchIndex{}, err
	}
	var ix searchIndex
	ix.projectName, ix.actOf, ix.filing = liveIndex(projects, acts)
	ix.entries = entries
	return ix, nil
}

// find returns the entries q matches, newest first: Unassigned ones and those
// of live projects.
func (ix searchIndex) find(q domain.Query) []domain.JournalEntry {
	var out []domain.JournalEntry
	for _, e := range ix.entries {
		project, activity, live := where(ix.projectName, ix.actOf, e.ActivityID, e.ProjectID)
		if live && q.Match(e, project, activity) {
			out = append(out, e)
		}
	}
	return out
}

func (s *Server) search(w http.ResponseWriter, r *http.Request) {
	s.renderSearch(w, r, r.URL.RequestURI())
}

func (s *Server) renderSearch(w http.ResponseWriter, r *http.Request, uri string, mods ...func(*quickBar)) {
	v, err := s.buildSearch(r.Context(), uri)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.applyContext(r, &v.quickBar)
	for _, mod := range mods {
		mod(&v.quickBar)
	}
	w.Header().Set("Vary", "HX-Request")
	name := "search.html"
	if isHX(r) {
		name = "sdash-fragment"
	}
	s.render(w, http.StatusOK, name, v)
}

func (s *Server) buildSearch(ctx context.Context, uri string) (searchView, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return searchView{}, domain.Invalid("bad search address")
	}
	params := u.Query()
	limit := feedStep
	if n, err := strconv.Atoi(params.Get("feed")); err == nil && n >= feedStep && n <= feedMax {
		limit = n
	}
	ix, err := s.loadSearchIndex(ctx)
	if err != nil {
		return searchView{}, err
	}
	v := searchView{
		Title: "Search · Workline", Next: uri, Query: params.Get("q"), ClearHref: localPath(params.Get("from")),
		quickBar: newQuickBar(quickAddView{Pick: true, Next: uri}),
	}
	v.QuickFile = fileChoices(ix.filing, "")
	q, err := domain.ParseQuery(v.Query)
	var qe *domain.QueryError
	if errors.As(err, &qe) {
		v.Error = qe.Message
		return v, nil
	}
	found := ix.find(q)
	v.Count = len(found)
	v.CountLabel = strconv.Itoa(v.Count) + " results"
	if v.Count == 1 {
		v.CountLabel = "1 result"
	}
	shown := found[:min(limit, len(found))]
	v.Journal = journalView{Next: uri, Title: "Results", FullLoadLinks: true, Entries: pageFeedRows(shown, ix.projectName, ix.actOf, ix.filing)}
	if len(found) > limit && limit < feedMax {
		v.Journal.MoreHref, v.Journal.MoreCount = withParam(uri, "feed", strconv.Itoa(limit+feedStep)), feedStep
	}
	return v, nil
}

// searchIDs answers the ids of every entry the query matches, for marking the
// entries on the current page, or the query's error with 400.
func (s *Server) searchIDs(w http.ResponseWriter, r *http.Request) {
	q, err := domain.ParseQuery(r.URL.Query().Get("q"))
	var qe *domain.QueryError
	if errors.As(err, &qe) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": qe.Message})
		return
	}
	ix, err := s.loadSearchIndex(r.Context())
	if err != nil {
		slog.Error("search ids", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Search failed"})
		return
	}
	ids := []int64{}
	for _, e := range ix.find(q) {
		ids = append(ids, e.ID)
	}
	writeJSON(w, http.StatusOK, map[string][]int64{"ids": ids})
}

type searchKey struct {
	Key    string   `json:"key"`
	Values []string `json:"values"`
}

// searchWords answers what the search line can complete: the qualifiers with
// their values, and the live project and activity names.
func (s *Server) searchWords(w http.ResponseWriter, r *http.Request) {
	ix, err := s.loadSearchIndex(r.Context())
	if err != nil {
		slog.Error("search words", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Search failed"})
		return
	}
	words := struct {
		Keys       []searchKey `json:"keys"`
		Projects   []string    `json:"projects"`
		Activities []string    `json:"activities"`
	}{Projects: []string{}, Activities: []string{}}
	for _, k := range domain.QueryKeys {
		words.Keys = append(words.Keys, searchKey{k.Key, append([]string{}, k.Values...)})
	}
	seen := map[string]bool{}
	for _, g := range ix.filing {
		words.Projects = append(words.Projects, g.Project.Name)
		for _, a := range g.Activities {
			if !seen[a.Name] {
				seen[a.Name] = true
				words.Activities = append(words.Activities, a.Name)
			}
		}
	}
	writeJSON(w, http.StatusOK, words)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("write json", "err", err)
	}
}
