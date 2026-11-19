package domain

import (
	"sort"
	"strings"
)

// Group is a project with the activities shown under it.
type Group struct {
	Project    Project
	Activities []Activity
}

func priorityRank(p Priority) int {
	switch p {
	case PriorityHigh:
		return 0
	case PriorityMedium:
		return 1
	case PriorityLow:
		return 2
	}
	return 3
}

// SortTasks orders tasks for a priority list: open first, then priority
// (high to none), then the oldest opened, then id.
func SortTasks(ts []Task) {
	sort.SliceStable(ts, func(i, j int) bool {
		a, b := ts[i], ts[j]
		if ao, bo := a.ClosedOn().IsZero(), b.ClosedOn().IsZero(); ao != bo {
			return ao
		}
		if ra, rb := priorityRank(a.Priority), priorityRank(b.Priority); ra != rb {
			return ra < rb
		}
		if a.CreatedOn != b.CreatedOn {
			return a.CreatedOn.Before(b.CreatedOn)
		}
		return a.ID < b.ID
	})
}

// SortActivities orders activities within a project (SPEC §5.1): not-done
// first, then priority, then due date, then id.
func SortActivities(as []Activity) {
	sort.SliceStable(as, func(i, j int) bool {
		a, b := as[i], as[j]
		if done := a.Status == StatusDone; done != (b.Status == StatusDone) {
			return !done
		}
		if ra, rb := priorityRank(a.Priority), priorityRank(b.Priority); ra != rb {
			return ra < rb
		}
		if a.Due != b.Due {
			return a.Due.Before(b.Due)
		}
		return a.ID < b.ID
	})
}

// bestRank is the best priority among a group's not-done activities; a group
// with only done activities ranks after every other.
func bestRank(g Group) int {
	best := priorityRank(PriorityNone) + 1
	for _, a := range g.Activities {
		if a.Status != StatusDone && priorityRank(a.Priority) < best {
			best = priorityRank(a.Priority)
		}
	}
	return best
}

// SortGroups orders projects (SPEC §5.1): by the best priority among their
// shown not-done activities, then by name.
func SortGroups(gs []Group) {
	sort.SliceStable(gs, func(i, j int) bool {
		a, b := gs[i], gs[j]
		if ra, rb := bestRank(a), bestRank(b); ra != rb {
			return ra < rb
		}
		if na, nb := strings.ToLower(a.Project.Name), strings.ToLower(b.Project.Name); na != nb {
			return na < nb
		}
		return a.Project.ID < b.Project.ID
	})
}
