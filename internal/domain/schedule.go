package domain

// LateTolerancePct is how far past its due date, as a share of its planned
// span, an activity may close before it counts as badly late (SPEC §10 item 128).
const LateTolerancePct = 20

// End is the last day the activity covers: its due date, or the day it was
// closed when that came later.
func (a Activity) End() Date {
	if a.Status == StatusDone && a.DoneOn.After(a.Due) {
		return a.DoneOn
	}
	return a.Due
}

// Overrun tells whether a done activity closed after its due date, and the
// last day of the tolerance: due plus LateTolerancePct of its planned span
// (start..due), rounded up. Days up to it are late; days after it, very late.
func (a Activity) Overrun() (toleratedUntil Date, late bool) {
	if a.End() == a.Due {
		return 0, false
	}
	planned := a.Due.Sub(a.Start) + 1
	return a.Due.AddDays((planned*LateTolerancePct + 99) / 100), true
}
