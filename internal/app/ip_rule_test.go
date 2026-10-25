package app

import (
	"context"
	"fly/internal/config"
	"fly/internal/core"
	"fmt"
	"github.com/metacubex/mihomo/component/resolver"
	"github.com/miekg/dns"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"testing"
)

// Only external DNS is substituted. Actual rules, mixed listener and HTTP forwarding run normally.
type fixtureDNS struct{}

func (fixtureDNS) LookupIP(context.Context, string) ([]netip.Addr, error) {
	return []netip.Addr{netip.MustParseAddr("127.0.0.1")}, nil
}
func (fixtureDNS) LookupIPv4(ctx context.Context, host string) ([]netip.Addr, error) {
	return (fixtureDNS{}).LookupIP(ctx, host)
}
func (fixtureDNS) LookupIPv6(context.Context, string) ([]netip.Addr, error) {
	return nil, resolver.ErrIPv6Disabled
}
func (fixtureDNS) ResolveECH(context.Context, string) ([]byte, error) {
	return nil, fmt.Errorf("not used")
}
func (fixtureDNS) ExchangeContext(context.Context, *dns.Msg) (*dns.Msg, error) {
	return nil, fmt.Errorf("not used")
}
func (fixtureDNS) Invalid() bool    { return true }
func (fixtureDNS) ClearCache()      {}
func (fixtureDNS) ResetConnection() {}

func TestIPOverrideAppliesToAResolvedDestinationDomain(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	defer target.Close()
	sock, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := sock.Addr().(*net.TCPAddr).Port
	sock.Close()
	cfg := config.Default("")
	cfg.MixedPort = port
	a := &App{Dir: t.TempDir(), Cfg: cfg}
	if err = a.ChangeSubscription(context.Background(), "", "", []byte("proxies:\n- {name: fixture, type: http, server: 127.0.0.1, port: 1}\nrules:\n- MATCH,DIRECT\n")); err != nil {
		t.Fatal(err)
	}
	original := resolver.SystemResolver
	resolver.SystemResolver = fixtureDNS{}
	defer func() { core.StopMixed(); resolver.SystemResolver = original }()
	proxy, _ := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", port))
	client := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxy), DisableKeepAlives: true}}
	request := func() int {
		u, _ := url.Parse(target.URL)
		_, p, _ := net.SplitHostPort(u.Host)
		response, err := client.Get("http://ip-rule.test:" + p)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		return response.StatusCode
	}
	if request() != 204 {
		t.Fatal("controlled DNS destination was not reachable before override")
	}
	if err = a.SaveUserRules([]config.UserRule{{Match: "ip_cidr", Value: "127.0.0.0/8", Action: "reject"}}); err != nil {
		t.Fatal(err)
	}
	if request() == 204 {
		t.Fatal("IP override ignored a domain resolving into its network")
	}
}
