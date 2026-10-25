// Package checks owns one bounded, cancelable diagnostic job. It never changes node selection.
package checks

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

var ErrBusy = errors.New("已有测试正在进行，请等待或取消")

type Task struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}
type Result struct {
	Name       string    `json:"name"`
	URL        string    `json:"url"`
	Status     string    `json:"status"`
	Delay      int       `json:"delay,omitempty"`
	HTTPStatus int       `json:"http_status,omitempty"`
	Message    string    `json:"message,omitempty"`
	At         time.Time `json:"at"`
}
type Job struct {
	ID           string    `json:"id"`
	Mode         string    `json:"mode"`
	Node         string    `json:"node"`
	Route        string    `json:"route"`
	Generation   uint64    `json:"generation"`
	StartedAt    time.Time `json:"started_at"`
	FinishedAt   time.Time `json:"finished_at"`
	Done         bool      `json:"done"`
	Stale        bool      `json:"stale"`
	CancelReason string    `json:"cancel_reason,omitempty"`
	Completed    int       `json:"completed"`
	Results      []Result  `json:"results"`
}
type Manager struct {
	latencies         map[string]Result
	latencyGeneration uint64
	mu                sync.Mutex
	workers           int
	timeout           time.Duration
	seq               uint64
	current           *Job
	cancel            context.CancelFunc
}

func New(workers int, timeout time.Duration) *Manager {
	if workers < 1 {
		workers = 1
	}
	return &Manager{workers: workers, timeout: timeout}
}
func (m *Manager) Start(mode, node, route string, generation uint64, tasks []Task, probe func(context.Context, Task) Result) (*Job, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.current != nil && !m.current.Done {
		return nil, ErrBusy
	}
	if len(tasks) == 0 || len(tasks) > 2000 || probe == nil {
		return nil, fmt.Errorf("没有可测试的项目，或项目超过 2000 个")
	}
	if m.latencyGeneration != generation || m.latencies == nil {
		m.latencies = map[string]Result{}
		m.latencyGeneration = generation
	}
	m.seq++
	job := &Job{ID: fmt.Sprint(m.seq), Mode: mode, Node: node, Route: route, Generation: generation, StartedAt: time.Now(), Results: make([]Result, len(tasks))}
	for i, t := range tasks {
		job.Results[i] = Result{Name: t.Name, URL: t.URL, Status: "pending"}
	}
	ctx, cancel := context.WithTimeout(context.Background(), m.timeout)
	m.cancel = cancel
	m.current = job
	go m.run(ctx, cancel, job, tasks, probe)
	return clone(job), nil
}
func (m *Manager) run(ctx context.Context, cancel context.CancelFunc, job *Job, tasks []Task, probe func(context.Context, Task) Result) {
	defer cancel()
	queue := make(chan int)
	var wg sync.WaitGroup
	for n := 0; n < m.workers; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range queue {
				if ctx.Err() != nil {
					continue
				}
				m.mu.Lock()
				job.Results[i].Status = "running"
				m.mu.Unlock()
				result := probe(ctx, tasks[i])
				result.Name = tasks[i].Name
				result.URL = tasks[i].URL
				result.At = time.Now()
				if ctx.Err() != nil {
					result.Status = "canceled"
					result.Delay = 0
				}
				m.mu.Lock()
				job.Results[i] = result
				if job.Mode == "latency" && m.latencyGeneration == job.Generation && result.Status != "canceled" {
					m.latencies[result.Name] = result
				}
				job.Completed++
				m.mu.Unlock()
			}
		}()
	}
dispatch:
	for i := range tasks {
		select {
		case queue <- i:
		case <-ctx.Done():
			break dispatch
		}
	}
	close(queue)
	wg.Wait()
	m.mu.Lock()
	defer m.mu.Unlock()
	if ctx.Err() != nil && job.CancelReason == "" {
		job.CancelReason = "任务达到时间上限，未完成项目未判定为节点失败"
	}
	for i := range job.Results {
		r := &job.Results[i]
		if r.Status == "pending" || r.Status == "running" {
			r.Status = "canceled"
			r.Message = "未完成测试"
		}
	}
	job.Done = true
	job.FinishedAt = time.Now()
}
func clone(job *Job) *Job {
	if job == nil {
		return nil
	}
	copy := *job
	copy.Results = append([]Result{}, job.Results...)
	return &copy
}
func (m *Manager) Snapshot(generation uint64) *Job {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.current != nil && m.current.Generation != generation {
		m.current.Stale = true
		if !m.current.Done {
			m.current.CancelReason = "订阅或规则已变更，请重新测试"
			m.cancel()
		}
	}
	return clone(m.current)
}
func (m *Manager) Cancel(id, reason string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.current == nil {
		return nil
	}
	if id != "" && id != m.current.ID {
		return fmt.Errorf("测试任务已变化，请刷新后重试")
	}
	if !m.current.Done {
		m.current.CancelReason = reason
		m.cancel()
	}
	return nil
}

func (m *Manager) Latencies(generation uint64) map[string]Result {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := map[string]Result{}
	if m.latencyGeneration == generation {
		for k, v := range m.latencies {
			out[k] = v
		}
	}
	return out
}
