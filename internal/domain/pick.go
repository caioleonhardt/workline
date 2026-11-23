package domain

import (
	"strconv"
	"strings"
)

// Candidate is something a command can name: a project or an activity.
type Candidate struct {
	ID   int64
	Name string
}

// Pick finds the candidate arg names (SPEC §10 item 121): a number is an id;
// otherwise the name, ignoring case, in order of preference: equal to arg,
// starting with it, with initials equal to it ("opm" for "Order platform
// modernisation"), then containing it. The first rule with matches decides;
// if it has several, Pick returns their names and ok is false. No match at
// all returns neither.
func Pick(arg string, cs []Candidate) (c Candidate, several []string, ok bool) {
	arg = strings.ToLower(strings.TrimSpace(arg))
	if arg == "" {
		return Candidate{}, nil, false
	}
	if id, err := strconv.ParseInt(arg, 10, 64); err == nil {
		for _, c := range cs {
			if c.ID == id {
				return c, nil, true
			}
		}
		return Candidate{}, nil, false
	}
	rules := []func(name string) bool{
		func(name string) bool { return name == arg },
		func(name string) bool { return strings.HasPrefix(name, arg) },
		func(name string) bool { return initials(name) == arg },
		func(name string) bool { return strings.Contains(name, arg) },
	}
	for _, rule := range rules {
		var hits []Candidate
		for _, c := range cs {
			if rule(strings.ToLower(c.Name)) {
				hits = append(hits, c)
			}
		}
		switch len(hits) {
		case 0:
			continue
		case 1:
			return hits[0], nil, true
		}
		for _, h := range hits {
			several = append(several, h.Name)
		}
		return Candidate{}, several, false
	}
	return Candidate{}, nil, false
}

// initials is the first letter of each word of name.
func initials(name string) string {
	var b strings.Builder
	for _, w := range strings.Fields(name) {
		b.WriteString(w[:1])
	}
	return b.String()
}
