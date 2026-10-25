package daemon

import (
	"os"
	"testing"
)

func TestRegressionIdentityTZ(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TZ", "UTC")
	if err := WritePID(dir); err != nil {
		t.Fatal(err)
	}
	defer RemovePID(dir)
	t.Setenv("TZ", "Asia/Shanghai")
	t.Setenv("LC_ALL", "zh_CN.UTF-8")
	if got := Running(dir); got != os.Getpid() {
		t.Fatalf("same live process no longer recognized: got %d", got)
	}
	// A failed process lookup must retain a record that could still be valid.
	t.Setenv("PATH", t.TempDir())
	if got := Running(dir); got != 0 {
		t.Fatal("unexpected identity without ps")
	}
	if _, err := os.Stat(pidPath(dir)); err != nil {
		t.Fatal("temporary lookup failure destroyed PID record")
	}
}
