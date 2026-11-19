package domain

import "testing"

func d(t *testing.T, s string) Date {
	t.Helper()
	v, err := ParseDate(s)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func mustPanic(t *testing.T, name string, f func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Errorf("%s: expected panic", name)
		}
	}()
	f()
}

// sub builds a task; closed is "" for open.
func sub(t *testing.T, id int64, created, closed string) Task {
	t.Helper()
	s := Task{ID: id, CreatedOn: d(t, created)}
	if closed != "" {
		s.ClosedOn = d(t, closed)
	}
	return s
}
