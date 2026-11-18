package domain

// percent returns round(n/d*100), half up, using integer arithmetic.
func percent(n, d int) int {
	assert(d > 0 && n >= 0, "percent needs d > 0 and n >= 0")
	return (200*n + d) / (2 * d)
}

// OpenAt counts subtasks created on or before t and not closed by t (§4.1).
func OpenAt(subs []Subtask, t Date) int {
	n := 0
	for _, s := range subs {
		if !s.CreatedOn.After(t) && (s.ClosedOn.IsZero() || s.ClosedOn.After(t)) {
			n++
		}
	}
	return n
}

// ScopeAt counts subtasks created on or before t (§4.1).
func ScopeAt(subs []Subtask, t Date) int {
	n := 0
	for _, s := range subs {
		if !s.CreatedOn.After(t) {
			n++
		}
	}
	return n
}

func countClosed(subs []Subtask) int {
	n := 0
	for _, s := range subs {
		if !s.ClosedOn.IsZero() {
			n++
		}
	}
	return n
}

// Progress is an activity's completion percentage (§4.2).
func Progress(status Status, subs []Subtask) int {
	if len(subs) == 0 {
		if status == StatusDone {
			return 100
		}
		return 0
	}
	return percent(countClosed(subs), len(subs))
}

// ProjectProgress is closed/total over the given subtasks (the subtasks of the
// project's visible activities). It is 0 when there are none.
func ProjectProgress(subs []Subtask) int {
	if len(subs) == 0 {
		return 0
	}
	return percent(countClosed(subs), len(subs))
}
