package domain

import "testing"

func TestOverrun(t *testing.T) {
	done := func(start, due, doneOn string) Activity {
		return Activity{Status: StatusDone, Start: d(t, start), Due: d(t, due), DoneOn: d(t, doneOn)}
	}
	for _, c := range []struct {
		name      string
		a         Activity
		end, warn string // warn "" for not late
	}{
		{"on time", done("2026-01-01", "2026-01-10", "2026-01-10"), "2026-01-10", ""},
		{"early", done("2026-01-01", "2026-01-10", "2026-01-05"), "2026-01-10", ""},
		{"late: 10 planned days tolerate 2", done("2026-01-01", "2026-01-10", "2026-01-11"), "2026-01-11", "2026-01-12"},
		{"very late", done("2026-01-01", "2026-01-10", "2026-01-20"), "2026-01-20", "2026-01-12"},
		{"rounded up: 11 days tolerate 3", done("2026-01-01", "2026-01-11", "2026-01-20"), "2026-01-20", "2026-01-14"},
		{"one-day activity tolerates 1", done("2026-01-01", "2026-01-01", "2026-01-03"), "2026-01-03", "2026-01-02"},
		{"not done: due is the end", Activity{Status: StatusActive, Start: d(t, "2026-01-01"), Due: d(t, "2026-01-10")}, "2026-01-10", ""},
	} {
		if got := c.a.End(); got != d(t, c.end) {
			t.Errorf("%s: end %s, want %s", c.name, got, c.end)
		}
		warn, late := c.a.Overrun()
		if late != (c.warn != "") || late && warn != d(t, c.warn) {
			t.Errorf("%s: overrun %s %v, want %q", c.name, warn, late, c.warn)
		}
	}
}
