package checks

import (
	"context"
	"fly/internal/core"
	"fmt"
	"github.com/metacubex/mihomo/adapter/outboundgroup"
	"github.com/metacubex/mihomo/tunnel"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"sync/atomic"
	"testing"
)

func TestPinnedNodeDoesNotChangeWhenSelectionChangesAndMixedUsesRules(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	defer target.Close()
	destination, _ := url.Parse(target.URL)
	relay := func(count *atomic.Int32) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "CONNECT" || r.Host != destination.Host {
				http.Error(w, "test target only", 502)
				return
			}
			count.Add(1)
			remote, err := net.Dial("tcp", destination.Host)
			if err != nil {
				http.Error(w, "dial", 502)
				return
			}
			conn, _, err := w.(http.Hijacker).Hijack()
			if err != nil {
				remote.Close()
				return
			}
			conn.Write([]byte("HTTP/1.1 200 Connection established\r\n\r\n"))
			go func() { defer conn.Close(); defer remote.Close(); go io.Copy(remote, conn); io.Copy(conn, remote) }()
		}))
	}
	var first, second atomic.Int32
	one, two := relay(&first), relay(&second)
	defer one.Close()
	defer two.Close()
	nodes := []map[string]any{}
	for i, server := range []*httptest.Server{one, two} {
		u, _ := url.Parse(server.URL)
		host, port, _ := net.SplitHostPort(u.Host)
		n, _ := strconv.Atoi(port)
		nodes = append(nodes, map[string]any{"name": fmt.Sprintf("node-%d", i), "type": "http", "server": host, "port": n})
	}
	sock, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := sock.Addr().(*net.TCPAddr).Port
	sock.Close()
	defer core.StopMixed()
	data, err := core.BuildConfig(nodes, core.Routing{Rules: []string{"MATCH,DIRECT"}}, core.Options{MixedPort: port})
	if err != nil {
		t.Fatal(err)
	}
	if err = core.Start(data); err != nil {
		t.Fatal(err)
	}
	selector := tunnel.Proxies()[core.GroupProxy].Adapter().(*outboundgroup.Selector)
	if err = selector.Set("node-0"); err != nil {
		t.Fatal(err)
	}
	pinned, err := core.SelectedProxy()
	if err != nil {
		t.Fatal(err)
	}
	if err = selector.Set("node-1"); err != nil {
		t.Fatal(err)
	}
	got := WebsiteProbe(context.Background(), Task{URL: target.URL}, NodeClient(pinned))
	if got.Status != "ok" || first.Load() != 1 || second.Load() != 0 {
		t.Fatalf("pinned route changed: %+v first=%d second=%d", got, first.Load(), second.Load())
	}
	got = WebsiteProbe(context.Background(), Task{URL: target.URL}, MixedClient(port))
	if got.Status != "ok" || first.Load() != 1 || second.Load() != 0 {
		t.Fatalf("mixed ignored direct rule: %+v first=%d second=%d", got, first.Load(), second.Load())
	}
	data, _ = core.BuildConfig(nodes, core.Routing{Rules: []string{"MATCH," + core.GroupProxy}}, core.Options{MixedPort: port})
	if err = core.Start(data); err != nil {
		t.Fatal(err)
	}
	selector = tunnel.Proxies()[core.GroupProxy].Adapter().(*outboundgroup.Selector)
	_ = selector.Set("node-1")
	got = WebsiteProbe(context.Background(), Task{URL: target.URL}, MixedClient(port))
	if got.Status != "ok" || second.Load() != 1 {
		t.Fatalf("mixed ignored proxy rule: %+v second=%d", got, second.Load())
	}
}
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "fly-check-core-test-")
	if err != nil {
		panic(err)
	}
	if err = core.Init(dir); err != nil {
		panic(err)
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}
