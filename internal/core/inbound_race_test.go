package core

import (
	"github.com/metacubex/mihomo/adapter/inbound"
	"github.com/metacubex/mihomo/common/queue"
	"github.com/metacubex/mihomo/component/resolver"
	"net"
	"net/netip"
	"sync"
	"testing"
)

// A live listener reads these prefixes while subscription/rule edits reload them.
func TestInboundPoliciesConcurrentWithConnections(t *testing.T) {
	prefixes := []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8")}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			inbound.SetSkipAuthPrefixes(prefixes)
			inbound.SetAllowedIPs(prefixes)
			inbound.SetDisAllowedIPs(nil)
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			inbound.SkipAuthRemoteAddress("127.0.0.1:1234")
			inbound.IsRemoteAddrDisAllowed(&net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 1234})
			inbound.SkipAuthPrefixes()
			inbound.AllowedIPs()
			inbound.DisAllowedIPs()
		}
	}()
	wg.Wait()
}

func TestIP4PPolicyConcurrentWithDials(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			resolver.SetIP4PEnable(false)
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			resolver.GetIP4PEnable()
			resolver.LookupIP4P(netip.MustParseAddr("::1"), "443")
		}
	}()
	wg.Wait()
}

// Automatic selection reads history while latency probes append and trim it.
func TestDelayHistoryConcurrentReadAndWrite(t *testing.T) {
	q := queue.New[int](10)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			q.Put(i)
			q.Pop()
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			q.Last()
			q.Copy()
		}
	}()
	wg.Wait()
}
