package checks

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestJobBoundsConcurrencyCancelsAndNeverMarksUntestedNodesFailed(t *testing.T) {
	var active, peak atomic.Int32
	tasks := make([]Task, 30)
	for i := range tasks {
		tasks[i] = Task{Name: "node"}
	}
	m := New(3, time.Second)
	job, err := m.Start("latency", "fixed", "node", 1, tasks, func(ctx context.Context, task Task) Result {
		current := active.Add(1)
		defer active.Add(-1)
		for old := peak.Load(); current > old && !peak.CompareAndSwap(old, current); old = peak.Load() {
		}
		<-ctx.Done()
		return Result{Status: "canceled"}
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = m.Start("latency", "", "", 1, tasks, nil); !errors.Is(err, ErrBusy) {
		t.Fatal("concurrent check accepted")
	}
	deadline := time.Now().Add(time.Second)
	for active.Load() != 3 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if err = m.Cancel(job.ID, "用户取消"); err != nil {
		t.Fatal(err)
	}
	for !m.Snapshot(1).Done && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	got := m.Snapshot(1)
	if !got.Done || peak.Load() > 3 || len(got.Results) != 30 {
		t.Fatalf("bad job lifecycle: %+v peak=%d", got, peak.Load())
	}
	for _, r := range got.Results {
		if r.Status != "canceled" {
			t.Fatalf("untested result fabricated: %+v", r)
		}
	}
}
func TestResultsKeepTheirActualTimeAndBecomeStaleAfterConfigChange(t *testing.T) {
	m := New(2, time.Second)
	_, err := m.Start("sites", "Pinned node", "node", 7, []Task{{Name: "site"}}, func(context.Context, Task) Result { return Result{Status: "restricted", HTTPStatus: 403} })
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for !m.Snapshot(7).Done && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	got := m.Snapshot(8)
	if !got.Stale || got.Node != "Pinned node" || got.Results[0].At.IsZero() || got.Results[0].HTTPStatus != 403 {
		t.Fatalf("lost provenance: %+v", got)
	}
}
