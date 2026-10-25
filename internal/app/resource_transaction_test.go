package app

import (
	"context"
	"fly/internal/config"
	"fly/internal/core"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
)

func TestRegressionFailedReplacementKeepsRuleResources(t *testing.T) {
	dir := t.TempDir()
	if err := core.Init(dir); err != nil {
		t.Fatal(err)
	}
	defer core.StopMixed()
	var changed atomic.Bool
	rs := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		domain := "old.example.org"
		if changed.Load() {
			domain = "new.example.org"
		}
		fmt.Fprintf(w, "payload:\n- DOMAIN,%s\n", domain)
	}))
	defer rs.Close()
	l, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	cfg := config.Default("")
	cfg.MixedPort = l.Addr().(*net.TCPAddr).Port
	l.Close()
	a := &App{Dir: dir, Cfg: cfg}
	body := fmt.Sprintf("proxies:\n- {name: old, type: http, server: 127.0.0.1, port: 1}\nrule-providers:\n  same: {type: http, behavior: classical, url: '%s'}\nrules:\n- RULE-SET,same,REJECT\n- MATCH,DIRECT\n", rs.URL)
	if err := a.ChangeSubscription(context.Background(), "", "", []byte(body)); err != nil {
		t.Fatal(err)
	}
	original := a.Info.RuleFiles
	changed.Store(true)
	bad := strings.Replace(body, "MATCH,DIRECT", "INVALID-TYPE,foo,DIRECT", 1)
	if err := a.ChangeSubscription(context.Background(), "", "", []byte(bad)); err == nil {
		t.Fatal("bad config accepted")
	}
	check := func(current *App) {
		t.Helper()
		page, err := current.ProviderRules("same", "", 0, 10)
		if err != nil {
			t.Fatal(err)
		}
		if got := page.(rulePage).Items; len(got) != 1 || got[0] != "DOMAIN,old.example.org" {
			t.Fatalf("failed replacement changed accepted rule contents: %v", got)
		}
		if !reflect.DeepEqual(original, current.Info.RuleFiles) {
			t.Fatal("accepted snapshot paths changed")
		}
	}
	check(a)
	// User-rule edits rebuild YAML but must preserve exactly the accepted resource snapshot.
	if err := a.SaveUserRules([]config.UserRule{{Match: "domain", Value: "test.example.org", Action: "direct"}}); err != nil {
		t.Fatal(err)
	}
	check(a)
	// Rule preparation may succeed before settings persistence fails. Old cache and
	// immutable resource paths must still reconstruct the same routing on restart.
	if os.Geteuid() != 0 {
		if err := os.Chmod(dir, 0555); err != nil {
			t.Fatal(err)
		}
		err := a.ChangeSubscription(context.Background(), "", "", []byte(body))
		_ = os.Chmod(dir, 0700)
		if err == nil {
			t.Fatal("unwritable settings accepted")
		}
		check(a)
	}
	rs.Close()
	core.StopMixed()
	saved, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	restarted := &App{Dir: dir, Cfg: saved}
	if err = restarted.LoadSubscription(); err != nil {
		t.Fatal(err)
	}
	if err = restarted.StartCore(); err != nil {
		t.Fatal(err)
	}
	check(restarted)

}

func TestRegressionDifferentAuthCannotBorrowRuleCache(t *testing.T) {
	dir := t.TempDir()
	if err := core.Init(dir); err != nil {
		t.Fatal(err)
	}
	defer core.StopMixed()
	rs := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer account-a" {
			http.Error(w, "unauthorized", 401)
			return
		}
		fmt.Fprint(w, "payload:\n- DOMAIN,account-a.example.org\n")
	}))
	defer rs.Close()
	l, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	cfg := config.Default("")
	cfg.MixedPort = l.Addr().(*net.TCPAddr).Port
	l.Close()
	a := &App{Dir: dir, Cfg: cfg}
	body := fmt.Sprintf("proxies:\n- {name: account-a, type: http, server: 127.0.0.1, port: 1}\nrule-providers:\n  same: {type: http, behavior: classical, url: '%s', header: {Authorization: ['Bearer account-a']}}\nrules:\n- RULE-SET,same,REJECT\n- MATCH,DIRECT\n", rs.URL)
	if err := a.ChangeSubscription(context.Background(), "", "", []byte(body)); err != nil {
		t.Fatal(err)
	}
	next := strings.ReplaceAll(body, "account-a", "account-b")
	if err := a.ChangeSubscription(context.Background(), "", "", []byte(next)); err == nil {
		page, e := a.ProviderRules("same", "", 0, 10)
		t.Fatalf("account B activation succeeded using account A rule cache: %v %v", page, e)
	}
}
