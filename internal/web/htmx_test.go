package web

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
	"testing"
)

// The vendored htmx must be exactly the registry's 2.0.11 build (SPEC §10 item 58).
func TestVendoredHtmx(t *testing.T) {
	h, _ := newApp(t)
	rec := get(h, "/static/htmx.min.js")
	wantStatus(t, rec, http.StatusOK)
	sum := sha256.Sum256(rec.Body.Bytes())
	if got := hex.EncodeToString(sum[:]); got != "d6fdc75f204e6bdefa99b69bf1e6d4ac69b8a364f77929f45c13476b4000f717" {
		t.Errorf("htmx.min.js sha256 = %s", got)
	}
}

func TestPagesLoadHtmxWithSafeConfig(t *testing.T) {
	h, _ := newApp(t)
	for _, target := range []string{"/roadmap", "/projects"} {
		body := get(h, target).Body.String()
		for _, want := range []string{`<script src="/static/htmx.min.js" defer>`, `"allowEval":false`, `"selfRequestsOnly":true`, `"historyCacheSize":0`} {
			if !strings.Contains(body, want) {
				t.Errorf("%s lacks %s", target, want)
			}
		}
	}
}
