package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrInvalid matches (errors.Is) every validation failure.
var ErrInvalid = errors.New("invalid value")

// InvalidError is a validation failure. Message is written for the person who
// made the mistake; wrapping context added around the error is for logs.
type InvalidError struct{ Message string }

func (e *InvalidError) Error() string { return "invalid value: " + e.Message }

// Is makes errors.Is(err, ErrInvalid) true.
func (e *InvalidError) Is(target error) bool { return target == ErrInvalid }

// Invalid returns an InvalidError with a formatted message.
func Invalid(format string, args ...any) error {
	return &InvalidError{Message: fmt.Sprintf(format, args...)}
}

// Status is an activity's lifecycle state.
type Status string

// Activity statuses.
const (
	StatusPlanned Status = "planned"
	StatusActive  Status = "active"
	StatusBlocked Status = "blocked"
	StatusDone    Status = "done"
)

// Valid reports whether s is a known status.
func (s Status) Valid() bool {
	switch s {
	case StatusPlanned, StatusActive, StatusBlocked, StatusDone:
		return true
	}
	return false
}

// Kind is a journal entry kind.
type Kind string

// Journal kinds.
const (
	KindUpdate   Kind = "update"
	KindDecision Kind = "decision"
	KindBlocker  Kind = "blocker"
	KindMeeting  Kind = "meeting"
	KindScopeAck Kind = "scope_ack"
)

// Valid reports whether k is a known kind.
func (k Kind) Valid() bool {
	switch k {
	case KindUpdate, KindDecision, KindBlocker, KindMeeting, KindScopeAck:
		return true
	}
	return false
}

// Priority orders activities; the empty value means "no priority".
type Priority string

// Priorities.
const (
	PriorityNone   Priority = ""
	PriorityHigh   Priority = "high"
	PriorityMedium Priority = "medium"
	PriorityLow    Priority = "low"
)

// Valid reports whether p is a known priority or empty.
func (p Priority) Valid() bool {
	switch p {
	case PriorityNone, PriorityHigh, PriorityMedium, PriorityLow:
		return true
	}
	return false
}

// Project groups activities.
type Project struct {
	ID       int64
	Name     string
	Archived bool
}

// Activity is a piece of work someone asked for.
type Activity struct {
	ID          int64
	ProjectID   int64
	Name        string
	RequestedBy string
	Status      Status
	Priority    Priority
	Start       Date
	Due         Date
	DoneOn      Date // zero unless Status is done
}

// Subtask is one checkable item of an activity. ClosedOn is zero while open.
type Subtask struct {
	ID         int64
	ActivityID int64
	Text       string
	CreatedOn  Date
	ClosedOn   Date
	ClosedAt   string // "YYYY-MM-DD HH:MM" local time it was closed; empty if open or unknown
}

// JournalEntry is a dated note on an activity.
type JournalEntry struct {
	ID         int64
	ActivityID int64
	Date       Date
	Kind       Kind
	Text       string
	LoggedAt   string // "YYYY-MM-DD HH:MM" local time it was logged; empty for older entries
}

// stampLayout is the format of JournalEntry.LoggedAt.
const stampLayout = "2006-01-02 15:04"

// Stamp formats the wall-clock fields of t as a LoggedAt value.
func Stamp(t time.Time) string { return t.Format(stampLayout) }

// StampTime returns the "HH:MM" part of a LoggedAt value, or "" for none.
func StampTime(stamp string) string {
	if stamp == "" {
		return ""
	}
	return stamp[len(stampLayout)-5:]
}

// Time returns the "HH:MM" part of LoggedAt, or "" when there is none.
func (j JournalEntry) Time() string { return StampTime(j.LoggedAt) }

// ScopeAck records an acknowledged scope change (SPEC §3).
type ScopeAck struct {
	ID            int64
	ActivityID    int64
	Date          Date
	BaselineCount int
	LastSubtaskID int64
}

// ActivityWithSubtasks pairs an activity with its subtasks and its latest
// scope acknowledgement, if any.
type ActivityWithSubtasks struct {
	Activity
	Subtasks []Subtask
	Ack      *ScopeAck
}

func validStamp(s string) bool {
	_, err := time.Parse(stampLayout, s)
	return err == nil && len(s) == len(stampLayout)
}

func blank(s string) bool { return strings.TrimSpace(s) == "" }

// Validate checks the activity's own fields (not its ID or ProjectID).
func (a Activity) Validate() error {
	switch {
	case blank(a.Name):
		return Invalid("activity name is required")
	case !a.Status.Valid():
		return Invalid("unknown status %q", a.Status)
	case !a.Priority.Valid():
		return Invalid("unknown priority %q", a.Priority)
	case !a.DoneOn.IsZero() && a.Status != StatusDone:
		return Invalid("done date set on a %s activity", a.Status)
	case a.Start.IsZero() || a.Due.IsZero():
		return Invalid("start and due dates are required")
	case a.Due.Before(a.Start):
		return Invalid("due date %s is before start date %s", a.Due, a.Start)
	case a.Due.Sub(a.Start) > maxSpanDays:
		return Invalid("an activity can span at most %d days", maxSpanDays)
	}
	return nil
}

// Validate checks the subtask's own fields. created_on may precede the
// activity's start (SPEC §10 decision 10).
func (s Subtask) Validate() error {
	switch {
	case blank(s.Text):
		return Invalid("subtask text is required")
	case s.CreatedOn.IsZero():
		return Invalid("subtask created date is required")
	case !s.ClosedOn.IsZero() && s.ClosedOn.Before(s.CreatedOn):
		return Invalid("closed date %s is before created date %s", s.ClosedOn, s.CreatedOn)
	case s.ClosedAt != "" && !validStamp(s.ClosedAt):
		return Invalid("closed time %q is not YYYY-MM-DD HH:MM", s.ClosedAt)
	case s.ClosedAt != "" && (s.ClosedOn.IsZero() || s.ClosedAt[:10] != s.ClosedOn.String()):
		return Invalid("closed time %q does not match closed date %q", s.ClosedAt, s.ClosedOn)
	}
	return nil
}

// ValidateAt is Validate plus: a subtask cannot be closed after today.
func (s Subtask) ValidateAt(today Date) error {
	if err := s.Validate(); err != nil {
		return err
	}
	if s.ClosedOn.After(today) {
		return Invalid("closed date %s is after today (%s)", s.ClosedOn, today)
	}
	return nil
}

// Validate checks the journal entry's own fields.
func (j JournalEntry) Validate() error {
	switch {
	case blank(j.Text):
		return Invalid("journal text is required")
	case j.Date.IsZero():
		return Invalid("journal date is required")
	case !j.Kind.Valid():
		return Invalid("unknown journal kind %q", j.Kind)
	case j.LoggedAt != "" && !validStamp(j.LoggedAt):
		return Invalid("logged time %q is not YYYY-MM-DD HH:MM", j.LoggedAt)
	}
	return nil
}

// DefaultStatus is the status of a newly created activity.
func DefaultStatus(start, today Date) Status {
	if start.After(today) {
		return StatusPlanned
	}
	return StatusActive
}
