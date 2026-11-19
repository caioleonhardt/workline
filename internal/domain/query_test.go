package domain

import (
	"errors"
	"testing"
)

func TestQueryMatch(t *testing.T) {
	closed := d(t, "2026-11-10")
	open := JournalEntry{ID: 1, ActivityID: 7, Kind: KindTask, Text: "Review the API deploy", Priority: PriorityHigh}
	done := JournalEntry{ID: 2, ActivityID: 7, Kind: KindTask, Text: "Draft plan", Priority: PriorityLow, ClosedOn: closed}
	note := JournalEntry{ID: 3, ActivityID: 8, Kind: KindDecision, Text: "Go with code review on Fridays, see https://x.test/a"}
	loose := JournalEntry{ID: 4, Kind: KindMeeting, Text: "1:1 notes"}
	ack := JournalEntry{ID: 5, ActivityID: 8, Kind: KindScopeAck, Text: "Scope acknowledged"}
	names := map[int64][2]string{7: {"Atlas", "Migration to Postgres"}, 8: {"Big Thing", "Hiring"}}
	all := []JournalEntry{open, done, note, loose, ack}

	for _, c := range []struct {
		q    string
		want []int64
	}{
		{"review", []int64{1, 3}},
		{"REVIEW", []int64{1, 3}},
		{"review deploy", []int64{1}},
		{"!review", []int64{2, 4, 5}},
		{`"code review"`, []int64{3}},
		{`!"code review" review`, []int64{1}},
		{"p:1", []int64{1}},
		{"p:high", []int64{1}},
		{"p:2", nil},
		{"p:low", []int64{2}},
		{"!p:1", []int64{2, 3, 4, 5}},
		{"k:t", []int64{1, 2}},
		{"k:decision", []int64{3}},
		{"k:m", []int64{4}},
		{"k:ack", []int64{5}},
		{"!k:task", []int64{3, 4, 5}},
		{"proj:atl", []int64{1, 2}},
		{`proj:"big thing"`, []int64{3, 5}},
		{"proj:thing", nil}, // a prefix, not a substring
		{"act:migra", []int64{1, 2}},
		{"!act:hir", []int64{1, 2, 4}},
		{"is:open", []int64{1}},
		{"is:closed", []int64{2}},
		{"!is:open", []int64{2, 3, 4, 5}},
		{"is:unassigned", []int64{4}},
		{"k:t !is:closed p:1 proj:atlas", []int64{1}},
		{"https://x.test/a", []int64{3}}, // an unknown key: is text
		{"1:1", []int64{4}},
		{`"" plan`, []int64{2}}, // an empty phrase matches nothing extra
		{"  plan  ", []int64{2}},
	} {
		q, err := ParseQuery(c.q)
		if err != nil {
			t.Errorf("%q: %v", c.q, err)
			continue
		}
		var got []int64
		for _, e := range all {
			n := names[e.ActivityID]
			if q.Match(e, n[0], n[1]) {
				got = append(got, e.ID)
			}
		}
		if len(got) != len(c.want) {
			t.Errorf("%q matched %v, want %v", c.q, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("%q matched %v, want %v", c.q, got, c.want)
				break
			}
		}
	}
}

func TestParseQueryErrors(t *testing.T) {
	for _, c := range []struct{ q, msg string }{
		{"", "E35: No previous regular expression"},
		{"   ", "E35: No previous regular expression"},
		{`""`, "E35: No previous regular expression"},
		{"p:9", "E475: Invalid argument: p:9"},
		{"p:", "E475: Invalid argument: p:"},
		{"plan !k:x", "E475: Invalid argument: !k:x"},
		{"is:done", "E475: Invalid argument: is:done"},
		{"proj:", "E475: Invalid argument: proj:"},
		{`act:""`, `E475: Invalid argument: act:""`},
		{`plan "code rev`, `E114: Missing quote: "code rev`},
	} {
		_, err := ParseQuery(c.q)
		var qe *QueryError
		if !errors.As(err, &qe) || qe.Error() != c.msg {
			t.Errorf("%q: got %v, want %q", c.q, err, c.msg)
		}
	}
}

func TestQueryKeysParse(t *testing.T) {
	for _, k := range QueryKeys {
		for _, v := range k.Values {
			if _, err := ParseQuery(k.Key + ":" + v); err != nil {
				t.Errorf("completion offers %s:%s, which is refused: %v", k.Key, v, err)
			}
		}
	}
}
