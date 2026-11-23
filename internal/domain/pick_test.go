package domain

import (
	"reflect"
	"testing"
)

func TestPick(t *testing.T) {
	cs := []Candidate{
		{1, "CLM"}, {2, "CLM tooling"}, {3, "Order platform modernisation"},
		{4, "Platform and tooling"}, {5, "Team and people"},
	}
	for _, c := range []struct {
		arg     string
		want    int64
		several []string
	}{
		{"1", 1, nil},
		{"9", 0, nil},      // no such id
		{"clm", 1, nil},    // equal wins over the prefix of "CLM tooling"
		{"clm t", 2, nil},  // prefix
		{"Order", 3, nil},  // prefix, any case
		{"opm", 3, nil},    // initials
		{"people", 5, nil}, // substring
		{"tooling", 0, []string{"CLM tooling", "Platform and tooling"}},
		{"pl", 4, nil}, // prefix of one beats substring of others
		{"zzz", 0, nil},
		{"  ", 0, nil},
	} {
		got, several, ok := Pick(c.arg, cs)
		if ok != (c.want != 0) || got.ID != c.want || !reflect.DeepEqual(several, c.several) {
			t.Errorf("Pick(%q) = %v, %v, %v; want %d, %v", c.arg, got, several, ok, c.want, c.several)
		}
	}
}
