package app

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"fly/internal/config"
	"fly/internal/core"
	"fly/internal/sub"
)

// App 聚合运行时状态，供 CLI 与管理 API 共用。
type App struct {
	Dir string
	Cfg *config.Config

	mu       sync.Mutex
	Info     *sub.Info
	Nodes    int
	Skipped  int
	LastErr  string
	cfgBytes []byte
	starting bool // 订阅尚未加载完成（控制台已可访问，代理未就绪）
}

func (a *App) SetStarting(b bool) {
	a.mu.Lock()
	a.starting = b
	a.mu.Unlock()
}

func (a *App) Starting() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.starting
}

func New() (*App, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(home, ".fly")
	cfg, err := config.Load(dir)
	if err != nil {
		return nil, fmt.Errorf("读取配置失败: %w", err)
	}
	return &App{Dir: dir, Cfg: cfg}, nil
}

// LoadSubscription 拉取（或缓存兜底）并解析订阅，生成 mihomo 配置。
func (a *App) LoadSubscription() error {
	body, info, err := sub.Fetch(a.Cfg.SubscriptionURL)
	if err != nil {
		a.setLastErr(err.Error())
		if cerr := a.loadFromCache(); cerr != nil {
			return fmt.Errorf("拉取订阅失败: %v（%w）", err, cerr)
		}
		return nil
	}

	proxies, rules, skipped, perr := parseBody(body)
	if perr != nil {
		// 拉到了但解析不出节点（可能是网关页/空响应），回退缓存
		a.setLastErr(perr.Error())
		if cerr := a.loadFromCache(); cerr != nil {
			return fmt.Errorf("%v（%w）", perr, cerr)
		}
		return nil
	}
	sub.SaveCache(a.Dir, body, info)
	a.setLastErr("")
	a.applyResult(proxies, rules, skipped, info)
	return nil
}

func parseBody(body []byte) (proxies []map[string]any, rules []string, skipped int, err error) {
	links, yamlProxies, yamlRules := sub.BodyToLinks(body)
	if yamlProxies != nil {
		proxies = sub.FilterProxies(yamlProxies)
		skipped = len(yamlProxies) - len(proxies)
		if len(yamlRules) > 0 {
			rules = sub.RewriteRules(yamlRules, core.GroupProxy)
		}
	} else {
		proxies, skipped = sub.ParseLinks(links)
	}
	if len(proxies) == 0 {
		return nil, nil, 0, fmt.Errorf("订阅内容解析不出节点")
	}
	return proxies, rules, skipped, nil
}

func (a *App) loadFromCache() error {
	body, info, ok := sub.LoadCache(a.Dir)
	if !ok {
		return fmt.Errorf("无可用缓存")
	}
	proxies, rules, skipped, err := parseBody(body)
	if err != nil {
		return fmt.Errorf("缓存也无法解析: %w", err)
	}
	a.applyResult(proxies, rules, skipped, info)
	return nil
}

func (a *App) applyResult(proxies []map[string]any, rules []string, skipped int, info *sub.Info) {
	cfgBytes, err := core.BuildConfig(proxies, rules, core.Options{
		MixedPort:      a.Cfg.MixedPort,
		ControllerAddr: a.Cfg.ControllerAddr,
		Secret:         a.Cfg.Secret,
	})
	if err != nil {
		return // nodes>0 时不会失败
	}
	a.mu.Lock()
	a.Info = info
	a.Nodes = len(proxies)
	a.Skipped = skipped
	a.cfgBytes = cfgBytes
	a.mu.Unlock()
}

// StartCore 启动内核；再次调用即热重载（订阅更新）。
func (a *App) StartCore() error {
	a.mu.Lock()
	cfgBytes := a.cfgBytes
	a.mu.Unlock()
	if len(cfgBytes) == 0 {
		return fmt.Errorf("尚未加载订阅")
	}
	return core.Start(cfgBytes, a.Cfg.ControllerAddr, a.Cfg.Secret)
}

// Refresh 重新拉取订阅并热重载。
func (a *App) Refresh() error {
	if err := a.LoadSubscription(); err != nil {
		return err
	}
	return a.StartCore()
}

func (a *App) setLastErr(s string) {
	a.mu.Lock()
	a.LastErr = s
	a.mu.Unlock()
}

func (a *App) Snapshot() (info *sub.Info, nodes, skipped int, lastErr string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.Info, a.Nodes, a.Skipped, a.LastErr
}
