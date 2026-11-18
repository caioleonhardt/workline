package domain

import (
	"reflect"
	"strings"
	"testing"
)

func ids(as []Activity) []int64 {
	var out []int64
	for _, a := range as {
		out = append(out, a.ID)
	}
	return out
}

func TestSortActivities(t *testing.T) {
	mk := func(id int64, st Status, p Priority, due string) Activity {
		return Activity{ID: id, Status: st, Priority: p, Due: d(t, due)}
	}
	as := []Activity{
		mk(1, StatusDone, PriorityHigh, "2026-01-01"),   // done: after all not-done
		mk(2, StatusActive, PriorityNone, "2026-01-01"), // no priority
		mk(3, StatusActive, PriorityLow, "2026-03-01"),
		mk(4, StatusBlocked, PriorityHigh, "2026-05-01"),
		mk(5, StatusPlanned, PriorityHigh, "2026-02-01"), // same priority: earlier due first
		mk(6, StatusActive, PriorityMedium, "2026-02-01"),
		mk(7, StatusActive, PriorityMedium, "2026-02-01"), // same due: lower id first
		mk(8, StatusDone, PriorityNone, "2026-01-01"),     // done: priority still orders
	}
	SortActivities(as)
	if got, want := ids(as), []int64{5, 4, 6, 7, 3, 2, 1, 8}; !reflect.DeepEqual(got, want) {
		t.Errorf("order %v, want %v", got, want)
	}
}

func TestSortGroups(t *testing.T) {
	g := func(id int64, name string, acts ...Activity) Group {
		return Group{Project: Project{ID: id, Name: name}, Activities: acts}
	}
	act := func(st Status, p Priority) Activity { return Activity{Status: st, Priority: p} }
	gs := []Group{
		g(1, "zeta", act(StatusActive, PriorityNone)),
		g(2, "Only done", act(StatusDone, PriorityHigh)),
		g(3, "beta", act(StatusActive, PriorityMedium), act(StatusDone, PriorityHigh)), // done ignored: medium
		g(4, "Alpha", act(StatusPlanned, PriorityMedium)),
		g(5, "urgent", act(StatusBlocked, PriorityLow), act(StatusActive, PriorityHigh)),
		g(6, "empty"),
		g(7, "ALPHA", act(StatusActive, PriorityMedium)), // same rank and name as 4: id decides
	}
	SortGroups(gs)
	var names []string
	for _, x := range gs {
		names = append(names, x.Project.Name)
	}
	want := "urgent Alpha ALPHA beta zeta empty Only done"
	if got := strings.Join(names, " "); got != want {
		t.Errorf("groups %q, want %q", got, want)
	}
}
