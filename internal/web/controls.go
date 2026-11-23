package web

import (
	"fmt"
	"net/url"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"workline/internal/domain"
)

// pair is one hidden form input.
type pair struct{ Key, Value string }

func pairs(q url.Values) []pair {
	var out []pair
	for k, vs := range q {
		for _, v := range vs {
			out = append(out, pair{k, v})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

// toggle is a button in its own GET form; Pairs is the state a click leads to.
type toggle struct {
	Label    string
	Class    string // status colour class, if any
	Count    int
	HasCount bool
	Pressed  bool
	Pairs    []pair
}

type controlsView struct {
	Eyebrow            string
	Year               int
	PrevYear, NextYear string   // dashboard URLs
	Periods, Scales    []toggle // Scales is empty outside the Year view
	Pills              []toggle
	Creep              toggle
	Projects, Months   []option
	SelectPairs        []pair // state the selects' form keeps
	ClearHref          string // empty unless a filter is active
}

var periodLabels = []string{"Year", "Q1", "Q2", "Q3", "Q4"}

func buildControls(f Filter, projects []domain.Project, counts map[domain.Status]int, creepCount int) controlsView {
	v := controlsView{Year: f.Year, PrevYear: f.WithYear(f.Year - 1).URL(), NextYear: f.WithYear(f.Year + 1).URL()}
	if f.Period == domain.PeriodYear {
		v.Eyebrow = fmt.Sprintf("%d · ROADMAP", f.Year)
	} else {
		first := time.Month(3*(int(f.Period)-1) + 1)
		v.Eyebrow = fmt.Sprintf("Q%d %d · %s–%s", f.Period, f.Year,
			strings.ToUpper(first.String()[:3]), strings.ToUpper((first + 2).String()[:3]))
	}
	for i, label := range periodLabels {
		p := domain.Period(i)
		v.Periods = append(v.Periods, toggle{Label: label, Pressed: f.Period == p, Pairs: pairs(f.WithPeriod(p).Values())})
	}
	if f.Period == domain.PeriodYear {
		for i, label := range []string{"Quarters", "Months"} {
			sc := domain.Scale(i)
			v.Scales = append(v.Scales, toggle{Label: label, Pressed: f.Scale == sc, Pairs: pairs(f.WithScale(sc).Values())})
		}
	}
	for _, st := range allStatuses {
		label := strings.ToUpper(string(st)[:1]) + string(st)[1:]
		v.Pills = append(v.Pills, toggle{Label: label, Class: string(st), Count: counts[st], HasCount: true,
			Pressed: slices.Contains(f.Statuses, st), Pairs: pairs(f.ToggleStatus(st).Values())})
	}
	v.Creep = toggle{Label: "Scope creep", Count: creepCount, HasCount: true, Pressed: f.Creep, Pairs: pairs(f.ToggleCreep().Values())}

	v.Projects = []option{{"", "All projects", f.Project == 0}}
	for _, p := range projects {
		v.Projects = append(v.Projects, option{strconv.FormatInt(p.ID, 10), p.Name, p.ID == f.Project})
	}
	all := "All of " + strconv.Itoa(f.Year)
	months := make([]time.Month, 0, 12)
	for m := time.January; m <= time.December; m++ {
		if f.monthAllowed(m) {
			months = append(months, m)
		}
	}
	if f.Period != domain.PeriodYear {
		all = fmt.Sprintf("Whole Q%d", f.Period)
	}
	v.Months = []option{{"", all, f.Month == 0}}
	for _, m := range months {
		v.Months = append(v.Months, option{strconv.Itoa(int(m)), fmt.Sprintf("%s %d", m.String()[:3], f.Year), m == f.Month})
	}
	sf := f
	sf.Project, sf.Month = 0, 0
	v.SelectPairs = pairs(sf.Values())
	if f.Filtered() {
		v.ClearHref = f.Cleared().URL()
	}
	return v
}
