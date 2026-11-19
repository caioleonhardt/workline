package domain

import (
	"strings"
	"unicode"
)

// QueryError is a refused search query. Message reads like Vim's messages
// ("E475: Invalid argument: p:9"), since it is shown in the statusline.
type QueryError struct{ Message string }

func (e *QueryError) Error() string { return e.Message }

// Query is a parsed search (SPEC §10 item 102): every term must hold.
type Query struct{ terms []term }

type term struct {
	neg   bool
	holds func(e JournalEntry, project, activity string) bool
}

// QueryKey is a qualifier and the values it takes; Values is empty for keys
// that take a name (proj:, act:).
type QueryKey struct {
	Key    string
	Values []string
}

// QueryKeys lists the qualifiers in the order completion offers them.
var QueryKeys = []QueryKey{
	{"p", []string{"1", "2", "3", "high", "medium", "low"}},
	{"k", []string{"t", "d", "u", "b", "m", "ack", "task", "decision", "update", "blocker", "meeting", "scope_ack"}},
	{"proj", nil},
	{"act", nil},
	{"is", []string{"open", "closed", "unassigned"}},
}

var queryPriorities = map[string]Priority{
	"1": PriorityHigh, "high": PriorityHigh, "2": PriorityMedium, "medium": PriorityMedium, "3": PriorityLow, "low": PriorityLow,
}

var queryKinds = map[string]Kind{
	"t": KindTask, "task": KindTask, "d": KindDecision, "decision": KindDecision, "u": KindUpdate, "update": KindUpdate,
	"b": KindBlocker, "blocker": KindBlocker, "m": KindMeeting, "meeting": KindMeeting, "ack": KindScopeAck, "scope_ack": KindScopeAck,
}

// ParseQuery parses a search. Tokens are separated by spaces outside double
// quotes; a leading ! negates one; a known key: qualifies one, any other
// token (including an unknown key:) is text the entry must contain.
func ParseQuery(s string) (Query, error) {
	tokens, err := splitQuery(s)
	if err != nil {
		return Query{}, err
	}
	var q Query
	for _, tok := range tokens {
		t, ok, err := parseTerm(tok)
		if err != nil {
			return Query{}, err
		}
		if ok {
			q.terms = append(q.terms, t)
		}
	}
	if len(q.terms) == 0 {
		return Query{}, &QueryError{"E35: No previous regular expression"}
	}
	return q, nil
}

// splitQuery splits s at spaces outside double quotes, keeping the quotes.
func splitQuery(s string) ([]string, error) {
	var out []string
	var cur strings.Builder
	quoted, start := false, 0
	for i, r := range s {
		switch {
		case r == '"':
			if !quoted {
				start = i
			}
			quoted = !quoted
			cur.WriteRune(r)
		case unicode.IsSpace(r) && !quoted:
			if cur.Len() > 0 {
				out = append(out, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(r)
		}
	}
	if quoted {
		return nil, &QueryError{"E114: Missing quote: " + s[start:]}
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out, nil
}

// parseTerm reads one token; ok is false for a token with nothing to match ("").
func parseTerm(tok string) (term, bool, error) {
	t := term{}
	body := tok
	if len(body) > 1 && body[0] == '!' {
		t.neg, body = true, body[1:]
	}
	key, raw, hasKey := strings.Cut(body, ":")
	value := strings.ToLower(strings.ReplaceAll(raw, `"`, ""))
	bad := &QueryError{"E475: Invalid argument: " + tok}
	if hasKey {
		switch key {
		case "p":
			p, ok := queryPriorities[value]
			if !ok {
				return t, false, bad
			}
			t.holds = func(e JournalEntry, _, _ string) bool { return e.Priority == p }
			return t, true, nil
		case "k":
			k, ok := queryKinds[value]
			if !ok {
				return t, false, bad
			}
			t.holds = func(e JournalEntry, _, _ string) bool { return e.Kind == k }
			return t, true, nil
		case "proj", "act":
			if value == "" {
				return t, false, bad
			}
			proj := key == "proj"
			t.holds = func(_ JournalEntry, project, activity string) bool {
				name := activity
				if proj {
					name = project
				}
				return strings.HasPrefix(strings.ToLower(name), value)
			}
			return t, true, nil
		case "is":
			switch value {
			case "open", "closed":
				closed := value == "closed"
				t.holds = func(e JournalEntry, _, _ string) bool { return e.Kind == KindTask && e.ClosedOn().IsZero() != closed }
			case "unassigned":
				t.holds = func(e JournalEntry, _, _ string) bool { return e.ActivityID == 0 }
			default:
				return t, false, bad
			}
			return t, true, nil
		}
	}
	text := strings.ToLower(strings.ReplaceAll(body, `"`, ""))
	if text == "" {
		return t, false, nil
	}
	t.holds = func(e JournalEntry, _, _ string) bool { return strings.Contains(strings.ToLower(e.Text), text) }
	return t, true, nil
}

// Match reports whether the entry, with its project and activity names (empty
// when Unassigned), satisfies every term.
func (q Query) Match(e JournalEntry, project, activity string) bool {
	for _, t := range q.terms {
		if t.holds(e, project, activity) == t.neg {
			return false
		}
	}
	return true
}
