package domain

import "testing"

func TestComputeTiles(t *testing.T) {
	today := d(t, "2026-11-18")
	mk := func(id int64, st Status, doneOn string, subs ...Task) ActivityWithTasks {
		a := Activity{ID: id, Status: st, Start: d(t, "2026-06-01"), Due: d(t, "2026-12-31")}
		if doneOn != "" {
			a.DoneOn = d(t, doneOn)
		}
		return ActivityWithTasks{Activity: a, Tasks: subs}
	}
	open := func(n int) []Task { return many(t, 1, n, "2026-06-01", "") }
	flagged := cat(open(2), many(t, 3, 3, "2026-11-10", "")) // growth 150% and outpacing

	items := []ActivityWithTasks{
		mk(1, StatusActive, "", flagged...),
		mk(2, StatusActive, "", open(3)...),
		mk(3, StatusBlocked, "", open(1)...),
		mk(4, StatusPlanned, ""),
		mk(5, StatusDone, "2026-03-01", open(4)...), // leftovers of a done activity are not open work
		mk(6, StatusDone, "2025-12-31"),             // done last year
		mk(7, StatusDone, "2026-12-31"),
		mk(8, StatusDone, ""), // no done date: counts for no year
	}
	want := Tiles{Active: 2, Blocked: 1, Planned: 1, DoneThisYear: 2, OpenTasks: 5 + 3 + 1, ScopeFlags: 1}
	if got := ComputeTiles(items, 2026, today); got != want {
		t.Errorf("tiles %+v, want %+v", got, want)
	}
	if got := ComputeTiles(items, 2025, today); got.DoneThisYear != 1 {
		t.Errorf("2025 done = %d", got.DoneThisYear)
	}
	// an acknowledgement clears the flag
	items[0].Ack = &ScopeAck{BaselineCount: 5, LastTaskID: 5}
	if got := ComputeTiles(items, 2026, today); got.ScopeFlags != 0 {
		t.Errorf("acked flag = %d", got.ScopeFlags)
	}
	if got := ComputeTiles(nil, 2026, today); got != (Tiles{}) {
		t.Errorf("empty: %+v", got)
	}
}
