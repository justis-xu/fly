package core

import (
	"github.com/metacubex/mihomo/component/mmdb"
	"github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/tunnel"
	"gopkg.in/yaml.v3"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

// Rule downloads must bootstrap independently of the selected (possibly dead) node.
func TestRuleDownloadWithUnavailableNode(t *testing.T) {
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("payload:\n  - DOMAIN,example.org\n"))
	}))
	defer source.Close()
	if err := Init(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	b, err := BuildConfig([]map[string]any{{"name": "offline", "type": "http", "server": "127.0.0.1", "port": 1}}, Routing{Rules: []string{"RULE-SET,test,DIRECT", "MATCH," + GroupProxy}, Providers: map[string]map[string]any{"test": {"type": "http", "behavior": "classical", "url": source.URL, "interval": 86400}}}, Options{MixedPort: 0})
	if err != nil {
		t.Fatal(err)
	}
	if err = Start(b); err != nil {
		t.Fatal(err)
	}
	p := tunnel.RuleProviders()["test"]
	if p == nil || p.Count() != 1 {
		t.Fatal("rule download depended on unavailable selected node")
	}
	matched, target := tunnel.Rules()[0].Match(&constant.Metadata{Host: "example.org"}, constant.RuleMatchHelper{CheckPassRule: func(string) bool { return false }})
	if !matched || target != "DIRECT" {
		t.Fatalf("downloaded subscription rule did not match: %v %s", matched, target)
	}
}

func TestInitIncludesASN(t *testing.T) {
	dir := t.TempDir()
	if err := Init(dir); err != nil {
		t.Fatal(err)
	}
	if !mmdb.Verify(filepath.Join(dir, "ASN.mmdb")) {
		t.Fatal("ASN database missing or invalid; subscription IP-ASN rules would trigger a runtime download")
	}
}

func TestOccupiedProxyPortIsReported(t *testing.T) {
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer occupied.Close()
	port := occupied.Addr().(*net.TCPAddr).Port
	b, err := BuildConfig([]map[string]any{{"name": "test", "type": "http", "server": "127.0.0.1", "port": 1}}, Routing{Rules: []string{"MATCH," + GroupProxy}}, Options{MixedPort: port})
	if err != nil {
		t.Fatal(err)
	}
	if err = Start(b); err == nil {
		t.Fatal("occupied proxy port incorrectly reported success")
	}
}

func TestRuleCacheChangesWithSourceURL(t *testing.T) {
	build := func(url string) string {
		b, err := BuildConfig([]map[string]any{{"name": "test", "type": "http", "server": "127.0.0.1", "port": 1}}, Routing{Rules: []string{"MATCH," + GroupProxy}, Providers: map[string]map[string]any{"same-name": {"type": "http", "behavior": "classical", "url": url}}}, Options{})
		if err != nil {
			t.Fatal(err)
		}
		var c struct {
			Providers map[string]map[string]any `yaml:"rule-providers"`
		}
		if err := yaml.Unmarshal(b, &c); err != nil {
			t.Fatal(err)
		}
		return c.Providers["same-name"]["path"].(string)
	}
	if build("https://example.org/a") == build("https://example.org/b") {
		t.Fatal("different rule sources share stale cache")
	}
}

func TestUDPPortCollisionIsReported(t *testing.T) {
	occupied, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer occupied.Close()
	port := occupied.LocalAddr().(*net.UDPAddr).Port
	b, err := BuildConfig([]map[string]any{{"name": "test", "type": "http", "server": "127.0.0.1", "port": 1}}, Routing{Rules: []string{"MATCH," + GroupProxy}}, Options{MixedPort: port})
	if err != nil {
		t.Fatal(err)
	}
	if err = Start(b); err == nil || Ready(port) {
		t.Fatal("UDP conflict closed TCP listener but proxy reported ready")
	}
}
