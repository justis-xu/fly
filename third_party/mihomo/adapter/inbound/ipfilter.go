package inbound

import (
	"net"
	"net/netip"
	"slices"
	"sync"

	C "github.com/metacubex/mihomo/constant"
)

var filterMu sync.RWMutex

var lanAllowedIPs []netip.Prefix
var lanDisAllowedIPs []netip.Prefix

func SetAllowedIPs(prefixes []netip.Prefix) {
	filterMu.Lock()
	defer filterMu.Unlock()
	lanAllowedIPs = slices.Clone(prefixes)
}

func SetDisAllowedIPs(prefixes []netip.Prefix) {
	filterMu.Lock()
	defer filterMu.Unlock()
	lanDisAllowedIPs = slices.Clone(prefixes)
}

func AllowedIPs() []netip.Prefix {
	filterMu.RLock()
	defer filterMu.RUnlock()
	return slices.Clone(lanAllowedIPs)
}

func DisAllowedIPs() []netip.Prefix {
	filterMu.RLock()
	defer filterMu.RUnlock()
	return slices.Clone(lanDisAllowedIPs)
}

func IsRemoteAddrDisAllowed(addr net.Addr) bool {
	m := C.Metadata{}
	if err := m.SetRemoteAddr(addr); err != nil {
		return false
	}
	ipAddr := m.AddrPort().Addr()
	if ipAddr.IsValid() {
		return isAllowed(ipAddr) && !isDisAllowed(ipAddr)
	}
	return false
}

func isAllowed(addr netip.Addr) bool {
	filterMu.RLock()
	defer filterMu.RUnlock()
	return prefixesContains(lanAllowedIPs, addr)
}

func isDisAllowed(addr netip.Addr) bool {
	filterMu.RLock()
	defer filterMu.RUnlock()
	return prefixesContains(lanDisAllowedIPs, addr)
}
