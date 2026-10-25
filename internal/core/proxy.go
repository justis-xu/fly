package core

import (
	"context"
	"fmt"
	C "github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/listener"
	"github.com/metacubex/mihomo/tunnel"
	"net"
	"net/netip"
	"strconv"
	"sync"
)

// Runtime protects fly's reads and mutations across a complete configuration replacement.
var Runtime sync.RWMutex

// SelectedProxy resolves automatic selection once. A caller never changes the user's selection.
func SelectedProxy() (C.Proxy, error) {
	Runtime.RLock()
	defer Runtime.RUnlock()
	proxies := tunnel.Proxies()
	name := GroupProxy
	for i := 0; i < 4; i++ {
		p := proxies[name]
		if p == nil {
			return nil, fmt.Errorf("节点尚未就绪")
		}
		if g, ok := p.Adapter().(interface{ Now() string }); ok {
			name = g.Now()
			continue
		}
		return p, nil
	}
	return nil, fmt.Errorf("无法确定实际节点")
}
func ProxyDial(p C.Proxy) func(context.Context, string, string) (net.Conn, error) {
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		n, err := strconv.ParseUint(port, 10, 16)
		if err != nil {
			return nil, err
		}
		metadata := &C.Metadata{NetWork: C.TCP, Host: host, DstPort: uint16(n)}
		if ip, err := netip.ParseAddr(host); err == nil {
			metadata.DstIP = ip
			metadata.Host = ""
		}
		return p.DialContext(ctx, metadata)
	}
}

func StopMixed() { Runtime.Lock(); defer Runtime.Unlock(); listener.ReCreateMixed(0, tunnel.Tunnel) }
