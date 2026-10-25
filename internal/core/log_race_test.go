package core

import (
	"sync"
	"testing"

	mihomolog "github.com/metacubex/mihomo/log"
)

// Reload sets the log level while provider health checks emit debug messages.
func TestLogLevelConcurrentWithHealthMessages(t *testing.T) {
	old := mihomolog.Level()
	defer mihomolog.SetLevel(old)
	var wg sync.WaitGroup
	start := make(chan struct{})
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		for range 1000 {
			mihomolog.SetLevel(mihomolog.WARNING)
		}
	}()
	go func() {
		defer wg.Done()
		<-start
		for range 1000 {
			_ = mihomolog.Level()
			mihomolog.Debugln("fly health-check log concurrency regression")
		}
	}()
	close(start)
	wg.Wait()
}
