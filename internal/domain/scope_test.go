package domain

import (
	"reflect"
	"testing"
)

// many returns n subtasks with ids from firstID, all created and closed on the given days.
func many(t *testing.T, firstID int64, n int, created, closed string) []Subtask {
	var out []Subtask
	for i := 0; i < n; i++ {
		out = append(out, sub(t, firstID+int64(i), created, closed))
	}
	return out
}

func cat(parts ...[]Subtask) []Subtask {
	var out []Subtask
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

type got struct {
	Baseline, Added, Growth, Add30, Close30 int
	GrowthFlag, Outpacing, Flagged          bool
}

func TestScopeCreep(t *testing.T) {
	// start 2026-06-01 => kickoff cutoff 2026-06-08; today 2026-11-18 => window starts after 2026-10-19.
	today := d(t, "2026-11-18")
	mkAct := func(s Status) Activity {
		return Activity{Status: s, Start: d(t, "2026-06-01"), Due: d(t, "2026-12-31")}
	}
	old := func(first int64, n int) []Subtask { return many(t, first, n, "2026-06-01", "") }
	ack := func(baseline int, lastID int64) *ScopeAck {
		return &ScopeAck{ID: 1, Date: d(t, "2026-10-01"), BaselineCount: baseline, LastSubtaskID: lastID}
	}

	tests := []struct {
		name   string
		status Status
		subs   []Subtask
		ack    *ScopeAck
		want   got
	}{
		{"no subtasks", StatusActive, nil, nil, got{}},
		{"all newer than grace: baseline 0, no growth flag", StatusActive,
			many(t, 1, 3, "2026-06-09", ""), nil, got{Baseline: 0, Added: 3}},
		{"created on cutoff is baseline", StatusActive,
			many(t, 1, 2, "2026-06-08", ""), nil, got{Baseline: 2}},
		{"created day after cutoff is newer", StatusActive,
			cat(old(1, 2), many(t, 3, 1, "2026-06-09", "")), nil, got{Baseline: 2, Added: 1, Growth: 50}},
		{"growth exactly 75 flagged", StatusActive,
			cat(old(1, 4), many(t, 5, 3, "2026-07-01", "")), nil,
			got{Baseline: 4, Added: 3, Growth: 75, GrowthFlag: true, Flagged: true}},
		{"growth 74 not flagged", StatusActive,
			cat(old(1, 27), many(t, 28, 20, "2026-07-01", "")), nil,
			got{Baseline: 27, Added: 20, Growth: 74}},
		{"growth 74.5 rounds up to 75 and flags", StatusActive,
			cat(old(1, 200), many(t, 201, 149, "2026-07-01", "")), nil,
			got{Baseline: 200, Added: 149, Growth: 75, GrowthFlag: true, Flagged: true}},
		{"outpacing: equal is not flagged", StatusActive,
			cat(old(1, 10), many(t, 11, 1, "2026-11-01", ""), many(t, 12, 1, "2026-06-01", "2026-11-01")), nil,
			got{Baseline: 11, Added: 1, Growth: 9, Add30: 1, Close30: 1}},
		{"outpacing: more added than closed", StatusActive,
			cat(old(1, 10), many(t, 11, 2, "2026-11-01", "")), nil,
			got{Baseline: 10, Added: 2, Growth: 20, Add30: 2, Outpacing: true, Flagged: true}},
		{"created on today-30 excluded from add30", StatusActive,
			cat(old(1, 10), many(t, 11, 1, "2026-10-19", "")), nil,
			got{Baseline: 10, Added: 1, Growth: 10}},
		{"created on today-29 included in add30", StatusActive,
			cat(old(1, 10), many(t, 11, 1, "2026-10-20", "")), nil,
			got{Baseline: 10, Added: 1, Growth: 10, Add30: 1, Outpacing: true, Flagged: true}},
		{"closed on today-30 excluded from close30", StatusActive,
			many(t, 1, 3, "2026-06-01", "2026-10-19"), nil, got{Baseline: 3}},
		{"close30 counts old subtasks too", StatusActive,
			many(t, 1, 3, "2026-06-01", "2026-10-20"), nil, got{Baseline: 3, Close30: 3}},
		{"both flags", StatusActive,
			cat(old(1, 2), many(t, 3, 3, "2026-11-10", "")), nil,
			got{Baseline: 2, Added: 3, Growth: 150, Add30: 3, GrowthFlag: true, Outpacing: true, Flagged: true}},
		{"done suppresses both", StatusDone,
			cat(old(1, 2), many(t, 3, 3, "2026-11-10", "")), nil,
			got{Baseline: 2, Added: 3, Growth: 150, Add30: 3}},
		{"blocked still flags", StatusBlocked,
			cat(old(1, 2), many(t, 3, 3, "2026-11-10", "")), nil,
			got{Baseline: 2, Added: 3, Growth: 150, Add30: 3, GrowthFlag: true, Outpacing: true, Flagged: true}},
		{"planned still flags", StatusPlanned,
			cat(old(1, 2), many(t, 3, 3, "2026-11-10", "")), nil,
			got{Baseline: 2, Added: 3, Growth: 150, Add30: 3, GrowthFlag: true, Outpacing: true, Flagged: true}},

		// after an acknowledgement
		{"ack: nothing newer, clear", StatusActive, old(1, 6), ack(6, 6), got{Baseline: 6}},
		{"ack: growth below threshold", StatusActive,
			cat(old(1, 6), many(t, 7, 1, "2026-06-02", "")), ack(6, 6),
			got{Baseline: 6, Added: 1, Growth: 17}},
		{"ack: regrowth to 75% flags again", StatusActive,
			cat(old(1, 4), many(t, 5, 3, "2026-09-01", "")), ack(4, 4),
			got{Baseline: 4, Added: 3, Growth: 75, GrowthFlag: true, Flagged: true}},
		{"ack: newer chosen by id, not by date (backfilled)", StatusActive,
			cat(old(1, 4), many(t, 5, 1, "2026-03-01", "")), ack(4, 4),
			got{Baseline: 4, Added: 1, Growth: 25}},
		{"ack: older id with recent date is not newer", StatusActive,
			cat(many(t, 1, 4, "2026-11-10", "")), ack(4, 4),
			got{Baseline: 4}},
		{"ack with last id 0 and baseline 0", StatusActive,
			many(t, 1, 2, "2026-09-01", ""), ack(0, 0),
			got{Baseline: 0, Added: 2}},
		{"ack: outpacing resets (only newer count as added)", StatusActive,
			cat(many(t, 1, 5, "2026-11-10", ""), many(t, 6, 1, "2026-06-01", "2026-11-12")), ack(6, 6),
			got{Baseline: 6, Close30: 1}},
		{"ack: deletions after ack clamp added at 0", StatusActive, old(1, 8), ack(10, 12),
			got{Baseline: 10}},
	}
	for _, c := range tests {
		cr := ScopeCreep(mkAct(c.status), c.subs, c.ack, today)
		g := got{cr.Baseline, cr.Added, cr.GrowthPct, cr.Add30, cr.Close30, cr.GrowthFlag, cr.OutpacingFlag, cr.Flagged}
		if g != c.want {
			t.Errorf("%s:\n got  %+v\n want %+v", c.name, g, c.want)
		}
		if cr.Total != len(c.subs) || cr.Acked != (c.ack != nil) {
			t.Errorf("%s: Total/Acked wrong: %+v", c.name, cr)
		}
	}
}

func TestReasonsAndAckText(t *testing.T) {
	both := Creep{Baseline: 4, Total: 8, Added: 4, GrowthPct: 100, Add30: 5, Close30: 2, GrowthFlag: true, OutpacingFlag: true, Flagged: true}
	want := []string{
		"Grew from 4 to 8 subtasks since kickoff (+100%).",
		"In the last 30 days 5 added vs 2 closed: work is arriving faster than it closes.",
	}
	if got := both.Reasons(); !reflect.DeepEqual(got, want) {
		t.Errorf("reasons: %q", got)
	}

	acked := both
	acked.Acked, acked.AckDate, acked.OutpacingFlag = true, d(t, "2026-10-01"), false
	if got := acked.Reasons(); !reflect.DeepEqual(got, []string{"Grew from 4 to 8 subtasks since the acknowledgement on 2026-10-01 (+100%)."}) {
		t.Errorf("acked reasons: %q", got)
	}
	if got := (Creep{}).Reasons(); len(got) != 0 {
		t.Errorf("no flags: %q", got)
	}

	if got, w := AckJournalText(both),
		"Scope change acknowledged: 4 → 8 subtasks (+100%); last 30 days +5 / −2. New baseline: 8."; got != w {
		t.Errorf("ack text: %q", got)
	}
	none := Creep{Baseline: 3, Total: 3, Add30: 0, Close30: 1}
	if got, w := AckJournalText(none),
		"Scope change acknowledged: 3 → 3 subtasks; last 30 days +0 / −1. New baseline: 3."; got != w {
		t.Errorf("ack text without growth: %q", got)
	}
}
