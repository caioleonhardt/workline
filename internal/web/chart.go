package web

import (
	"fmt"

	"workline/internal/domain"
)

// chartBin is one week of the panel's burndown. Heights are percentages of the
// activity's total subtasks, as in the mockup.
type chartBin struct {
	Scope, Open pct
	Ahead       bool // starts after today: drawn as a dashed frame
	Today       bool
	Tip         string
}

type chartRow struct {
	Week        string
	Open, Scope int
	Ahead       bool
}

type chartView struct {
	Bins                 []chartBin
	Rows                 []chartRow // the same data as a table for screen readers
	Peak                 int        // most open subtasks at any week measured so far
	StartLabel, EndLabel string
	Summary              string
}

func buildChart(a domain.Activity, subs []domain.Subtask, today domain.Date) chartView {
	total := max(1, len(subs))
	v := chartView{StartLabel: a.Start.String(), EndLabel: a.Due.String()}
	for _, b := range domain.BurndownDetail(a, subs, today) {
		cb := chartBin{Ahead: b.Ahead, Today: !today.Before(b.Start) && !today.After(b.End)}
		week := "Week of " + b.Start.String()
		if b.Ahead {
			cb.Scope, cb.Tip = 100, week+": ahead"
		} else {
			cb.Scope = pct(float64(b.Scope) / float64(total) * 100)
			cb.Open = pct(float64(b.Open) / float64(total) * 100)
			cb.Tip = fmt.Sprintf("%s: %d open of %d", week, b.Open, b.Scope)
			v.Peak = max(v.Peak, b.Open)
		}
		v.Bins = append(v.Bins, cb)
		v.Rows = append(v.Rows, chartRow{Week: b.Start.String(), Open: b.Open, Scope: b.Scope, Ahead: b.Ahead})
	}
	v.Summary = fmt.Sprintf("Burndown by week: peak %d open of %d subtasks", v.Peak, len(subs))
	return v
}
