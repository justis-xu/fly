package daemon

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestStalePIDNeverIdentifiesAnUnrelatedLiveProcess(t *testing.T) {
	dir := t.TempDir()
	// A recycled PID may be alive but must never authorize stopping that process.
	if err := os.WriteFile(filepath.Join(dir, "fly.pid"), []byte(fmt.Sprint(os.Getpid())), 0600); err != nil {
		t.Fatal(err)
	}
	if pid := Running(dir); pid != 0 {
		t.Fatalf("unrelated process accepted as fly: %d", pid)
	}
}
func TestRecordedIdentityAndExclusiveStartup(t *testing.T) {
	dir := t.TempDir()
	if err := WritePID(dir); err != nil {
		t.Fatal(err)
	}
	defer RemovePID(dir)
	if got := Running(dir); got != os.Getpid() {
		t.Fatalf("own registered process missing: %d", got)
	}
	if err := WritePID(dir); err == nil {
		t.Fatal("duplicate startup replaced active ownership")
	}
}
