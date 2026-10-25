package checks

import (
	"context"
	"github.com/metacubex/mihomo/adapter"
	"github.com/metacubex/mihomo/adapter/outbound"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestWebsiteStatusesAreIndependentFromLatencyHistory(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/204":
			w.WriteHeader(204)
		case "/403":
			w.WriteHeader(403)
		case "/429":
			w.WriteHeader(429)
		case "/500":
			w.WriteHeader(500)
		case "/redirect":
			http.Redirect(w, r, "/204", 302)
		case "/wait":
			<-r.Context().Done()
		default:
			w.WriteHeader(200)
		}
	}))
	defer target.Close()
	p := adapter.NewProxy(outbound.NewDirect())
	for _, tc := range []struct {
		path, status string
		code         int
	}{{"/204", "ok", 204}, {"/403", "restricted", 403}, {"/429", "restricted", 429}, {"/500", "site_error", 500}, {"/redirect", "ok", 204}} {
		result := WebsiteProbe(context.Background(), Task{URL: target.URL + tc.path}, NodeClient(p))
		if result.Status != tc.status || result.HTTPStatus != tc.code {
			t.Fatalf("%s: %+v", tc.path, result)
		}
	}
	if len(p.DelayHistory()) != 0 {
		t.Fatal("website requests polluted latency/automatic selection history")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	result := WebsiteProbe(ctx, Task{URL: target.URL + "/wait"}, NodeClient(p))
	if result.Status != "timeout" {
		t.Fatalf("timeout classified as %+v", result)
	}
	result = LatencyProbe(context.Background(), p, target.URL+"/403")
	if result.Status == "ok" {
		t.Fatal("latency accepted HTTP 403 as healthy")
	}
	result = LatencyProbe(context.Background(), p, target.URL+"/204")
	if result.Status != "ok" || result.Delay < 1 {
		t.Fatalf("successful latency missing: %+v", result)
	}
}
