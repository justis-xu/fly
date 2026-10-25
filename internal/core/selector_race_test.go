package core

import (
	"github.com/metacubex/mihomo/adapter/outboundgroup"
	"github.com/metacubex/mihomo/tunnel"
	"sync"
	"testing"
)

func TestSelectingNodeConcurrentWithReadingCurrentNode(t *testing.T) {
	if err := Init(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	data, err := BuildConfig([]map[string]any{{"name": "A", "type": "http", "server": "127.0.0.1", "port": 1}}, Routing{Rules: []string{"MATCH,DIRECT"}}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err = Start(data); err != nil {
		t.Fatal(err)
	}
	selector := tunnel.Proxies()[GroupProxy].Adapter().(*outboundgroup.Selector)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			_ = selector.Set("A")
			_ = selector.Set(GroupAuto)
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			selector.Now()
		}
	}()
	wg.Wait()
}
