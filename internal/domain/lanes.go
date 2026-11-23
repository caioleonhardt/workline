package domain

// Span is the inclusive days From..To an activity covers on the roadmap.
type Span struct{ From, To Date }

// Lanes packs spans into lanes, as on a roadmap where a project is one row
// and its activities are bars in it (SPEC §10 item 126): each span, in the
// given order, goes into the first lane whose last span ends before it
// starts, else a new one. Given spans sorted by start, no lane holds two
// overlapping spans and the lanes are as few as can be. It returns each
// span's lane (from 0) and the number of lanes.
func Lanes(spans []Span) (lane []int, n int) {
	var ends []Date // the last day taken in each lane
	lane = make([]int, len(spans))
	for i, s := range spans {
		assert(!s.To.Before(s.From), "span is inverted")
		l := 0
		for l < len(ends) && !ends[l].Before(s.From) {
			l++
		}
		if l == len(ends) {
			ends = append(ends, s.To)
		} else {
			ends[l] = s.To
		}
		lane[i] = l
	}
	return lane, len(ends)
}
