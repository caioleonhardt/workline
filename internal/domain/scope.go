package domain

import "fmt"

// Scope-creep constants (§4.4).
const (
	GrowthThreshold = 75 // percent
	Window          = 30 // days
	KickoffGrace    = 7  // days
)

// Creep is the result of the scope-creep rule for one activity.
type Creep struct {
	Baseline, Total, Added, GrowthPct int
	Add30, Close30                    int
	Acked                             bool
	AckDate                           Date
	GrowthFlag, OutpacingFlag         bool
	Flagged                           bool
}

// ScopeCreep applies §4.4. ack is the latest ScopeAck, or nil.
func ScopeCreep(a Activity, subs []Subtask, ack *ScopeAck, today Date) Creep {
	total := len(subs)
	windowStart := today.AddDays(-Window)
	c := Creep{Total: total}

	var newer []Subtask
	if ack != nil {
		c.Acked, c.AckDate = true, ack.Date
		c.Baseline = ack.BaselineCount
		for _, s := range subs {
			if s.ID > ack.LastSubtaskID {
				newer = append(newer, s)
			}
		}
	} else {
		cutoff := a.Start.AddDays(KickoffGrace)
		for _, s := range subs {
			if s.CreatedOn.After(cutoff) {
				newer = append(newer, s)
			}
		}
		c.Baseline = total - len(newer)
	}

	if c.Added = total - c.Baseline; c.Added < 0 {
		c.Added = 0
	}
	if c.Baseline > 0 {
		c.GrowthPct = percent(c.Added, c.Baseline)
	}
	for _, s := range newer {
		if s.CreatedOn.After(windowStart) {
			c.Add30++
		}
	}
	for _, s := range subs {
		if !s.ClosedOn.IsZero() && s.ClosedOn.After(windowStart) {
			c.Close30++
		}
	}

	live := a.Status != StatusDone
	c.GrowthFlag = live && c.Baseline > 0 && c.GrowthPct >= GrowthThreshold
	c.OutpacingFlag = live && c.Add30 > c.Close30
	c.Flagged = c.GrowthFlag || c.OutpacingFlag
	return c
}

// Reasons returns the warning texts for the raised flags (§4.4).
func (c Creep) Reasons() []string {
	var out []string
	if c.GrowthFlag {
		since := "kickoff"
		if c.Acked {
			since = "the acknowledgement on " + c.AckDate.String()
		}
		out = append(out, fmt.Sprintf("Grew from %d to %d subtasks since %s (+%d%%).", c.Baseline, c.Total, since, c.GrowthPct))
	}
	if c.OutpacingFlag {
		out = append(out, fmt.Sprintf("In the last %d days %d added vs %d closed: work is arriving faster than it closes.", Window, c.Add30, c.Close30))
	}
	return out
}

// AckJournalText is the journal text written by Acknowledge (§5.4).
func AckJournalText(c Creep) string {
	growth := ""
	if c.Added > 0 {
		growth = fmt.Sprintf(" (+%d%%)", c.GrowthPct)
	}
	return fmt.Sprintf("Scope change acknowledged: %d → %d subtasks%s; last %d days +%d / −%d. New baseline: %d.",
		c.Baseline, c.Total, growth, Window, c.Add30, c.Close30, c.Total)
}
