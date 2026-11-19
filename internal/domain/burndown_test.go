package domain

import (
	"reflect"
	"testing"
)

func act(t *testing.T, start, due string) Activity {
	return Activity{Name: "a", Status: StatusActive, Start: d(t, start), Due: d(t, due)}
}

func binStr(bins []Bin) [][3]any {
	var out [][3]any
	for _, b := range bins {
		out = append(out, [3]any{b.Start.String() + ".." + b.End.String(), b.Open, b.Scope})
	}
	return out
}

func TestBurndownTimeline(t *testing.T) {
	// Mon 2026-03-02. Task 1 open all along, 2 created 03-09 closed 03-16.
	subs := []Task{sub(t, 1, "2026-03-02", ""), sub(t, 2, "2026-03-09", "2026-03-16")}
	a := act(t, "2026-03-02", "2026-03-22") // 21 days = 3 exact weeks

	got := binStr(Burndown(a, subs, 7, d(t, "2026-12-01")))
	want := [][3]any{
		{"2026-03-02..2026-03-08", 1, 1},
		{"2026-03-09..2026-03-15", 2, 2},
		{"2026-03-16..2026-03-22", 1, 2},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("exact multiple: %v", got)
	}

	// today cuts the last bin short
	got = binStr(Burndown(a, subs, 7, d(t, "2026-03-17")))
	want = [][3]any{
		{"2026-03-02..2026-03-08", 1, 1},
		{"2026-03-09..2026-03-15", 2, 2},
		{"2026-03-16..2026-03-17", 1, 2},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("clipped to today: %v", got)
	}

	// due before today, partial final bin clipped to due
	got = binStr(Burndown(act(t, "2026-03-02", "2026-03-10"), subs, 7, d(t, "2026-12-01")))
	if len(got) != 2 || got[1][0] != "2026-03-09..2026-03-10" {
		t.Errorf("clipped to due: %v", got)
	}

	// start == today: one single-day bin
	if got := binStr(Burndown(a, subs, 7, d(t, "2026-03-02"))); len(got) != 1 || got[0][0] != "2026-03-02..2026-03-02" {
		t.Errorf("start==today: %v", got)
	}
	// start > today: empty
	if got := Burndown(a, subs, 7, d(t, "2026-03-01")); len(got) != 0 {
		t.Errorf("start>today: %v", got)
	}
	// step 2
	got = binStr(Burndown(a, subs, 2, d(t, "2026-03-07")))
	if len(got) != 3 || got[2][0] != "2026-03-06..2026-03-07" {
		t.Errorf("step 2: %v", got)
	}
	// step 1 on a one-day activity
	if got := Burndown(act(t, "2026-03-02", "2026-03-02"), nil, 1, d(t, "2026-03-05")); len(got) != 1 {
		t.Errorf("one-day activity: %v", got)
	}

	// a done activity stops at its done date, but only if that is earlier
	done := a
	done.Status, done.DoneOn = StatusDone, d(t, "2026-03-10")
	got = binStr(Burndown(done, subs, 7, d(t, "2026-12-01")))
	if len(got) != 2 || got[1][0] != "2026-03-09..2026-03-10" {
		t.Errorf("stops at done_on: %v", got)
	}
	done.DoneOn = d(t, "2026-06-01")
	if got := Burndown(done, subs, 7, d(t, "2026-12-01")); len(got) != 3 {
		t.Errorf("done_on after due changes nothing: %v", binStr(got))
	}
	done.DoneOn = d(t, "2026-03-10")
	if got := Burndown(done, subs, 7, d(t, "2026-03-05")); len(got) != 1 {
		t.Errorf("today before done_on: %v", binStr(got))
	}

	mustPanic(t, "step 0", func() { Burndown(a, subs, 0, d(t, "2026-03-05")) })
	mustPanic(t, "inverted", func() { Burndown(act(t, "2026-03-05", "2026-03-04"), nil, 7, d(t, "2026-03-05")) })
	mustPanic(t, "no dates", func() { Burndown(Activity{}, nil, 7, d(t, "2026-03-05")) })
	mustPanic(t, "too long", func() { Burndown(act(t, "2000-01-01", "2026-01-01"), nil, 7, d(t, "2026-03-05")) })
}

func TestBurndownDetail(t *testing.T) {
	subs := []Task{sub(t, 1, "2026-03-02", ""), sub(t, 2, "2026-03-09", "2026-03-16"), sub(t, 3, "2026-03-10", "")}
	a := act(t, "2026-03-02", "2026-03-26") // 25 days: 3 full weeks + 4-day bin
	bins := BurndownDetail(a, subs, d(t, "2026-03-17"))
	want := [][3]any{
		{"2026-03-02..2026-03-08", 1, 1},
		{"2026-03-09..2026-03-15", 3, 3},
		{"2026-03-16..2026-03-22", 2, 3}, // straddles today, measured at today
		{"2026-03-23..2026-03-26", 0, 0},
	}
	if !reflect.DeepEqual(binStr(bins), want) {
		t.Errorf("detail: %v", binStr(bins))
	}
	for i, b := range bins {
		if b.Ahead != (i == 3) {
			t.Errorf("bin %d Ahead = %v", i, b.Ahead)
		}
	}
	// whole activity in the future: all ahead
	for _, b := range BurndownDetail(a, subs, d(t, "2026-01-01")) {
		if !b.Ahead {
			t.Error("expected all ahead")
		}
	}
	// whole activity in the past: none ahead, last bin ends at due
	past := BurndownDetail(a, subs, d(t, "2026-12-01"))
	if past[len(past)-1].End != a.Due || past[len(past)-1].Ahead || past[len(past)-1].Open != 2 {
		t.Errorf("past: %+v", past[len(past)-1])
	}
}

func TestClipBins(t *testing.T) {
	bins := []Bin{
		{Start: d(t, "2026-03-02"), End: d(t, "2026-03-08"), Open: 1},
		{Start: d(t, "2026-03-09"), End: d(t, "2026-03-15"), Open: 2},
		{Start: d(t, "2026-03-16"), End: d(t, "2026-03-22"), Open: 3},
		{Start: d(t, "2026-03-23"), End: d(t, "2026-03-29"), Open: 4},
	}
	got := ClipBins(bins, d(t, "2026-03-12"), d(t, "2026-03-18"))
	want := [][3]any{
		{"2026-03-12..2026-03-15", 2, 0},
		{"2026-03-16..2026-03-18", 3, 0},
	}
	if !reflect.DeepEqual(binStr(got), want) {
		t.Errorf("clip: %v", binStr(got))
	}
	if got := ClipBins(bins, d(t, "2026-03-09"), d(t, "2026-03-15")); len(got) != 1 || got[0].Start != bins[1].Start {
		t.Errorf("exact window: %v", got)
	}
	if got := ClipBins(bins, d(t, "2027-01-01"), d(t, "2027-01-31")); len(got) != 0 {
		t.Errorf("outside: %v", got)
	}
	if bins[0].Start != d(t, "2026-03-02") {
		t.Error("input mutated")
	}
	mustPanic(t, "inverted window", func() { ClipBins(bins, d(t, "2026-03-18"), d(t, "2026-03-12")) })
}

func TestGlobalMax(t *testing.T) {
	if GlobalMax(nil) != 0 || GlobalMax([][]Bin{{}}) != 0 {
		t.Error("empty must be 0")
	}
	got := GlobalMax([][]Bin{
		{{Open: 3}, {Open: 5}},
		{{Open: 4}, {Open: 99, Ahead: true}},
	})
	if got != 5 {
		t.Errorf("GlobalMax = %d, want 5 (ahead bins ignored)", got)
	}
}

func TestBurndownRange(t *testing.T) {
	tasks := []Task{
		{ID: 1, CreatedOn: d(t, "2026-11-01")},
		{ID: 2, CreatedOn: d(t, "2026-11-10"), ClosedOn: d(t, "2026-11-11")},
		{ID: 3, CreatedOn: d(t, "2026-11-12")},
	}
	bins := BurndownRange(tasks, d(t, "2026-11-10"), d(t, "2026-11-12"), 1)
	want := []Bin{
		{Start: d(t, "2026-11-10"), End: d(t, "2026-11-10"), Open: 2, Scope: 2},
		{Start: d(t, "2026-11-11"), End: d(t, "2026-11-11"), Open: 1, Scope: 2},
		{Start: d(t, "2026-11-12"), End: d(t, "2026-11-12"), Open: 2, Scope: 3},
	}
	if !reflect.DeepEqual(bins, want) {
		t.Errorf("bins = %+v", bins)
	}
	if got := BurndownRange(tasks, d(t, "2026-11-13"), d(t, "2026-11-12"), 1); len(got) != 0 {
		t.Errorf("empty range: %+v", got)
	}
}
