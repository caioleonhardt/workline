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
	KindTask     Kind = "task"
)

// Valid reports whether k is a known kind.
func (k Kind) Valid() bool {
	switch k {
	case KindUpdate, KindDecision, KindBlocker, KindMeeting, KindScopeAck, KindTask:
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
	CreatedOn   Date // the local day it was created
}

// Task is one checkable item of an activity: the view of a journal entry of
// kind task that the burndown, progress and scope rules work on.
type Task struct {
	ID         int64
	ActivityID int64 // 0: Unassigned
	Text       string
	CreatedOn  Date
	ClosedAt   string // "YYYY-MM-DD HH:MM" local time it was closed; empty while open
	Priority   Priority
}

// ClosedOn is the day the task was closed, zero while open.
func (s Task) ClosedOn() Date { return stampDate(s.ClosedAt) }

// JournalEntry is a dated note on an activity, or Unassigned when ActivityID
// is 0. An entry of kind task is a task opened on Date; only tasks have
// ClosedAt.
type JournalEntry struct {
	ID         int64
	ActivityID int64 // 0: Unassigned
	Date       Date
	Kind       Kind
	Text       string
	LoggedAt   string // "YYYY-MM-DD HH:MM" local time it was logged; empty for older entries
	ClosedAt   string
	Priority   Priority // tasks only
	Starred    bool     // picked for the 1:1 report's highlights
}

// ClosedOn is the day a task entry was closed, zero while open or for other kinds.
func (j JournalEntry) ClosedOn() Date { return stampDate(j.ClosedAt) }

// LogDate is the day the entry sits at in the journal: a closed task's close
// day, else its date.
func (j JournalEntry) LogDate() Date {
	if c := j.ClosedOn(); !c.IsZero() {
		return c
	}
	return j.Date
}

// LogTime is the "HH:MM" that goes with LogDate, or "" for none.
func (j JournalEntry) LogTime() string {
	if j.ClosedAt != "" {
		return StampTime(j.ClosedAt)
	}
	return j.Time()
}

// Task returns the entry as a task.
func (j JournalEntry) Task() Task {
	return Task{ID: j.ID, ActivityID: j.ActivityID, Text: j.Text, CreatedOn: j.Date, ClosedAt: j.ClosedAt, Priority: j.Priority}
}

// stampLayout is the format of JournalEntry.LoggedAt.
const stampLayout = "2006-01-02 15:04"

// Stamp formats the wall-clock fields of t as a LoggedAt value.
func Stamp(t time.Time) string { return t.Format(stampLayout) }

// stampDate returns the day of a stamp, or the zero Date for none or a bad one.
func stampDate(stamp string) Date {
	if len(stamp) < 10 {
		return 0
	}
	d, _ := ParseDate(stamp[:10]) // zero when invalid
	return d
}

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
	LastTaskID    int64
}

// ActivityWithTasks pairs an activity with its tasks and its latest
// scope acknowledgement, if any.
type ActivityWithTasks struct {
	Activity
	Tasks []Task
	Ack   *ScopeAck
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

// Validate checks the task's own fields. created_on may precede the
// activity's start (SPEC §10 decision 10).
func (s Task) Validate() error {
	switch {
	case blank(s.Text):
		return Invalid("task text is required")
	case s.CreatedOn.IsZero():
		return Invalid("task created date is required")
	case s.ClosedAt != "" && !validStamp(s.ClosedAt):
		return Invalid("closed time %q is not YYYY-MM-DD HH:MM", s.ClosedAt)
	case s.ClosedAt != "" && s.ClosedOn().Before(s.CreatedOn):
		return Invalid("closed date %s is before created date %s", s.ClosedOn(), s.CreatedOn)
	}
	return nil
}

// ValidateAt is Validate plus: a task cannot be closed after today.
func (s Task) ValidateAt(today Date) error {
	if err := s.Validate(); err != nil {
		return err
	}
	if s.ClosedOn().After(today) {
		return Invalid("closed date %s is after today (%s)", s.ClosedOn(), today)
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
	case j.Kind == KindScopeAck && j.ActivityID == 0:
		return Invalid("an acknowledgement belongs to an activity")
	case j.LoggedAt != "" && !validStamp(j.LoggedAt):
		return Invalid("logged time %q is not YYYY-MM-DD HH:MM", j.LoggedAt)
	case !j.Priority.Valid():
		return Invalid("unknown priority %q", j.Priority)
	case j.Kind == KindTask:
		return j.Task().Validate()
	case j.ClosedAt != "":
		return Invalid("only a task can be closed")
	case j.Priority != PriorityNone:
		return Invalid("only a task has a priority")
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
