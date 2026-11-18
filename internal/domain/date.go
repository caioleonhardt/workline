// Package domain holds Workline's pure rules (SPEC §4). It performs no I/O
// and imports only the standard library.
package domain

import (
	"fmt"
	"time"
)

// Date is a civil calendar date, stored as a day count where 0001-01-01 is 1.
// The zero value means "no date". There is no time zone: callers decide what
// "today" is and hand it in as a Date.
type Date int32

// daysBeforeUnixEpoch is the Date value of 1970-01-01.
const daysBeforeUnixEpoch = 719163

// assert panics when a programmer-error precondition is violated.
func assert(cond bool, msg string) {
	if !cond {
		panic("domain: " + msg)
	}
}

// DateOf returns the Date for year, month and day. It panics if the triple is
// not a real calendar date in years 1..9999.
func DateOf(year int, month time.Month, day int) Date {
	t := time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
	assert(t.Year() == year && t.Month() == month && t.Day() == day, "invalid civil date")
	return fromTime(t)
}

// ParseDate parses a strict YYYY-MM-DD string.
func ParseDate(s string) (Date, error) {
	t, err := time.Parse("2006-01-02", s)
	if err != nil || len(s) != 10 || t.Year() < 1 {
		return 0, fmt.Errorf("%w: date %q is not YYYY-MM-DD", ErrInvalid, s)
	}
	return fromTime(t), nil
}

func fromTime(t time.Time) Date {
	secs := t.Unix()
	assert(secs%86400 == 0, "time is not midnight UTC")
	return Date(secs/86400 + daysBeforeUnixEpoch)
}

func (d Date) time() time.Time {
	assert(d > 0, "zero date has no calendar value")
	return time.Unix((int64(d)-daysBeforeUnixEpoch)*86400, 0).UTC()
}

// String formats d as YYYY-MM-DD. The zero Date formats as "".
func (d Date) String() string {
	if d.IsZero() {
		return ""
	}
	return d.time().Format("2006-01-02")
}

// IsZero reports whether d is the "no date" value.
func (d Date) IsZero() bool { return d == 0 }

// AddDays returns d shifted by n days (n may be negative).
func (d Date) AddDays(n int) Date { return d + Date(n) }

// Sub returns the number of days from o to d.
func (d Date) Sub(o Date) int { return int(d) - int(o) }

// Before reports whether d is earlier than o.
func (d Date) Before(o Date) bool { return d < o }

// After reports whether d is later than o.
func (d Date) After(o Date) bool { return d > o }

// Weekday returns the day of the week.
func (d Date) Weekday() time.Weekday { return d.time().Weekday() }

// Year returns the calendar year.
func (d Date) Year() int { return d.time().Year() }

// Month returns the calendar month.
func (d Date) Month() time.Month { return d.time().Month() }

// Day returns the day of the month.
func (d Date) Day() int { return d.time().Day() }

// Clock supplies "today". Tests use FixedClock.
type Clock interface {
	Today() Date
}

// SystemClock reports today's date in the machine's local time zone.
type SystemClock struct{}

// Today implements Clock.
func (SystemClock) Today() Date {
	n := time.Now()
	return DateOf(n.Year(), n.Month(), n.Day())
}

// FixedClock always reports D.
type FixedClock struct{ D Date }

// Today implements Clock.
func (c FixedClock) Today() Date { return c.D }
