package app

import (
	"context"
	"fly/internal/config"
	"fly/internal/core"
	"fly/internal/sub"
	"fmt"
	"gopkg.in/yaml.v3"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestFailedReplacementKeepsWorkingSourceAndCache(t *testing.T) {
	old := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("proxies:\n- {name: old, type: http, server: 127.0.0.1, port: 1}\nrules:\n- MATCH,DIRECT\n"))
	}))
	defer old.Close()
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("<!doctype html><title>Login</title>")) }))
	defer bad.Close()
	dir := t.TempDir()
	cfg := config.Default("")
	cfg.SubscriptionURL = old.URL
	if err := config.Save(dir, cfg); err != nil {
		t.Fatal(err)
	}
	if err := sub.SaveSourceCache(dir, old.URL, []byte("old-cache"), &sub.Info{}); err != nil {
		t.Fatal(err)
	}
	a := &App{Dir: dir, Cfg: cfg, Nodes: 7, cfgBytes: []byte("previous")}
	if err := a.ChangeSubscription(context.Background(), bad.URL, "", nil); err == nil {
		t.Fatal("HTML replacement accepted")
	}
	persisted, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.SubscriptionURL != old.URL || a.SubscriptionURL() != old.URL || a.Nodes != 7 {
		t.Fatal("failed replacement changed active source")
	}
	if b, _, ok := sub.LoadSourceCache(dir, old.URL); !ok || string(b) != "old-cache" {
		t.Fatal("failed replacement replaced cache")
	}
}
func TestImportOfflineRestartIsIsolated(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("FLY_HOME", dir)
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	sock, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	a.Cfg.MixedPort = sock.Addr().(*net.TCPAddr).Port
	sock.Close()

	body := []byte("proxies:\n- {name: local, type: http, server: 127.0.0.1, port: 1}\nrules:\n- MATCH,DIRECT\n")
	if err := a.ChangeSubscription(context.Background(), "", "", body); err != nil {
		t.Fatal(err)
	}
	b, err := New()
	if err != nil {
		t.Fatal(err)
	}
	if b.Dir != dir || !b.HasSubscription() {
		t.Fatal("imported source or isolated directory lost")
	}
	if err := b.Initialize(); err != nil {
		t.Fatal(err)
	}
	if b.Nodes != 1 || b.SubscriptionURL() != "" {
		t.Fatal("offline import was not recovered")
	}
	// Invalid import never replaces the accepted local file.
	if err := b.ChangeSubscription(context.Background(), "", "", []byte("garbage")); err == nil {
		t.Fatal("invalid import accepted")
	}
	if cached, _, ok := sub.LoadSourceCache(dir, "local-import"); !ok || string(cached) != string(body) {
		t.Fatal("accepted local import cache was lost")
	}
	// Close the test listener without relying on a real user's configuration.
	cfg := a.cfgBytes
	var shutdown map[string]any
	_ = yaml.Unmarshal(cfg, &shutdown)
	shutdown["mixed-port"] = 0
	bytes, _ := yaml.Marshal(shutdown)
	_ = core.Start(bytes)
}
func TestProviderOnlySubscriptionAndOfflineCacheBody(t *testing.T) {
	body := []byte("proxy-providers:\n  sample:\n    type: inline\n    payload:\n    - {name: provided, type: http, server: 127.0.0.1, port: 1}\nrules:\n- MATCH,DIRECT\n")
	expanded, err := sub.ExpandProviders(context.Background(), body, sub.FetchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	nodes, _, _, err := parseBody(expanded)
	if err != nil || len(nodes) != 1 {
		t.Fatalf("provider-only source failed: %v %v", nodes, err)
	}
}

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "fly-app-core-test-")
	if err != nil {
		panic(err)
	}
	if err = core.Init(dir); err != nil {
		panic(err)
	}
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

func TestUserRulesSurviveRefreshAndInvalidSave(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default("")
	cfg.MixedPort = 0
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("proxies:\n- {name: node, type: http, server: 127.0.0.1, port: 1}\nrules:\n- MATCH,DIRECT\n"))
	}))
	defer source.Close()
	cfg.SubscriptionURL = source.URL
	a := &App{Dir: dir, Cfg: cfg}
	if err := a.Refresh(); err != nil {
		t.Fatal(err)
	}
	rules := []config.UserRule{{Match: "domain", Value: "example.org", Action: "reject"}}
	if err := a.SaveUserRules(rules); err != nil {
		t.Fatal(err)
	}
	if err := a.Refresh(); err != nil {
		t.Fatal(err)
	}
	var built struct {
		Rules []string `yaml:"rules"`
	}
	_ = yaml.Unmarshal(a.cfgBytes, &built)
	if len(built.Rules) != 2 || built.Rules[0] != "DOMAIN,example.org,REJECT" || built.Rules[1] != "MATCH,DIRECT" {
		t.Fatalf("user precedence lost: %v", built.Rules)
	}
	if err := a.SaveUserRules([]config.UserRule{{Match: "domain", Value: "bad,value", Action: "proxy"}}); err == nil {
		t.Fatal("invalid rule accepted")
	}
	persisted, err := config.LoadRules(dir)
	if err != nil || len(persisted) != 1 || persisted[0].Value != "example.org" {
		t.Fatalf("invalid save changed rules: %v %v", persisted, err)
	}
	// Restart independently reconstructs the accepted overrides from disk.
	b := &App{Dir: dir, Cfg: cfg}
	if err := b.Initialize(); err != nil {
		t.Fatal(err)
	}
	_ = yaml.Unmarshal(b.cfgBytes, &built)
	if built.Rules[0] != "DOMAIN,example.org,REJECT" {
		t.Fatal("restart lost overrides")
	}
}

func TestMalformedSubscriptionRuleIsNotSilentlyDiscarded(t *testing.T) {
	body := []byte("proxies:\n- {name: valid, type: http, server: 127.0.0.1, port: 1}\nrules:\n- DOMAIN,example.org\n")
	if _, _, _, err := parseBody(body); err == nil {
		t.Fatal("malformed rule silently became a catch-all proxy rule")
	}
}

func TestDiskSaveFailureRestoresLiveSourceAndItsCache(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permission failure requires an unprivileged user")
	}
	source := func(name string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintf(w, "proxies:\n- {name: %s, type: http, server: 127.0.0.1, port: 1}\nrules:\n- MATCH,DIRECT\n", name)
		}))
	}
	old, next := source("old-node"), source("new-node")
	defer old.Close()
	defer next.Close()
	dir := t.TempDir()
	cfg := config.Default("")
	cfg.SubscriptionURL = old.URL
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	cfg.MixedPort = listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	defer core.StopMixed()
	if err = config.Save(dir, cfg); err != nil {
		t.Fatal(err)
	}
	a := &App{Dir: dir, Cfg: cfg}
	if err = a.Refresh(); err != nil {
		t.Fatal(err)
	}
	if err = os.Chmod(dir, 0555); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(dir, 0700)
	if err = a.ChangeSubscription(context.Background(), next.URL, "", nil); err == nil {
		t.Fatal("unwritable config accepted")
	}
	persisted, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	pinned, err := core.SelectedProxy()
	if err != nil {
		t.Fatal(err)
	}
	if persisted.SubscriptionURL != old.URL || a.SubscriptionURL() != old.URL || pinned.Name() != "old-node" {
		t.Fatal("failed persistence changed live or saved source")
	}
	if _, _, ok := sub.LoadSourceCache(dir, old.URL); !ok {
		t.Fatal("old source cache destroyed")
	}
	if _, _, ok := sub.LoadSourceCache(dir, next.URL); ok {
		t.Fatal("failed transaction retained a new accepted-source cache")
	}
}
