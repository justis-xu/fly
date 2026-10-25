package sub

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestFetchIgnoresEnvironmentProxy(t *testing.T) {
	// DefaultTransport is intentionally poisoned as well: no global transport may leak into fetching.
	old := http.DefaultTransport
	http.DefaultTransport = &http.Transport{Proxy: func(*http.Request) (*url.URL, error) { return url.Parse("http://127.0.0.1:1") }}
	defer func() { http.DefaultTransport = old }()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("nodes")) }))
	defer source.Close()
	b, _, _, err := FetchWithOptions(context.Background(), source.URL, FetchOptions{})
	if err != nil || string(b) != "nodes" {
		t.Fatalf("direct download borrowed environment: %q %v", b, err)
	}
}
func TestFetchFallbackProxy(t *testing.T) {
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Host != "127.0.0.1:1" {
			t.Error(r.URL.Host)
		}
		w.Write([]byte("fallback"))
	}))
	defer proxy.Close()
	b, _, route, err := FetchWithOptions(context.Background(), "http://127.0.0.1:1/private-token", FetchOptions{ProxyURL: proxy.URL})
	if err != nil || string(b) != "fallback" || route != "备用代理" {
		t.Fatalf("fallback: %q %q %v", b, route, err)
	}
}
func TestOversizedSubscriptionRejectedAndErrorsRedacted(t *testing.T) {
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(strings.Repeat("x", MaxBody+1))) }))
	defer source.Close()
	_, _, _, err := FetchWithOptions(context.Background(), source.URL+"/secret", FetchOptions{})
	if err == nil || strings.Contains(err.Error(), "secret") {
		t.Fatalf("oversize or secret leak: %v", err)
	}
}
func TestCacheDoesNotCrossSources(t *testing.T) {
	dir := t.TempDir()
	if err := SaveSourceCache(dir, "https://a.test/token", []byte("cached"), &Info{}); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := LoadSourceCache(dir, "https://b.test/token"); ok {
		t.Fatal("new subscription reused different source")
	}
	if b, _, ok := LoadSourceCache(dir, "https://a.test/token"); !ok || string(b) != "cached" {
		t.Fatal("own cache lost")
	}
}
func TestNodeNamesAndInvalidPartialSubscriptions(t *testing.T) {
	list := []map[string]any{
		{"name": "DIRECT", "type": "http", "server": "example.org", "port": 80},
		{"name": "A", "type": "http", "server": "example.org", "port": 80},
		{"name": "A #2", "type": "http", "server": "example.org", "port": 80},
		{"name": "A", "type": "http", "server": "example.org", "port": 80},
		{"name": "broken", "type": "ss", "server": "example.org", "port": 443, "cipher": "bad", "password": "x"},
	}
	got := FilterProxies(list)
	if len(got) != 4 {
		t.Fatalf("partial validation: %v", got)
	}
	used := map[string]bool{"DIRECT": true}
	for _, p := range got {
		n := p["name"].(string)
		if used[n] {
			t.Fatalf("ambiguous/reserved name %s", n)
		}
		used[n] = true
	}
	nodes, skipped := ParseLinks("trojan://secret@example.org:443\nss://YWVzLTEyOC1nY206cHc@example.org:443?plugin=unsupported#bad")
	if len(nodes) != 1 || skipped != 1 || nodes[0]["name"] == "" {
		t.Fatalf("unnamed or unsupported plugin: %v %d", nodes, skipped)
	}
}

func TestAuthenticationErrorDoesNotRetryThroughAnotherNetwork(t *testing.T) {
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "denied", 401) }))
	defer source.Close()
	called := false
	fallback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true; w.Write([]byte("unexpected")) }))
	defer fallback.Close()
	_, _, _, err := FetchWithOptions(context.Background(), source.URL+"/token", FetchOptions{ProxyURL: fallback.URL})
	if err == nil || called || strings.Contains(err.Error(), "/token") {
		t.Fatalf("authentication retry leaked state: %v %v", called, err)
	}
}
func TestProviderHTTPIsExpandedAndFilterApplied(t *testing.T) {
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("proxies:\n- {name: HK, type: http, server: example.org, port: 80}\n- {name: JP, type: http, server: example.org, port: 80}\n"))
	}))
	defer provider.Close()
	body := []byte("proxy-providers:\n  sample:\n    type: http\n    url: " + provider.URL + "\n    filter: HK\n    override: {additional-prefix: 'fly '}\n")
	expanded, err := ExpandProviders(context.Background(), body, FetchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	_, list, _ := BodyToLinks(expanded)
	if len(list) != 1 || list[0]["name"] != "fly HK" {
		t.Fatalf("provider expansion lost filters: %s", expanded)
	}
	if strings.Contains(string(expanded), provider.URL) {
		t.Fatal("expanded cache still depends on provider URL")
	}
}
