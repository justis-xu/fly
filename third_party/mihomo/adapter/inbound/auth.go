package inbound

import (
	"net"
	"net/netip"
	"slices"
	"sync"

	C "github.com/metacubex/mihomo/constant"
)

var authMu sync.RWMutex

var skipAuthPrefixes []netip.Prefix

func SetSkipAuthPrefixes(prefixes []netip.Prefix) {
	authMu.Lock()
	defer authMu.Unlock()
	skipAuthPrefixes = slices.Clone(prefixes)
}

func SkipAuthPrefixes() []netip.Prefix {
	authMu.RLock()
	defer authMu.RUnlock()
	return slices.Clone(skipAuthPrefixes)
}

func SkipAuthRemoteAddr(addr net.Addr) bool {
	m := C.Metadata{}
	if err := m.SetRemoteAddr(addr); err != nil {
		return false
	}
	return skipAuth(m.AddrPort().Addr())
}

func SkipAuthRemoteAddress(addr string) bool {
	m := C.Metadata{}
	if err := m.SetRemoteAddress(addr); err != nil {
		return false
	}
	return skipAuth(m.AddrPort().Addr())
}

func skipAuth(addr netip.Addr) bool {
	authMu.RLock()
	defer authMu.RUnlock()
	return prefixesContains(skipAuthPrefixes, addr)
}
