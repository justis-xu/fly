package app

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"

	"fly/internal/config"
	"fly/internal/core"
	"fly/internal/sub"
)

// App 聚合运行时状态，供 CLI 与管理 API 共用。
type App struct {
	Dir string
	Cfg *config.Config

	refreshMu         sync.Mutex
	mu                sync.Mutex
	Info              *sub.Info
	Nodes             int
	Skipped           int
	LastErr           string
	cacheBody         []byte
	cfgBytes          []byte
	resourcesPrepared bool
	resourceRefresh   bool
	DownloadRoute     string
	generation        uint64
	starting          bool // 订阅尚未加载完成（控制台已可访问，代理未就绪）
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
	if custom := os.Getenv("FLY_HOME"); custom != "" {
		if !filepath.IsAbs(custom) {
			return nil, fmt.Errorf("FLY_HOME 须为绝对路径")
		}
		dir = filepath.Clean(custom)
	}
	cfg, err := config.Load(dir)
	if err != nil {
		return nil, fmt.Errorf("读取配置失败: %w", err)
	}
	return &App{Dir: dir, Cfg: cfg}, nil
}

func (a *App) ConfigSnapshot() config.Config { a.mu.Lock(); defer a.mu.Unlock(); return *a.Cfg }
func (a *App) HasSubscription() bool {
	c := a.ConfigSnapshot()
	return c.Imported || c.SubscriptionURL != ""
}
func (a *App) Generation() uint64 { a.mu.Lock(); defer a.mu.Unlock(); return a.generation }
func (a *App) SourceStatus() (bool, string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.Cfg.Imported, a.DownloadRoute
}
func (a *App) fetchOptions() sub.FetchOptions {
	c := a.ConfigSnapshot()
	opt := sub.FetchOptions{ProxyURL: c.DownloadProxy}
	if p, err := core.SelectedProxy(); err == nil {
		opt.FallbackDial = core.ProxyDial(p)
	}
	return opt
}
func (a *App) sourceKey() string {
	if a.Cfg.Imported {
		return "local-import"
	}
	return a.Cfg.SubscriptionURL
}

// LoadSubscription prepares a candidate. Cache fallback is only for its accepted source.
func (a *App) LoadSubscription() error {
	if !a.HasSubscription() {
		return fmt.Errorf("请先在管理网页填写订阅地址或导入订阅文件")
	}
	if a.Cfg.Imported {
		return a.loadFromCache()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	body, info, route, err := sub.FetchWithOptions(ctx, a.SubscriptionURL(), a.fetchOptions())
	if err == nil {
		body, err = sub.ExpandProviders(ctx, body, a.fetchOptions())
	}
	if err == nil {
		err = a.prepare(body, info)
		a.DownloadRoute = route
		a.resourceRefresh = true
	}
	if err == nil {
		return nil
	}
	a.setLastErr(sub.Redact(err.Error()))
	if cacheErr := a.loadFromCache(); cacheErr != nil {
		return fmt.Errorf("%s（此订阅无可用缓存）", sub.Redact(err.Error()))
	}
	return nil
}
func (a *App) prepare(body []byte, info *sub.Info) error {
	if len(body) > sub.MaxBody {
		return fmt.Errorf("订阅文件超过 8 MiB")
	}
	if bytes.HasPrefix(bytes.TrimSpace(body), []byte("<")) {
		return fmt.Errorf("收到的是网页而非订阅，请复制服务商提供的 Clash/Mihomo 订阅链接")
	}
	proxies, rules, skipped, err := parseBody(body)
	if err != nil {
		return err
	}
	if err = a.applyResult(proxies, rules, skipped, info); err != nil {
		return err
	}
	a.cacheBody = body
	return nil
}

func parseBody(body []byte) (proxies []map[string]any, rules core.Routing, skipped int, err error) {
	links, yamlProxies, yamlRules := sub.BodyToLinks(body)
	if yamlProxies != nil {
		proxies = sub.FilterProxies(yamlProxies)
		skipped = len(yamlProxies) - len(proxies)
		if len(yamlRules) > 0 {
			for i, line := range yamlRules {
				parts := strings.Split(line, ",")
				valid := len(parts) >= 3 || len(parts) == 2 && strings.EqualFold(strings.TrimSpace(parts[0]), "MATCH")
				if !valid {
					return nil, core.Routing{}, 0, fmt.Errorf("订阅第 %d 条分流规则格式不完整，请修正规则源", i+1)
				}
			}
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
	if len(proxies) > 2000 {
		return nil, core.Routing{}, 0, fmt.Errorf("最多支持 2000 个节点，请精简订阅")
	}
	if len(proxies) == 0 {
		return nil, core.Routing{}, 0, fmt.Errorf("没有受支持的有效节点：请使用含 proxies 的 Clash/Mihomo YAML，或 SS/VLESS/VMess/Trojan/Hysteria2 分享链接列表；不要填写服务商登录页")
	}
	return proxies, rules, skipped, nil
}

func (a *App) loadFromCache() error {
	body, info, ok := sub.LoadSourceCache(a.Dir, a.sourceKey())
	if !ok && !a.Cfg.Imported {
		// One-time compatibility path: only a cold start of the accepted URL may migrate legacy data.
		if !sub.HasSourceCaches(a.Dir) {
			body, info, ok = sub.LoadCache(a.Dir)
		}
	}
	if !ok {
		return fmt.Errorf("此订阅无可用缓存")
	}
	if err := a.prepare(body, info); err != nil {
		return fmt.Errorf("缓存无效：%w", err)
	}
	a.resourceRefresh = false
	a.DownloadRoute = "本地文件"
	if !a.Cfg.Imported {
		a.DownloadRoute = "缓存"
	}
	if info.LegacyCache {
		a.DownloadRoute = "旧缓存（来源未核实）"
		a.setLastErr(a.LastErr + "；升级前旧缓存未记录来源，请联网更新订阅后确认节点")
	}
	return nil
}

func (a *App) applyResult(proxies []map[string]any, rules core.Routing, skipped int, info *sub.Info) error {
	overrides, err := config.LoadRules(a.Dir)
	if err != nil {
		return err
	}
	cfgBytes, err := core.BuildConfig(proxies, rules, core.Options{
		UserRules: config.RuleLines(overrides, core.GroupProxy),
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

// StartCore activates a prepared candidate. Persistence belongs to the transaction below.
func (a *App) StartCore() error {
	a.mu.Lock()
	data := a.cfgBytes
	a.mu.Unlock()
	if len(data) == 0 {
		return fmt.Errorf("尚未加载订阅")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	if !a.resourcesPrepared {
		if err := a.prepareRuleResources(ctx); err != nil {
			return err
		}
	}
	if err := core.Start(data); err != nil {
		return fmt.Errorf("订阅内核配置无效：%s", sub.Redact(err.Error()))
	}
	return nil
}

// Refresh never replaces a working runtime with a failed download or unrelated cache.
func (a *App) Refresh() error {
	a.refreshMu.Lock()
	defer a.refreshMu.Unlock()
	cfg := a.ConfigSnapshot()
	candidate := &App{Dir: a.Dir, Cfg: &cfg}
	err := candidate.LoadSubscription()
	a.mu.Lock()
	hasRuntime := len(a.cfgBytes) > 0
	a.mu.Unlock()
	if err == nil && candidate.LastErr != "" && hasRuntime {
		err = fmt.Errorf("%s；继续保留当前配置", candidate.LastErr)
	}
	if err == nil {
		err = candidate.StartCore()
	}
	if err != nil && !hasRuntime && a.HasSubscription() {
		cached := &App{Dir: a.Dir, Cfg: &cfg}
		if cacheErr := cached.loadFromCache(); cacheErr == nil {
			if cacheErr = cached.StartCore(); cacheErr == nil {
				cached.LastErr = sub.Redact(err.Error())
				candidate = cached
				err = nil
			}
		}
	}
	if err != nil {
		a.setLastErr(sub.Redact(err.Error()))
		return err
	}
	if err = sub.SaveSourceCache(a.Dir, candidate.sourceKey(), candidate.cacheBody, candidate.Info); err != nil {
		if restoreErr := a.rollbackCore(); restoreErr != nil {
			a.setLastErr("缓存保存失败，旧配置恢复失败，请检查端口并重新启动")
			return fmt.Errorf("缓存保存失败，旧配置恢复失败，请检查端口并重新启动")
		}
		a.setLastErr("缓存保存失败，原配置已保留，请检查数据目录权限/磁盘空间")
		return fmt.Errorf("缓存保存失败，原配置已保留，请检查数据目录权限/磁盘空间")
	}
	a.adopt(candidate)
	return nil
}
func (a *App) adopt(candidate *App) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.Info, a.Nodes, a.Skipped, a.LastErr, a.cfgBytes, a.cacheBody = candidate.Info, candidate.Nodes, candidate.Skipped, candidate.LastErr, candidate.cfgBytes, candidate.cacheBody
	a.Cfg.SubscriptionURL = candidate.Cfg.SubscriptionURL
	a.Cfg.DownloadProxy = candidate.Cfg.DownloadProxy
	a.Cfg.Imported = candidate.Cfg.Imported
	a.DownloadRoute = candidate.DownloadRoute
	a.generation++
}

// ChangeSubscription validates a completely new source without consulting the old cache.
// The request is synchronous so a successful response means the candidate really activated.
func (a *App) ChangeSubscription(ctx context.Context, rawURL, downloadProxy string, imported []byte) error {
	a.refreshMu.Lock()
	defer a.refreshMu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	rawURL = strings.TrimSpace(rawURL)
	downloadProxy = strings.TrimSpace(downloadProxy)
	if err := sub.ValidateProxy(downloadProxy); err != nil {
		return err
	}
	old := a.ConfigSnapshot()
	cfg := old
	cfg.SubscriptionURL = rawURL
	cfg.DownloadProxy = downloadProxy
	cfg.Imported = imported != nil
	if err := cfg.Validate(); err != nil {
		return err
	}
	candidate := &App{Dir: a.Dir, Cfg: &cfg}
	opt := candidate.fetchOptions()
	var body []byte
	var info *sub.Info
	var route string
	var err error
	if imported != nil {
		if len(imported) == 0 || len(imported) > sub.MaxBody {
			return fmt.Errorf("文件为空或超过 8 MiB")
		}
		cfg.SubscriptionURL = ""
		body = imported
		info = &sub.Info{FetchedAt: time.Now()}
		route = "本地文件"
	} else {
		body, info, route, err = sub.FetchWithOptions(ctx, rawURL, opt)
		if err != nil {
			return err
		}
	}
	body, err = sub.ExpandProviders(ctx, body, opt)
	if err != nil {
		return err
	}
	if err = candidate.prepare(body, info); err != nil {
		return err
	}
	candidate.DownloadRoute = route
	candidate.resourceRefresh = true
	if err = candidate.prepareRuleResources(ctx); err != nil {
		return err
	}
	if ctx.Err() != nil {
		return fmt.Errorf("操作已取消，原配置已保留")
	}
	// Activate first. A parse/port failure never writes the replacement URL to disk.
	if err = candidate.StartCore(); err != nil {
		a.rollbackCore()
		return err
	}
	rollback := func(problem string) error {
		if err := a.rollbackCore(); err != nil {
			return fmt.Errorf("%s；旧内核配置恢复失败，请重新启动并检查端口", problem)
		}
		return fmt.Errorf("%s", problem)
	}
	// Bind a legacy cache to the previously accepted source before creating a different record.
	if !sub.HasSourceCaches(a.Dir) && old.SubscriptionURL != "" && !old.Imported {
		if legacy, meta, ok := sub.LoadCache(a.Dir); ok {
			if err = sub.SaveSourceCache(a.Dir, old.SubscriptionURL, legacy, meta); err != nil {
				return rollback("旧订阅缓存迁移失败，请检查磁盘空间/权限")
			}
		}
	}
	previousBody, previousInfo, previousOK := sub.LoadSourceCache(a.Dir, candidate.sourceKey())
	if err = sub.SaveSourceCache(a.Dir, candidate.sourceKey(), body, info); err != nil {
		return rollback("新订阅缓存保存失败，原配置已保留，请检查磁盘空间/权限")
	}
	// Each source has its own atomically written cache. Persisting the URL last keeps
	// either the old or the new source recoverable even if the process stops here.
	if err = config.Save(a.Dir, &cfg); err != nil {
		var restoreErr error
		if previousOK {
			restoreErr = sub.SaveSourceCache(a.Dir, candidate.sourceKey(), previousBody, previousInfo)
		} else {
			restoreErr = sub.RemoveSourceCache(a.Dir, candidate.sourceKey())
		}
		if restoreErr != nil {
			return rollback("配置写入失败；缓存回滚也失败，请检查磁盘空间/权限后重新启动")
		}
		return rollback("配置保存失败，原配置已保留，请检查数据目录权限/磁盘空间")
	}
	a.adopt(candidate)
	return nil
}

// rollbackCore restores the previous runtime after an activation/persistence failure.
func (a *App) rollbackCore() error {
	a.mu.Lock()
	previous := a.cfgBytes
	a.mu.Unlock()
	if len(previous) > 0 {
		return core.Start(previous)
	} else {
		core.StopMixed()
	}
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
	a.SetStarting(a.HasSubscription())
	defer a.SetStarting(false)
	if !a.HasSubscription() {
		return nil
	}
	if err := a.Refresh(); err != nil {
		a.setLastErr(err.Error())
		return err
	}
	return nil
}
