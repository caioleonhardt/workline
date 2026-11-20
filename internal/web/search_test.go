package web

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

func searchURL(q string, extra ...string) string {
	v := url.Values{"q": {q}}
	for i := 0; i+1 < len(extra); i += 2 {
		v.Set(extra[i], extra[i+1])
	}
	return "/search?" + v.Encode()
}

func TestSearchPage(t *testing.T) {
	h, st := newApp(t)
	wantStatus(t, post(h, "/journal", url.Values{"activity": {"2"}, "kind": {"task"}, "priority": {"high"}, "text": {"Zebra rollout plan"}}), 303)
	wantStatus(t, post(h, "/journal", url.Values{"activity": {"2"}, "kind": {"decision"}, "text": {"Zebra goes first"}}), 303)
	wantStatus(t, post(h, "/journal", url.Values{"kind": {"meeting"}, "text": {"Zebra sync"}}), 303) // Unassigned

	rec := get(h, searchURL("zebra", "from", "/timeline?sel=2"))
	wantStatus(t, rec, 200)
	wantBody(t, rec, `<div id="dash" class="search-page"`, `data-search="zebra" data-count="3"`, "3 results",
		"Zebra rollout plan", "Zebra goes first", "Zebra sync", `<a class="btn secondary clear-search" href="/timeline?sel=2"`,
		`<input type="hidden" name="next" value="/search?from=%2Ftimeline%3Fsel%3D2&amp;q=zebra">`, `id="quickadd"`, `<div class="status mono">`)
	if strings.Index(rec.Body.String(), "Zebra sync") > strings.Index(rec.Body.String(), "Zebra rollout plan") {
		t.Error("results must be newest first")
	}

	rec = get(h, searchURL("zebra k:t p:1"))
	wantBody(t, rec, "1 result<", "Zebra rollout plan")
	if strings.Contains(rec.Body.String(), "Zebra goes first") {
		t.Error("k:t p:1 matched a decision")
	}
	if body := get(h, searchURL("zebra !is:unassigned")).Body.String(); strings.Contains(body, "Zebra sync") || !strings.Contains(body, "2 results") {
		t.Error("!is:unassigned kept the Unassigned entry")
	}

	rec = get(h, searchURL("p:9", "from", "https://evil.test/"))
	wantStatus(t, rec, 200)
	wantBody(t, rec, `<p class="error-bar mono" role="alert">E475: Invalid argument: p:9</p>`, `class="btn secondary clear-search" href="/"`)

	// Archived projects are left out (SPEC §10 item 33).
	if err := st.SetProjectArchived(ctx, 4, true); err != nil { // holds "Request received in 1:1"
		t.Fatal(err)
	}
	if strings.Contains(get(h, searchURL("request")).Body.String(), "Request received in 1:1") {
		t.Error("archived project's entry in the results")
	}

	frag := hxGet(h, searchURL("zebra"))
	wantStatus(t, frag, 200)
	if b := frag.Body.String(); strings.Contains(b, "<html") || !strings.HasPrefix(strings.TrimSpace(b), `<div id="dash" class="search-page"`) || !strings.Contains(b, `id="quickadd-target" hx-swap-oob`) {
		t.Errorf("HX search is not the fragment:\n%s", b)
	}
}

func TestSearchPageShowsMore(t *testing.T) {
	h, _ := newApp(t)
	for i := 0; i < 31; i++ {
		post(h, "/journal", url.Values{"kind": {"update"}, "text": {"Quokka note"}})
	}
	rec := get(h, searchURL("quokka"))
	wantBody(t, rec, "31 results", `href="/search?feed=60&amp;q=quokka">Show 30 more</a>`)
	if n := strings.Count(rec.Body.String(), "Quokka note</span>"); n != 30 {
		t.Errorf("shown %d, want 30", n)
	}
	if n := strings.Count(get(h, "/search?feed=60&q=quokka").Body.String(), "Quokka note</span>"); n != 31 {
		t.Errorf("after more: %d, want 31", n)
	}
}

// A mutation made from the search page comes back to it.
func TestSearchPageMutationsReturn(t *testing.T) {
	h, _ := newApp(t)
	post(h, "/journal", url.Values{"activity": {"2"}, "kind": {"task"}, "text": {"Walrus check"}})
	ids := searchIDs(t, h, "walrus")
	next := searchURL("walrus")
	rec := hxPost(h, "/tasks/"+ids[0]+"/toggle", url.Values{"next": {next}})
	wantStatus(t, rec, 200)
	if rec.Header().Get("HX-Push-Url") != next {
		t.Errorf("HX-Push-Url = %q", rec.Header().Get("HX-Push-Url"))
	}
	wantBody(t, rec, `class="search-page"`, "Walrus check", "opened ")
	rec = post(h, "/tasks/"+ids[0]+"/toggle", url.Values{"next": {next}})
	wantStatus(t, rec, 303)
	if rec.Header().Get("Location") != next {
		t.Errorf("Location = %q", rec.Header().Get("Location"))
	}
}

func atoi(s string) int { n, _ := strconv.Atoi(s); return n }

// searchIDs returns the ids /search/ids answers for q.
func searchIDs(t *testing.T, h http.Handler, q string) []string {
	t.Helper()
	rec := get(h, "/search/ids?"+url.Values{"q": {q}}.Encode())
	wantStatus(t, rec, 200)
	var got struct{ IDs []int64 }
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, id := range got.IDs {
		out = append(out, strconv.FormatInt(id, 10))
	}
	return out
}

func TestSearchIDs(t *testing.T) {
	h, _ := newApp(t)
	post(h, "/journal", url.Values{"activity": {"2"}, "kind": {"update"}, "text": {"Narwhal one"}})
	post(h, "/journal", url.Values{"kind": {"update"}, "text": {"Narwhal two"}})
	if ids := searchIDs(t, h, "narwhal"); len(ids) != 2 || atoi(ids[0]) < atoi(ids[1]) {
		t.Errorf("ids = %v, want two, newest first", ids)
	}
	rec := get(h, "/search/ids?q=xyzzy")
	if rec.Body.String() != `{"ids":[]}`+"\n" || rec.Header().Get("Content-Type") != "application/json" {
		t.Errorf("no match: %q %q", rec.Body, rec.Header().Get("Content-Type"))
	}
	rec = get(h, "/search/ids?q=k:zzz")
	wantStatus(t, rec, 400)
	if rec.Body.String() != `{"error":"E475: Invalid argument: k:zzz"}`+"\n" {
		t.Errorf("error body %q", rec.Body)
	}
	if rec := get(h, "/search/ids?q="); rec.Code != 400 || !strings.Contains(rec.Body.String(), "E35") {
		t.Errorf("empty query: %d %s", rec.Code, rec.Body)
	}
}

func TestSearchWords(t *testing.T) {
	h, st := newApp(t)
	if err := st.SetProjectArchived(ctx, 4, true); err != nil {
		t.Fatal(err)
	}
	archived, err := st.GetProject(ctx, 4)
	if err != nil {
		t.Fatal(err)
	}
	rec := get(h, "/search/words")
	wantStatus(t, rec, 200)
	var words struct {
		Keys []struct {
			Key    string
			Values []string
		}
		Projects, Activities []string
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &words); err != nil {
		t.Fatal(err)
	}
	if len(words.Keys) != 5 || words.Keys[0].Key != "p" || words.Keys[2].Key != "proj" || len(words.Keys[2].Values) != 0 {
		t.Errorf("keys = %+v", words.Keys)
	}
	if len(words.Projects) == 0 || len(words.Activities) == 0 {
		t.Errorf("no names: %+v", words)
	}
	for _, p := range words.Projects {
		if p == archived.Name {
			t.Errorf("archived project %q offered", p)
		}
	}
}
