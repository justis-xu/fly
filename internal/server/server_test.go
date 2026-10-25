package server

import (
	"encoding/json"
	"fly/internal/app"
	"fly/internal/config"
	"net"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

func TestSubscriptionSaveFailureKeepsOldURL(t *testing.T) {
	a := &app.App{Dir: t.TempDir() + "/missing", Cfg: config.Default("")}
	old := a.SubscriptionURL()
	s := New(a, "test")
	r := httptest.NewRequest("POST", "/api/subscription", strings.NewReader(`{"url":"https://example.org/sub"}`))
	r.Host = "127.0.0.1:7889"
	w := httptest.NewRecorder()
	s.http.Handler.ServeHTTP(w, r)
	if w.Code != 500 || a.SubscriptionURL() != old {
		t.Fatalf("failed save must return 500 and keep old URL, got %d", w.Code)
	}
}
func TestSubscriptionDoesNotChangeWhileUpdating(t *testing.T) {
	a := &app.App{Dir: t.TempDir(), Cfg: config.Default("")}
	s := New(a, "test")
	s.updating.Store(true)
	r := httptest.NewRequest("POST", "/api/subscription", strings.NewReader(`{"url":"https://example.org/sub"}`))
	r.Host = "127.0.0.1:7889"
	w := httptest.NewRecorder()
	s.http.Handler.ServeHTTP(w, r)
	if w.Code != 409 {
		t.Fatalf("got %d, want 409", w.Code)
	}
}

func TestRejectForeignHostAndOrigin(t *testing.T) {
	s := New(&app.App{Dir: t.TempDir(), Cfg: config.Default("")}, "test")
	for _, host := range []string{"rebind.example:7889", "127.0.0.1:9000"} {
		r := httptest.NewRequest("GET", "/api/subscription", nil)
		r.Host = host
		w := httptest.NewRecorder()
		s.http.Handler.ServeHTTP(w, r)
		if w.Code != 403 {
			t.Fatalf("host %s was accepted (%d)", host, w.Code)
		}
	}
}

func TestFirstRunManagementRemainsAvailable(t *testing.T) {
	cfg := config.Default("")
	a := &app.App{Dir: t.TempDir(), Cfg: cfg}
	if err := a.Initialize(); err != nil {
		t.Fatal(err)
	}
	s := New(a, "test")
	r := httptest.NewRequest("GET", "http://127.0.0.1:7889/api/info", nil)
	w := httptest.NewRecorder()
	s.http.Handler.ServeHTTP(w, r)
	var info struct {
		Ready             bool `json:"ready"`
		NeedsSubscription bool `json:"needs_subscription"`
		Starting          bool `json:"starting"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &info); err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 || info.Ready || !info.NeedsSubscription || info.Starting {
		t.Fatalf("invalid fresh-install state: status=%d %+v", w.Code, info)
	}
	r = httptest.NewRequest("GET", "http://127.0.0.1:7889/", nil)
	w = httptest.NewRecorder()
	s.http.Handler.ServeHTTP(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "Proxy Switcher") {
		t.Fatal("first-run GUI unavailable or missing Proxy Switcher guidance")
	}
}

func TestManagementPortCollisionIsReported(t *testing.T) {
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer occupied.Close()
	cfg := config.Default("")
	cfg.AdminAddr = occupied.Addr().String()
	s := New(&app.App{Dir: t.TempDir(), Cfg: cfg}, "test")
	if err := s.Start(); err == nil {
		defer s.http.Close()
		t.Fatal("occupied management port accepted")
	}
}

func TestRejectForeignOrigin(t *testing.T) {
	s := New(&app.App{Dir: t.TempDir(), Cfg: config.Default("")}, "test")
	r := httptest.NewRequest("GET", "http://127.0.0.1:7889/api/subscription", nil)
	r.Header.Set("Origin", "https://example.org")
	w := httptest.NewRecorder()
	s.http.Handler.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("foreign-origin request accepted")
	}
}

func TestExportedFrontendAssetsAreServed(t *testing.T) {
	s := New(&app.App{Dir: t.TempDir(), Cfg: config.Default("")}, "test")
	root := httptest.NewRecorder()
	s.http.Handler.ServeHTTP(root, httptest.NewRequest("GET", "http://127.0.0.1:7889/", nil))
	assets := regexp.MustCompile(`(?:src|href)="(/_next/[^"?]+)`).FindAllStringSubmatch(root.Body.String(), -1)
	if len(assets) == 0 {
		t.Fatal("management page contains no Next.js assets")
	}
	for _, asset := range assets {
		w := httptest.NewRecorder()
		s.http.Handler.ServeHTTP(w, httptest.NewRequest("GET", "http://127.0.0.1:7889"+asset[1], nil))
		if w.Code != 200 || w.Body.Len() == 0 {
			t.Fatalf("missing exported asset: %s (%d)", asset[1], w.Code)
		}
		contentType := w.Header().Get("Content-Type")
		if strings.HasSuffix(asset[1], ".js") && !strings.Contains(contentType, "javascript") {
			t.Fatalf("script served as %s", contentType)
		}
		if strings.HasSuffix(asset[1], ".css") && !strings.Contains(contentType, "text/css") {
			t.Fatalf("stylesheet served as %s", contentType)
		}
	}
	missing := httptest.NewRecorder()
	s.http.Handler.ServeHTTP(missing, httptest.NewRequest("GET", "http://127.0.0.1:7889/_next/static/missing.js", nil))
	if missing.Code != 404 {
		t.Fatal("missing scripts must not fall back to HTML")
	}
}
