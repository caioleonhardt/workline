package domain

import (
	"errors"
	"testing"
	"time"
)

func TestParseDate(t *testing.T) {
	bad := []string{"", "2026-02-29", "2026-13-01", "2026-1-1", "2026-01-01x", " 2026-01-01", "0000-01-01", "26-01-01", "2026/01/01"}
	for _, s := range bad {
		if _, err := ParseDate(s); !errors.Is(err, ErrInvalid) {
			t.Errorf("ParseDate(%q) err = %v, want ErrInvalid", s, err)
		}
	}
	for _, s := range []string{"2024-02-29", "2026-12-31", "0001-01-01", "9999-12-31"} {
		got, err := ParseDate(s)
		if err != nil || got.String() != s {
			t.Errorf("ParseDate(%q) = %v, %v", s, got, err)
		}
	}
	if d(t, "0001-01-01") != 1 {
		t.Error("epoch anchor changed")
	}
}

func TestDateRoundTripAndArithmetic(t *testing.T) {
	start := d(t, "2023-12-25")
	for i := 0; i < 3000; i++ {
		x := start.AddDays(i)
		back, err := ParseDate(x.String())
		if err != nil || back != x {
			t.Fatalf("round trip %d: %v %v", i, back, err)
		}
	}
	tests := []struct {
		from, want string
		n          int
	}{
		{"2026-01-31", "2026-02-01", 1},
		{"2024-02-28", "2024-03-01", 2},
		{"2025-02-28", "2025-03-01", 1},
		{"2026-12-31", "2027-01-01", 1},
		{"2026-03-01", "2026-02-28", -1},
	}
	for _, c := range tests {
		if got := d(t, c.from).AddDays(c.n); got != d(t, c.want) {
			t.Errorf("%s+%d = %s, want %s", c.from, c.n, got, c.want)
		}
	}
	a, b := d(t, "2026-11-18"), d(t, "2026-11-20")
	if b.Sub(a) != 2 || !a.Before(b) || !b.After(a) || a.After(b) || a.Before(a) {
		t.Error("ordering or Sub wrong")
	}
	if a.Weekday() != time.Wednesday || a.Year() != 2026 || a.Month() != time.November || a.Day() != 18 {
		t.Error("calendar accessors wrong")
	}
	var zero Date
	if !zero.IsZero() || zero.String() != "" || a.IsZero() {
		t.Error("zero handling wrong")
	}
	mustPanic(t, "zero.Weekday", func() { zero.Weekday() })
	mustPanic(t, "DateOf invalid", func() { DateOf(2026, time.February, 30) })
}

func TestClocks(t *testing.T) {
	want := d(t, "2026-11-18")
	var c Clock = FixedClock{D: want}
	if c.Today() != want {
		t.Error("FixedClock")
	}
	if (SystemClock{}).Today().IsZero() {
		t.Error("SystemClock returned zero")
	}
}
