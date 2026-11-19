package domain

// Tiles are the dashboard's summary numbers (SPEC §5.1).
type Tiles struct {
	Active, Blocked, Planned int
	DoneThisYear             int // done activities whose done date falls in the year
	OpenTasks                int // of activities that are not done
	ScopeFlags               int
}

// ComputeTiles summarises the given activities as of today.
func ComputeTiles(items []ActivityWithTasks, year int, today Date) Tiles {
	var t Tiles
	for _, it := range items {
		switch it.Status {
		case StatusActive:
			t.Active++
		case StatusBlocked:
			t.Blocked++
		case StatusPlanned:
			t.Planned++
		case StatusDone:
			if !it.DoneOn.IsZero() && it.DoneOn.Year() == year {
				t.DoneThisYear++
			}
			continue
		}
		t.OpenTasks += OpenAt(it.Tasks, today)
		if ScopeCreep(it.Activity, it.Tasks, it.Ack, today).Flagged {
			t.ScopeFlags++
		}
	}
	return t
}
