package app

import (
	"bytes"
	"context"
	"fly/internal/config"
	"fly/internal/core"
	"fmt"
	P "github.com/metacubex/mihomo/constant/provider"
	RP "github.com/metacubex/mihomo/rules/provider"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
)

func TestProviderSearchReadsYAMLTextAndMRSWithoutLiveStrategy(t *testing.T) {
	for _, format := range []string{"yaml", "text", "mrs"} {
		t.Run(format, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "rules")
			if err := os.MkdirAll(path, 0700); err != nil {
				t.Fatal(err)
			}
			content := []byte("payload:\n- example.org\n- other.test\n- api.example.org\n")
			if format == "text" {
				content = []byte("example.org\nother.test\napi.example.org\n")
			}
			if format == "mrs" {
				var out bytes.Buffer
				if err := RP.ConvertToMrs(content, P.Domain, P.YamlRule, &out); err != nil {
					t.Fatal(err)
				}
				content = out.Bytes()
			}
			if err := os.WriteFile(filepath.Join(path, "source"), content, 0600); err != nil {
				t.Fatal(err)
			}
			a := &App{Dir: dir, Cfg: config.Default(""), cfgBytes: []byte(fmt.Sprintf("rule-providers:\n  example:\n    type: http\n    path: ./rules/source\n    behavior: domain\n    format: %s\n", format))}
			result, err := a.ProviderRules("example", "EXAMPLE", 0, 1)
			if err != nil {
				t.Fatal(err)
			}
			page := result.(rulePage)
			if len(page.Items) != 1 || page.Total != 2 || !page.More {
				t.Fatalf("search/pagination failed: %+v", page)
			}
			if _, err := a.ProviderRules("../../private", "", 0, 1); err == nil {
				t.Fatal("arbitrary path allowed")
			}
		})
	}
}

func TestEditedRulesActuallyRouteDirectProxyAndReject(t *testing.T) {
	var upstream atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("target")) }))
	defer target.Close()
	targetURL, _ := url.Parse(target.URL)
	relay := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "CONNECT" || r.Host != targetURL.Host {
			http.Error(w, "only controlled target", 502)
			return
		}
		upstream.Add(1)
		remote, err := net.Dial("tcp", targetURL.Host)
		if err != nil {
			http.Error(w, "dial", 502)
			return
		}
		conn, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			remote.Close()
			return
		}
		_, _ = conn.Write([]byte("HTTP/1.1 200 Connection established\r\n\r\n"))
		go func() {
			defer conn.Close()
			defer remote.Close()
			go io.Copy(remote, conn)
			_, _ = io.Copy(conn, remote)
		}()
	}))
	defer relay.Close()
	relayURL, _ := url.Parse(relay.URL)
	relayHost, relayPort, _ := net.SplitHostPort(relayURL.Host)
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	cfg := config.Default("")
	cfg.MixedPort = port
	a := &App{Dir: t.TempDir(), Cfg: cfg}
	defer core.StopMixed()
	body := []byte(fmt.Sprintf("proxies:\n- {name: controlled, type: http, server: %s, port: %s}\nrules:\n- MATCH,DIRECT\n", relayHost, relayPort))
	if err := a.ChangeSubscription(context.Background(), "", "", body); err != nil {
		t.Fatal(err)
	}
	mixed, _ := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", port))
	request := func() error {
		tr := &http.Transport{Proxy: http.ProxyURL(mixed), DisableKeepAlives: true}
		defer tr.CloseIdleConnections()
		resp, err := (&http.Client{Transport: tr}).Get(target.URL)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		data, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		if resp.StatusCode != 200 || string(data) != "target" {
			return fmt.Errorf("HTTP %d", resp.StatusCode)
		}
		return nil
	}
	for _, action := range []string{"reject", "direct", "proxy"} {
		if err := a.SaveUserRules([]config.UserRule{{Match: "ip_cidr", Value: "127.0.0.1/32", Action: action}}); err != nil {
			t.Fatal(err)
		}
		before := upstream.Load()
		err := request()
		if action == "reject" {
			if err == nil {
				t.Fatal("reject rule permitted traffic")
			}
			continue
		}
		if err != nil {
			t.Fatalf("%s failed: %v", action, err)
		}
		if action == "proxy" && upstream.Load() == before {
			t.Fatal("proxy rule silently bypassed selected node")
		}
		if action == "direct" && upstream.Load() != before {
			t.Fatal("direct rule used upstream")
		}
	}
}

func TestRuleResourcesUpdateAndKeepBoundCacheWhenOffline(t *testing.T) {
	var offline atomic.Bool
	var changed atomic.Bool
	rules := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if offline.Load() {
			http.Error(w, "offline", 503)
			return
		}
		if changed.Load() {
			w.Write([]byte("payload:\n- DOMAIN,new.example.org\n"))
		} else {
			w.Write([]byte("payload:\n- DOMAIN,old.example.org\n"))
		}
	}))
	defer rules.Close()
	dir := t.TempDir()
	if err := core.Init(dir); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default("")
	cfg.MixedPort = 0
	a := &App{Dir: dir, Cfg: cfg}
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(fmt.Sprintf("proxies:\n- {name: node, type: http, server: 127.0.0.1, port: 1}\nrule-providers:\n  example: {type: http, behavior: classical, url: '%s', interval: 86400}\nrules:\n- RULE-SET,example,DIRECT\n- MATCH,DIRECT\n", rules.URL)))
	}))
	defer source.Close()
	cfg.SubscriptionURL = source.URL
	if err := a.Refresh(); err != nil {
		t.Fatal(err)
	}
	check := func(want string) {
		t.Helper()
		page, err := a.ProviderRules("example", "", 0, 10)
		if err != nil {
			t.Fatal(err)
		}
		if got := page.(rulePage).Items; len(got) != 1 || got[0] != want {
			t.Fatalf("resource mismatch: %v", got)
		}
	}
	check("DOMAIN,old.example.org")
	changed.Store(true)
	if err := a.Refresh(); err != nil {
		t.Fatal(err)
	}
	check("DOMAIN,new.example.org")
	offline.Store(true)
	if err := a.Refresh(); err != nil {
		t.Fatal(err)
	}
	check("DOMAIN,new.example.org")
	if a.LastErr == "" {
		t.Fatal("cached rule fallback presented as fresh successful download")
	}
}
