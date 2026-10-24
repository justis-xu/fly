package app

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"gopkg.in/yaml.v3"

	"fly/internal/config"
	"fly/internal/core"
	"fly/internal/sub"
)

// App 聚合运行时状态，供 CLI 与管理 API 共用。
type App struct {
	Dir string
	Cfg *config.Config

	refreshMu sync.Mutex
	mu        sync.Mutex
	Info      *sub.Info
	Nodes     int
	Skipped   int
	LastErr   string
	cacheBody []byte
	cfgBytes  []byte
	starting  bool // 订阅尚未加载完成（控制台已可访问，代理未就绪）
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
	u := a.SubscriptionURL()
	if u == "" {
		return fmt.Errorf("请先在管理网页填写订阅地址")
	}
	body, info, err := sub.Fetch(u)
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
	if err := a.applyResult(proxies, rules, skipped, info); err != nil {
		return err
	}
	a.cacheBody = body
	a.setLastErr("")
	return nil
}

func parseBody(body []byte) (proxies []map[string]any, rules core.Routing, skipped int, err error) {
	links, yamlProxies, yamlRules := sub.BodyToLinks(body)
	if yamlProxies != nil {
		proxies = sub.FilterProxies(yamlProxies)
		skipped = len(yamlProxies) - len(proxies)
		if len(yamlRules) > 0 {
			rules.Rules = sub.RewriteRules(yamlRules, core.GroupProxy)
			var source struct {
				Providers map[string]map[string]any `yaml:"rule-providers"`
				SubRules  map[string][]string       `yaml:"sub-rules"`
			}
			if err = yaml.Unmarshal(body, &source); err != nil {
				return
			}
			rules.Providers = source.Providers
			rules.SubRules = source.SubRules
			for name, list := range rules.SubRules {
				rules.SubRules[name] = sub.RewriteSubRules(list, core.GroupProxy)
			}
		}
	} else {
		proxies, skipped = sub.ParseLinks(links)
	}
	if len(proxies) == 0 {
		return nil, core.Routing{}, 0, fmt.Errorf("订阅内容解析不出节点")
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
	return a.applyResult(proxies, rules, skipped, info)
}

func (a *App) applyResult(proxies []map[string]any, rules core.Routing, skipped int, info *sub.Info) error {
	cfgBytes, err := core.BuildConfig(proxies, rules, core.Options{
		MixedPort: a.Cfg.MixedPort,
	})
	if err != nil {
		return err
	}
	a.mu.Lock()
	a.Info = info
	a.Nodes = len(proxies)
	a.Skipped = skipped
	a.cfgBytes = cfgBytes
	a.mu.Unlock()
	return nil
}

// StartCore 启动内核；再次调用即热重载（订阅更新）。
func (a *App) StartCore() error {
	a.mu.Lock()
	cfgBytes := a.cfgBytes
	a.mu.Unlock()
	if len(cfgBytes) == 0 {
		return fmt.Errorf("尚未加载订阅")
	}
	if err := core.Start(cfgBytes); err != nil {
		return err
	}
	if a.cacheBody != nil {
		sub.SaveCache(a.Dir, a.cacheBody, a.Info)
	}
	return nil
}

// Refresh 重新拉取订阅并热重载。
func (a *App) Refresh() error {
	a.refreshMu.Lock()
	defer a.refreshMu.Unlock()
	a.mu.Lock()
	cfg := *a.Cfg
	a.mu.Unlock()
	candidate := &App{Dir: a.Dir, Cfg: &cfg}
	err := candidate.LoadSubscription()
	if err == nil {
		err = candidate.StartCore()
	}
	if err != nil {
		original := err
		cached := &App{Dir: a.Dir, Cfg: &cfg}
		if err = cached.loadFromCache(); err == nil {
			err = cached.StartCore()
		}
		if err != nil {
			return fmt.Errorf("订阅加载失败: %v（缓存不可用: %w）", original, err)
		}
		cached.setLastErr(original.Error())
		candidate = cached
	}
	a.mu.Lock()
	a.Info, a.Nodes, a.Skipped, a.LastErr, a.cfgBytes = candidate.Info, candidate.Nodes, candidate.Skipped, candidate.LastErr, candidate.cfgBytes
	a.mu.Unlock()
	return nil
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

func (a *App) SubscriptionURL() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.Cfg.SubscriptionURL
}
func (a *App) SetSubscriptionURL(u string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	old := a.Cfg.SubscriptionURL
	a.Cfg.SubscriptionURL = u
	if err := config.Save(a.Dir, a.Cfg); err != nil {
		a.Cfg.SubscriptionURL = old
		return err
	}
	return nil
}

// Initialize keeps the management page available even if a subscription is absent or invalid.
func (a *App) Initialize() error {
	a.SetStarting(a.SubscriptionURL() != "")
	defer a.SetStarting(false)
	if a.SubscriptionURL() == "" {
		return nil
	}
	if err := a.Refresh(); err != nil {
		a.setLastErr(err.Error())
		return err
	}
	return nil
}
