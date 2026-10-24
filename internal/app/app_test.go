package app

import (
	"fly/internal/config"
	"fly/internal/core"
	"gopkg.in/yaml.v3"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestSubscriptionKeepsRuleSets(t *testing.T) {
	body := []byte(`proxies:
  - {name: test, type: ss, server: example.com, port: 443, cipher: aes-128-gcm, password: test}
rule-providers:
  local: {type: inline, behavior: classical, payload: ['DOMAIN,example.org']}
rules:
  - RULE-SET,local,DIRECT
  - MATCH,ProviderGroup
`)
	a := &App{Cfg: config.Default("")}
	proxies, rules, skipped, err := parseBody(body)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.applyResult(proxies, rules, skipped, nil); err != nil {
		t.Fatal(err)
	}
	var built struct {
		Rules     []string                  `yaml:"rules"`
		Providers map[string]map[string]any `yaml:"rule-providers"`
	}
	if err := yaml.Unmarshal(a.cfgBytes, &built); err != nil {
		t.Fatal(err)
	}
	if len(built.Rules) != 2 || built.Rules[0] != "RULE-SET,local,DIRECT" || built.Rules[1] != "MATCH,"+core.GroupProxy || built.Providers["local"] == nil {
		t.Fatalf("subscription routing lost: %+v", built)
	}
}

func TestInvalidRefreshKeepsRuntimeAndCache(t *testing.T) {
	body := `proxies:
  - {name: broken, type: ss, server: example.com, port: 443, cipher: invalid-cipher, password: test}
`
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(body)) }))
	defer source.Close()
	dir := t.TempDir()
	cfg := config.Default("")
	cfg.SubscriptionURL = source.URL
	cfg.MixedPort = 0
	a := &App{Dir: dir, Cfg: cfg, Nodes: 7, cfgBytes: []byte("previous")}
	if err := os.WriteFile(filepath.Join(dir, "sub-cache.txt"), []byte("previous-cache"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := a.Refresh(); err == nil {
		t.Fatal("invalid node accepted")
	}
	if a.Nodes != 7 {
		t.Fatalf("failed refresh changed active node count to %d", a.Nodes)
	}
	cached, _ := os.ReadFile(filepath.Join(dir, "sub-cache.txt"))
	if string(cached) != "previous-cache" {
		t.Fatal("failed refresh overwrote good cache")
	}
}

func TestEmptySubscriptionDoesNotUseCache(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default("")
	cfg.SubscriptionURL = ""
	cfg.MixedPort = 0
	a := &App{Dir: dir, Cfg: cfg}
	if err := os.WriteFile(filepath.Join(dir, "sub-cache.txt"), []byte("proxies:\n  - {name: cached, type: http, server: example.org, port: 80}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := a.LoadSubscription(); err == nil {
		t.Fatal("empty subscription must be rejected")
	}
	if a.Nodes != 0 {
		t.Fatal("empty subscription activated nodes")
	}
}

func TestSubRulesDoNotGainCatchAll(t *testing.T) {
	body := []byte(`proxies:
  - {name: test, type: http, server: example.com, port: 80}
sub-rules:
  service:
    - DOMAIN,example.org,OldGroup
rules:
  - SUB-RULE,(NETWORK,TCP),service
  - DOMAIN,local.test,DIRECT
  - MATCH,OldGroup
`)
	_, routing, _, err := parseBody(body)
	if err != nil {
		t.Fatal(err)
	}
	if len(routing.SubRules["service"]) != 1 {
		t.Fatalf("sub-rule gained a catch-all: %v", routing.SubRules)
	}
}

func TestColdStartFallsBackAfterInvalidCoreConfig(t *testing.T) {
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("proxies:\n  - {name: broken, type: ss, server: example.com, port: 443, cipher: invalid-cipher, password: test}\nrules:\n  - MATCH,DIRECT\n"))
	}))
	defer source.Close()
	dir := t.TempDir()
	cfg := config.Default("")
	cfg.SubscriptionURL = source.URL
	cfg.MixedPort = 0
	cached := []byte("proxies:\n  - {name: cached, type: http, server: example.com, port: 80}\nrules:\n  - MATCH,DIRECT\n")
	if err := os.WriteFile(filepath.Join(dir, "sub-cache.txt"), cached, 0600); err != nil {
		t.Fatal(err)
	}
	a := &App{Dir: dir, Cfg: cfg}
	if err := a.Initialize(); err != nil {
		t.Fatal(err)
	}
	if a.Nodes != 1 || a.LastErr == "" || a.Starting() {
		t.Fatalf("cached startup lost: nodes=%d warning=%q starting=%v", a.Nodes, a.LastErr, a.Starting())
	}
}
