package server

import (
	"context"
	"embed"
	"encoding/json"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"fly/internal/app"
	"fly/internal/core"
	"fly/internal/sub"
	"github.com/metacubex/mihomo/adapter/outboundgroup"
	"github.com/metacubex/mihomo/component/profile/cachefile"
	"github.com/metacubex/mihomo/tunnel"
)

//go:embed all:ui
var content embed.FS

type Server struct {
	app      *app.App
	version  string
	http     *http.Server
	testing  atomic.Bool
	updating atomic.Bool
	updateEr atomic.Value // string
}

func New(a *app.App, version string) *Server {
	s := &Server{app: a, version: version}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", s.handleStatic)
	mux.HandleFunc("GET /api/info", s.handleInfo)
	mux.HandleFunc("GET /api/subscription", s.handleGetSubscription)
	mux.HandleFunc("POST /api/subscription", s.handleSetSubscription)
	mux.HandleFunc("POST /api/update", s.handleUpdate)
	mux.HandleFunc("GET /panel", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/", http.StatusSeeOther) })
	mux.HandleFunc("GET /panel/", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/", http.StatusSeeOther) })
	mux.HandleFunc("GET /api/proxies", s.handleProxies)
	mux.HandleFunc("PUT /api/select", s.handleSelect)
	mux.HandleFunc("POST /api/test", s.handleTest)
	mux.HandleFunc("GET /api/rules", s.handleRules)
	mux.HandleFunc("GET /api/routing", s.handleRouting)
	mux.HandleFunc("PUT /api/routing", s.handleSaveRouting)
	mux.HandleFunc("GET /api/rule-providers/{name}", s.handleProviderRules)
	s.http = &http.Server{
		Addr:              a.Cfg.AdminAddr,
		Handler:           sameOrigin(mux, a.Cfg.AdminAddr),
		ReadHeaderTimeout: 5 * time.Second,
	}
	return s
}

func (s *Server) Start() error {
	listener, err := net.Listen("tcp", s.http.Addr)
	if err != nil {
		return err
	}
	go func() {
		if err := s.http.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Printf("管理网页服务退出: %v", err)
		}
	}()
	return nil
}

func sameOrigin(next http.Handler, addr string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, port, err := net.SplitHostPort(r.Host)
		_, expectedPort, _ := net.SplitHostPort(addr)
		ip := net.ParseIP(host)
		if err != nil || port != expectedPort || (host != "localhost" && (ip == nil || !ip.IsLoopback())) {
			http.Error(w, "仅允许本机管理地址", 403)
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" {
			u, err := url.Parse(origin)
			if err != nil || u.Host != r.Host || u.Scheme != "http" {
				http.Error(w, "不允许跨站访问", 403)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(v)
}
func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	root, err := fs.Sub(content, "ui")
	if err != nil {
		http.Error(w, "管理网页资源缺失", 500)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/_next/static/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "no-store")
	}
	http.FileServer(http.FS(root)).ServeHTTP(w, r)
}
func (s *Server) handleInfo(w http.ResponseWriter, r *http.Request) {
	info, nodes, skipped, lastErr := s.app.Snapshot()
	current, auto := s.queryCurrent()
	resp := map[string]any{"version": s.version, "proxy_port": s.app.Cfg.MixedPort, "admin_addr": s.app.Cfg.AdminAddr, "nodes": nodes, "skipped": skipped, "last_err": lastErr, "updating": s.updating.Load(), "ready": core.Ready(s.app.Cfg.MixedPort), "needs_subscription": !s.app.HasSubscription(), "starting": s.app.Starting(), "current": current, "auto_now": auto}
	imported, route := s.app.SourceStatus()
	resp["imported"] = imported
	resp["download_route"] = route
	resp["generation"] = s.app.Generation()
	if e, ok := s.updateEr.Load().(string); ok {
		resp["update_err"] = e
	}
	if info != nil {
		resp["subscription"] = map[string]any{"used": info.Used(), "total": info.Total, "expire": info.Expire, "fetched_at": info.FetchedAt.Unix()}
	}
	writeJSON(w, resp)
}
func (s *Server) queryCurrent() (string, string) {
	core.Runtime.RLock()
	defer core.Runtime.RUnlock()
	get := func(name string) string {
		if p := tunnel.Proxies()[name]; p != nil {
			if g, ok := p.Adapter().(interface{ Now() string }); ok {
				return g.Now()
			}
		}
		return ""
	}
	return get(core.GroupProxy), get(core.GroupAuto)
}
func (s *Server) handleProxies(w http.ResponseWriter, r *http.Request) {
	core.Runtime.RLock()
	defer core.Runtime.RUnlock()
	writeJSON(w, map[string]any{"proxies": tunnel.Proxies()})
}
func (s *Server) handleSelect(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	if json.NewDecoder(io.LimitReader(r.Body, 8192)).Decode(&body) != nil {
		http.Error(w, "节点无效", 400)
		return
	}
	core.Runtime.RLock()
	defer core.Runtime.RUnlock()
	p := tunnel.Proxies()[core.GroupProxy]
	if p == nil {
		http.Error(w, "代理正在启动", 503)
		return
	}
	selector, ok := p.Adapter().(outboundgroup.SelectAble)
	if !ok {
		http.Error(w, "节点选择不可用", 503)
		return
	}
	if err := selector.Set(body.Name); err != nil {
		http.Error(w, "节点不存在", 400)
		return
	}
	cachefile.Cache().SetSelected(core.GroupProxy, body.Name)
	writeJSON(w, map[string]bool{"ok": true})
}
func (s *Server) handleTest(w http.ResponseWriter, r *http.Request) {
	if !s.testing.CompareAndSwap(false, true) {
		http.Error(w, "正在测速", 409)
		return
	}
	defer s.testing.Store(false)
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	var mu sync.Mutex
	var wg sync.WaitGroup
	limit := make(chan struct{}, 8)
	results := map[string]uint16{}
	for name, p := range tunnel.Proxies() {
		if name == core.GroupProxy || name == core.GroupAuto || name == "DIRECT" || name == "REJECT" || name == "GLOBAL" || name == "COMPATIBLE" || name == "PASS" {
			continue
		}
		limit <- struct{}{}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-limit }()
			c, done := context.WithTimeout(ctx, 3*time.Second)
			defer done()
			delay, err := p.URLTest(c, "https://www.gstatic.com/generate_204", nil)
			if err != nil {
				delay = 0
			}
			mu.Lock()
			results[name] = delay
			mu.Unlock()
		}()
	}
	wg.Wait()
	writeJSON(w, results)
}
func (s *Server) handleRules(w http.ResponseWriter, r *http.Request) {
	core.Runtime.RLock()
	defer core.Runtime.RUnlock()
	rules := []map[string]any{}
	for _, rule := range tunnel.Rules() {
		rules = append(rules, map[string]any{"type": rule.RuleType().String(), "payload": rule.Payload(), "target": rule.Adapter()})
	}
	providers := []map[string]any{}
	for name, p := range tunnel.RuleProviders() {
		providers = append(providers, map[string]any{"name": name, "count": p.Count()})
	}
	writeJSON(w, map[string]any{"rules": rules, "providers": providers})
}

func (s *Server) handleGetSubscription(w http.ResponseWriter, r *http.Request) {
	cfg := s.app.ConfigSnapshot()
	writeJSON(w, map[string]any{"url": cfg.SubscriptionURL, "download_proxy": cfg.DownloadProxy, "imported": cfg.Imported})
}
func (s *Server) handleSetSubscription(w http.ResponseWriter, r *http.Request) {
	var body struct {
		URL           string  `json:"url"`
		DownloadProxy string  `json:"download_proxy"`
		Content       *string `json:"content"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 2*sub.MaxBody+65536)
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "输入无效或文件超过 8 MiB", 400)
		return
	}
	if s.app.Starting() {
		http.Error(w, "正在启动，请稍后保存", 409)
		return
	}
	if !s.updating.CompareAndSwap(false, true) {
		http.Error(w, "订阅正在更新，请稍后保存", 409)
		return
	}
	defer s.updating.Store(false)
	s.updateEr.Store("")
	var content []byte
	if body.Content != nil {
		content = []byte(*body.Content)
	}
	if err := s.app.ChangeSubscription(r.Context(), body.URL, body.DownloadProxy, content); err != nil {
		safe := sub.Redact(err.Error())
		s.updateEr.Store(safe)
		status := 400
		if strings.Contains(safe, "保存") || strings.Contains(safe, "磁盘") {
			status = 500
		}
		http.Error(w, safe, status)
		return
	}
	writeJSON(w, map[string]any{"ok": true, "message": "订阅已验证并启用"})
}

func (s *Server) handleUpdate(w http.ResponseWriter, r *http.Request) {
	if !s.app.HasSubscription() {
		http.Error(w, "请先在管理网页填写订阅地址", 400)
		return
	}
	if s.app.Starting() {
		http.Error(w, "正在启动，请稍后更新", 409)
		return
	}
	if !s.updating.CompareAndSwap(false, true) {
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"ok":true,"message":"已在刷新中"}`))
		return
	}
	s.updateEr.Store("")
	go func() {
		defer s.updating.Store(false)
		if err := s.app.Refresh(); err != nil {
			s.updateEr.Store(err.Error())
		}
	}()
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true,"message":"已开始刷新"}`))
}
