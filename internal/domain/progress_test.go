package domain

import "testing"

func TestOpenAndScopeAt(t *testing.T) {
	subs := []Task{
		sub(t, 1, "2026-03-10", ""),           // open from 03-10
		sub(t, 2, "2026-03-10", "2026-03-15"), // closed 03-15
		sub(t, 3, "2026-03-20", "2026-03-20"), // created and closed same day
		sub(t, 4, "2026-03-12", ""),
	}
	tests := []struct {
		at          string
		open, scope int
	}{
		{"2026-03-09", 0, 0}, // before anything
		{"2026-03-10", 2, 2}, // created on t counts
		{"2026-03-14", 3, 3}, // 4 created, 2 still open
		{"2026-03-15", 2, 3}, // closed on t counts as closed
		{"2026-03-20", 2, 4}, // same-day open/close never open
		{"2026-12-31", 2, 4},
	}
	for _, c := range tests {
		at := d(t, c.at)
		if got := OpenAt(subs, at); got != c.open {
			t.Errorf("OpenAt(%s) = %d, want %d", c.at, got, c.open)
		}
		if got := ScopeAt(subs, at); got != c.scope {
			t.Errorf("ScopeAt(%s) = %d, want %d", c.at, got, c.scope)
		}
	}
	if OpenAt(nil, d(t, "2026-01-01")) != 0 || ScopeAt(nil, d(t, "2026-01-01")) != 0 {
		t.Error("empty")
	}
	// closed one day after t is still open at t
	if OpenAt([]Task{sub(t, 1, "2026-03-10", "2026-03-11")}, d(t, "2026-03-10")) != 1 {
		t.Error("closed on t+1 should be open at t")
	}
}

func closedN(t *testing.T, closed, total int) []Task {
	var subs []Task
	for i := 0; i < total; i++ {
		c := ""
		if i < closed {
			c = "2026-02-01"
		}
		subs = append(subs, sub(t, int64(i+1), "2026-01-01", c))
	}
	return subs
}

func TestProgress(t *testing.T) {
	tests := []struct {
		name          string
		status        Status
		closed, total int
		want          int
	}{
		{"none done", StatusDone, 0, 0, 100},
		{"none active", StatusActive, 0, 0, 0},
		{"none blocked", StatusBlocked, 0, 0, 0},
		{"none planned", StatusPlanned, 0, 0, 0},
		{"1/3", StatusActive, 1, 3, 33},
		{"2/3", StatusActive, 2, 3, 67},
		{"1/8 half up", StatusActive, 1, 8, 13},
		{"5/8 half up", StatusActive, 5, 8, 63},
		{"all", StatusActive, 4, 4, 100},
		{"done status uses ratio", StatusDone, 1, 4, 25},
	}
	for _, c := range tests {
		if got := Progress(c.status, closedN(t, c.closed, c.total)); got != c.want {
			t.Errorf("%s: %d, want %d", c.name, got, c.want)
		}
	}
	if ProjectProgress(nil) != 0 {
		t.Error("empty project must be 0")
	}
	if got := ProjectProgress(closedN(t, 7, 14)); got != 50 {
		t.Errorf("ProjectProgress = %d", got)
	}
	if got := ProjectProgress(closedN(t, 3, 7)); got != 43 {
		t.Errorf("ProjectProgress = %d", got)
	}
	mustPanic(t, "percent d=0", func() { percent(1, 0) })
}
