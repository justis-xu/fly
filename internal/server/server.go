package server

import (
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"fly/internal/app"
	"fly/internal/config"
)

//go:embed all:panel
var panelFS embed.FS

//go:embed index.html
var indexHTML []byte

type Server struct {
	app      *app.App
	version  string
	http     *http.Server
	updating atomic.Bool
	updateEr atomic.Value // string
}

func New(a *app.App, version string) *Server {
	s := &Server{app: a, version: version}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.handleIndex)
	mux.HandleFunc("GET /api/info", s.handleInfo)
	mux.HandleFunc("GET /api/subscription", s.handleGetSubscription)
	mux.HandleFunc("POST /api/subscription", s.handleSetSubscription)
	mux.HandleFunc("POST /api/update", s.handleUpdate)
	mux.HandleFunc("GET /panel/", s.handlePanel)
	mux.HandleFunc("GET /panel", s.handlePanel)
	s.http = &http.Server{
		Addr:              a.Cfg.AdminAddr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	return s
}

func (s *Server) ListenAndServe() error { return s.http.ListenAndServe() }

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(indexHTML)
}

func (s *Server) handlePanel(w http.ResponseWriter, r *http.Request) {
	sub, err := fs.Sub(panelFS, "panel")
	if err != nil {
		http.Error(w, "panel missing", http.StatusInternalServerError)
		return
	}
	p := strings.TrimPrefix(r.URL.Path, "/panel/")
	if p == "" {
		p = "index.html"
	}
	if _, err := fs.Stat(sub, p); err != nil {
		b, ferr := fs.ReadFile(sub, "index.html")
		if ferr != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(b) // SPA 路由回退
		return
	}
	http.StripPrefix("/panel/", http.FileServer(http.FS(sub))).ServeHTTP(w, r)
}

func (s *Server) handleInfo(w http.ResponseWriter, r *http.Request) {
	info, nodes, skipped, lastErr := s.app.Snapshot()
	host, portStr, _ := strings.Cut(s.app.Cfg.ControllerAddr, ":")
	port, _ := strconv.Atoi(portStr)

	resp := map[string]any{
		"version":          s.version,
		"pid":              os.Getpid(),
		"proxy_port":       s.app.Cfg.MixedPort,
		"subscription_url": s.app.Cfg.SubscriptionURL,
		"controller": map[string]any{
			"host": host, "port": port, "secret": s.app.Cfg.Secret,
		},
		"admin_addr": s.app.Cfg.AdminAddr,
		"nodes":      nodes,
		"skipped":    skipped,
		"last_err":   lastErr,
		"updating":   s.updating.Load(),
		"starting":   s.app.Starting(),
	}
	if e, ok := s.updateEr.Load().(string); ok && e != "" {
		resp["update_err"] = e
	}
	if info != nil {
		resp["subscription"] = map[string]any{
			"upload":     info.Upload,
			"download":   info.Download,
			"total":      info.Total,
			"used":       info.Used(),
			"expire":     info.Expire,
			"fetched_at": info.FetchedAt.Unix(),
		}
	}
	if cur, auto := s.queryCurrent(); cur != "" {
		resp["current"] = cur
		resp["auto_now"] = auto
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// queryCurrent 服务端代查当前选中节点（给 CLI status 用），尽力而为。
func (s *Server) queryCurrent() (current, autoNow string) {
	host, portStr, _ := strings.Cut(s.app.Cfg.ControllerAddr, ":")
	get := func(name string) string {
		u := fmt.Sprintf("http://%s:%s/proxies/%s", host, portStr, url.PathEscape(name))
		req, _ := http.NewRequest(http.MethodGet, u, nil)
		req.Header.Set("Authorization", "Bearer "+s.app.Cfg.Secret)
		client := &http.Client{Timeout: 3 * time.Second, Transport: noProxyTransport()}
		resp, err := client.Do(req)
		if err != nil {
			return ""
		}
		defer resp.Body.Close()
		var out struct {
			Now string `json:"now"`
		}
		if json.NewDecoder(resp.Body).Decode(&out) != nil {
			return ""
		}
		return out.Now
	}
	return get("🚀 Proxy"), get("♻️ 自动选择")
}

func (s *Server) handleGetSubscription(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"url": s.app.Cfg.SubscriptionURL})
}

func (s *Server) handleSetSubscription(w http.ResponseWriter, r *http.Request) {
	var body struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 8192)).Decode(&body); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	u := strings.TrimSpace(body.URL)
	if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
		http.Error(w, `{"error":"订阅地址需以 http(s):// 开头"}`, http.StatusBadRequest)
		return
	}
	s.app.Cfg.SubscriptionURL = u
	if err := config.Save(s.app.Dir, s.app.Cfg); err != nil {
		http.Error(w, `{"error":"保存配置失败"}`, http.StatusInternalServerError)
		return
	}
	if !s.updating.CompareAndSwap(false, true) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"message":"已保存，正在刷新"}`))
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
	_, _ = w.Write([]byte(`{"ok":true,"message":"已保存，正在刷新订阅"}`))
}

func (s *Server) handleUpdate(w http.ResponseWriter, r *http.Request) {
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

func noProxyTransport() *http.Transport {
	return &http.Transport{Proxy: nil}
}
