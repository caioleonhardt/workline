package domain

import (
	"reflect"
	"testing"
)

func TestLanes(t *testing.T) {
	sp := func(from, to string) Span { return Span{d(t, from), d(t, to)} }
	for _, c := range []struct {
		name  string
		spans []Span
		lanes []int
		n     int
	}{
		{"none", nil, []int{}, 0},
		{"one", []Span{sp("2026-01-01", "2026-02-01")}, []int{0}, 1},
		{"after each other share a lane", []Span{sp("2026-01-01", "2026-02-01"), sp("2026-02-02", "2026-03-01")}, []int{0, 0}, 1},
		{"touching on a day overlap", []Span{sp("2026-01-01", "2026-02-01"), sp("2026-02-01", "2026-03-01")}, []int{0, 1}, 2},
		{"first lane that fits", []Span{
			sp("2026-01-01", "2026-06-30"), sp("2026-03-01", "2026-04-30"), sp("2026-05-01", "2026-05-31"), sp("2026-07-01", "2026-12-31"),
		}, []int{0, 1, 1, 0}, 2},
		{"order is kept, not re-sorted", []Span{sp("2026-06-01", "2026-06-30"), sp("2026-01-01", "2026-01-31")}, []int{0, 1}, 2},
	} {
		lanes, n := Lanes(c.spans)
		if !reflect.DeepEqual(lanes, c.lanes) || n != c.n {
			t.Errorf("%s: %v %d, want %v %d", c.name, lanes, n, c.lanes, c.n)
		}
	}
	mustPanic(t, "inverted", func() { Lanes([]Span{sp("2026-02-01", "2026-01-01")}) })
}
