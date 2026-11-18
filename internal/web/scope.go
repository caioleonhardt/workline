package web

import (
	"net/http"
	"strconv"

	"workline/internal/domain"
)

type scopeView struct {
	BaselineLabel                              string
	Baseline, Total, GrowthPct, Add30, Close30 int
	Flagged                                    bool
	Reasons                                    []string
	AckNote                                    string // set when acknowledged and no longer flagged
}

func buildScope(a domain.Activity, subs []domain.Subtask, ack *domain.ScopeAck, today domain.Date) scopeView {
	c := domain.ScopeCreep(a, subs, ack, today)
	v := scopeView{
		BaselineLabel: "At kickoff", Baseline: c.Baseline, Total: c.Total, GrowthPct: c.GrowthPct,
		Add30: c.Add30, Close30: c.Close30, Flagged: c.Flagged, Reasons: c.Reasons(),
	}
	if c.Acked {
		v.BaselineLabel = "Baseline (ack " + c.AckDate.String() + ")"
		if !c.Flagged {
			v.AckNote = "Scope change acknowledged on " + c.AckDate.String() + " with a baseline of " + strconv.Itoa(c.Baseline) + " subtasks."
		}
	}
	return v
}

func (s *Server) ackScope(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err == nil {
		_, err = s.store.AckScope(r.Context(), id, s.clock)
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	redirect(w, r, nextURL(r))
}
