package server

import (
	"context"
	"encoding/json"
	"fly/internal/checks"
	"fly/internal/core"
	"fmt"
	C "github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/tunnel"
	"net/http"
	"sort"
	"time"
)

func (s *Server) handleChecks(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{"job": s.checks.Snapshot(s.app.Generation()), "targets": checks.Websites, "latency_url": checks.LatencyURL})
}
func (s *Server) handleCancelChecks(w http.ResponseWriter, r *http.Request) {
	if err := s.checks.Cancel(r.URL.Query().Get("id"), "用户取消，未完成项目不代表节点失败"); err != nil {
		http.Error(w, err.Error(), 409)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}
func (s *Server) handleStartChecks(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Mode  string   `json:"mode"`
		Names []string `json:"names"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 128<<10)).Decode(&body) != nil {
		http.Error(w, "测试参数无效", 400)
		return
	}
	job, status, err := s.startChecks(body.Mode, body.Names)
	if err != nil {
		http.Error(w, err.Error(), status)
		return
	}
	writeJSON(w, map[string]any{"job": job})
}
func (s *Server) startChecks(mode string, names []string) (*checks.Job, int, error) {
	if s.updating.Load() || s.app.Starting() {
		return nil, 409, fmt.Errorf("正在加载订阅，请稍后测试")
	}
	if !core.Ready(s.app.Cfg.MixedPort) {
		return nil, 503, fmt.Errorf("本机代理尚未就绪，请先添加可用订阅")
	}
	if len(names) > 2000 {
		return nil, 400, fmt.Errorf("最多测试 2000 个节点")
	}
	var tasks []checks.Task
	var probe func(context.Context, checks.Task) checks.Result
	node := ""
	route := "node"
	switch mode {
	case "latency":
		core.Runtime.RLock()
		available := map[string]C.Proxy{}
		if group := tunnel.Proxies()[core.GroupProxy]; group != nil {
			if g, ok := group.Adapter().(interface{ Proxies() []C.Proxy }); ok {
				for _, p := range g.Proxies() {
					if p.Name() != core.GroupAuto && p.Name() != "DIRECT" {
						available[p.Name()] = p
					}
				}
			}
		}
		core.Runtime.RUnlock()
		if len(names) == 0 {
			for name := range available {
				names = append(names, name)
			}
			sort.Strings(names)
		}
		seen := map[string]bool{}
		for _, name := range names {
			if available[name] == nil || seen[name] {
				return nil, 400, fmt.Errorf("节点不存在或重复，请刷新列表")
			}
			seen[name] = true
			tasks = append(tasks, checks.Task{Name: name, URL: checks.LatencyURL})
		}
		probe = func(ctx context.Context, t checks.Task) checks.Result {
			return checks.LatencyProbe(ctx, available[t.Name], checks.LatencyURL)
		}
	case "sites":
		p, err := core.SelectedProxy()
		if err != nil {
			return nil, 503, err
		}
		node = p.Name()
		tasks = append([]checks.Task{}, checks.Websites...)
		probe = func(ctx context.Context, t checks.Task) checks.Result {
			return checks.WebsiteProbe(ctx, t, checks.NodeClient(p))
		}
	case "browser":
		route = "browser"
		node = "按当前规则分流（可能直连或自动切换）"
		tasks = append([]checks.Task{}, checks.Websites...)
		probe = func(ctx context.Context, t checks.Task) checks.Result {
			return checks.WebsiteProbe(ctx, t, checks.MixedClient(s.app.Cfg.MixedPort))
		}
	default:
		return nil, 400, fmt.Errorf("请选择延迟、当前节点网站或浏览器分流测试")
	}
	job, err := s.checks.Start(mode, node, route, s.app.Generation(), tasks, probe)
	if err != nil {
		return nil, 409, err
	}
	return job, 200, nil
}

// Legacy clients receive only completed measurements, never a fabricated zero for an untested node.
func (s *Server) handleTest(w http.ResponseWriter, r *http.Request) {
	job, status, err := s.startChecks("latency", nil)
	if err != nil {
		http.Error(w, err.Error(), status)
		return
	}
	timer := time.NewTimer(20 * time.Second)
	defer timer.Stop()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		current := s.checks.Snapshot(s.app.Generation())
		if current == nil || current.ID != job.ID {
			http.Error(w, "测试任务已变化，请刷新", 409)
			return
		}
		finish := current.Done
		select {
		case <-r.Context().Done():
			_ = s.checks.Cancel(job.ID, "请求已取消")
			return
		case <-timer.C:
			_ = s.checks.Cancel(job.ID, "兼容测速接口达到 20 秒上限，未完成项目未判定失败")
			finish = true
		case <-ticker.C:
		}
		if finish {
			results := map[string]int{}
			if !current.Stale {
				for _, result := range current.Results {
					if result.Status == "ok" {
						results[result.Name] = result.Delay
					} else if result.Status == "failed" || result.Status == "timeout" {
						results[result.Name] = 0
					}
				}
			}
			writeJSON(w, results)
			return
		}
	}
}
