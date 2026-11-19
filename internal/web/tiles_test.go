package web

import (
	"encoding/json"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"testing"

	"workline/internal/domain"
)

func tileValue(t *testing.T, body, label string) int {
	t.Helper()
	m := regexp.MustCompile(label + `</div><div class="tile-value mono">(\d+)</div>`).FindStringSubmatch(body)
	if m == nil {
		t.Fatalf("tile %q not found", label)
	}
	n, _ := strconv.Atoi(m[1])
	return n
}

// TestTilesMatchBaselineGolden renders the dashboard at the baseline's as_of
// date and compares the tiles with the baseline's expected block.
func TestTilesMatchBaselineGolden(t *testing.T) {
	raw, err := os.ReadFile("../store/seed/baseline.json")
	if err != nil {
		t.Fatal(err)
	}
	var golden struct {
		Expected struct {
			Tiles map[string]int `json:"tiles"`
		} `json:"expected"`
	}
	if err := json.Unmarshal(raw, &golden); err != nil {
		t.Fatal(err)
	}
	h, _ := newApp(t)
	body := get(h, "/").Body.String()
	for label, key := range map[string]string{
		"Active": "active", "Blocked": "blocked", "Planned": "planned", "Done this year": "done",
		"Open tasks now": "open_tasks", "Scope creep flags": "scope_flags",
	} {
		if got, want := tileValue(t, body, label), golden.Expected.Tiles[key]; got != want {
			t.Errorf("tile %s = %d, want %d", label, got, want)
		}
	}
}

func TestTilesFollowProjectFilterOnly(t *testing.T) {
	h, st := newApp(t)
	// project 3 alone: three active activities, none flagged; period and status filters do not matter
	body := get(h, "/?project=3&period=q1&status=").Body.String()
	if tileValue(t, body, "Active") != 3 || tileValue(t, body, "Scope creep flags") != 0 || tileValue(t, body, "Done this year") != 0 {
		t.Errorf("project 3 tiles wrong")
	}
	// done activities leave "open tasks now"; their done date drives "Done this year"
	if err := st.SetActivityStatus(ctx, 2, domain.StatusDone, date(t, testToday)); err != nil {
		t.Fatal(err)
	}
	body = get(h, "/").Body.String()
	if got := tileValue(t, body, "Open tasks now"); got != 13 { // 17 minus Kafka's 4
		t.Errorf("open tasks = %d", got)
	}
	if tileValue(t, body, "Done this year") != 4 || tileValue(t, get(h, "/?year=2025").Body.String(), "Done this year") != 0 {
		t.Error("done this year follows the selected year")
	}
	// archived projects are not counted
	if err := st.SetProjectArchived(ctx, 1, true); err != nil {
		t.Fatal(err)
	}
	if got := tileValue(t, get(h, "/").Body.String(), "Active"); got != 3 {
		t.Errorf("active with project 1 archived = %d", got)
	}
	wantStatus(t, get(h, "/"), http.StatusOK)
}
