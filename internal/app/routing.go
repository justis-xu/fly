package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fly/internal/config"
	"fly/internal/core"
	"fly/internal/storage"
	"fly/internal/sub"
	"fmt"
	P "github.com/metacubex/mihomo/constant/provider"
	RP "github.com/metacubex/mihomo/rules/provider"
	"gopkg.in/yaml.v3"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (a *App) SaveUserRules(input []config.UserRule) error {
	rules, err := config.NormalizeRules(input)
	if err != nil {
		return err
	}
	a.refreshMu.Lock()
	defer a.refreshMu.Unlock()
	a.mu.Lock()
	body := a.cacheBody
	previous := a.cfgBytes
	a.mu.Unlock()
	if len(previous) == 0 {
		return config.SaveRules(a.Dir, rules)
	}
	proxies, routing, _, err := parseBody(body)
	if err != nil {
		return err
	}
	cfg := a.ConfigSnapshot()
	next, err := core.BuildConfig(proxies, routing, core.Options{MixedPort: cfg.MixedPort, UserRules: config.RuleLines(rules, core.GroupProxy)})
	if err != nil {
		return err
	}
	next, err = bindRuleSnapshots(next, a.Info)
	if err != nil {
		return err
	}
	if err = core.Start(next); err != nil {
		_ = core.Start(previous)
		return fmt.Errorf("规则未启用：%s", sub.Redact(err.Error()))
	}
	if err = config.SaveRules(a.Dir, rules); err != nil {
		_ = core.Start(previous)
		return fmt.Errorf("规则保存失败，已恢复原配置，请检查数据目录权限/空间")
	}
	a.mu.Lock()
	a.cfgBytes = next
	a.generation++
	a.mu.Unlock()
	return nil
}

type ProviderSource struct {
	Type     string   `yaml:"type"`
	Behavior string   `yaml:"behavior"`
	Format   string   `yaml:"format"`
	Path     string   `yaml:"path"`
	Payload  []string `yaml:"payload"`
}

func (a *App) RoutingSource() (rules []string, providers map[string]ProviderSource, err error) {
	a.mu.Lock()
	data := a.cfgBytes
	a.mu.Unlock()
	var cfg struct {
		Rules     []string                  `yaml:"rules"`
		Providers map[string]ProviderSource `yaml:"rule-providers"`
	}
	err = yaml.Unmarshal(data, &cfg)
	return cfg.Rules, cfg.Providers, err
}

type rulePage struct {
	Items  []string `json:"items"`
	Total  int      `json:"total"`
	Offset int      `json:"offset"`
	More   bool     `json:"more"`
}

// A bounded writer lets MRS conversion stop without allocating a giant text dump.
type pageWriter struct {
	query                string
	offset, limit, total int
	items                []string
	pending              string
}

func (p *pageWriter) line(line string) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
		return
	}
	if !strings.Contains(strings.ToLower(line), p.query) {
		return
	}
	if p.total >= p.offset && len(p.items) < p.limit {
		p.items = append(p.items, line)
	}
	p.total++
}
func (p *pageWriter) Write(b []byte) (int, error) {
	s := p.pending + string(b)
	for {
		line, rest, ok := strings.Cut(s, "\n")
		if !ok {
			p.pending = s
			break
		}
		p.line(line)
		s = rest
	}
	if len(p.pending) > 65536 {
		return 0, fmt.Errorf("规则条目过长")
	}
	return len(b), nil
}
func (a *App) ProviderRules(name, query string, offset, limit int) (any, error) {
	_, providers, err := a.RoutingSource()
	if err != nil {
		return nil, err
	}
	source, ok := providers[name]
	if !ok {
		return nil, fmt.Errorf("规则集不存在")
	}
	if offset < 0 || offset > 10000000 || limit < 1 || limit > 200 || len(query) > 512 {
		return nil, fmt.Errorf("查询参数无效")
	}
	page := &pageWriter{query: strings.ToLower(query), offset: offset, limit: limit, items: []string{}}
	if source.Type == "inline" {
		for _, line := range source.Payload {
			page.line(line)
		}
	} else {
		// The path is emitted by BuildConfig, not accepted from an HTTP request or remote path.
		path := filepath.Join(a.Dir, source.Path)
		f, err := os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("规则资源尚未下载，请更新订阅或检查下载网络")
		}
		defer f.Close()
		stat, err := f.Stat()
		if err != nil || stat.Size() > 32<<20 {
			return nil, fmt.Errorf("规则资源无法读取或超过 32 MiB")
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("规则缓存读取失败")
		}
		switch source.Format {
		case "mrs":
			behavior, err := P.ParseBehavior(source.Behavior)
			if err != nil {
				return nil, fmt.Errorf("规则集类型无效")
			}
			if err = RP.ConvertToMrs(content, behavior, P.MrsRule, page); err != nil {
				return nil, fmt.Errorf("规则缓存未就绪或格式无效，请更新后重试")
			}
		case "text":
			_, err = page.Write(content)
			if err != nil {
				return nil, err
			}
			if page.pending != "" {
				page.line(page.pending)
			}
		default:
			var parsed struct {
				Payload []string `yaml:"payload"`
				Rules   []string `yaml:"rules"`
			}
			if yaml.Unmarshal(content, &parsed) != nil {
				return nil, fmt.Errorf("规则 YAML 无效，请更新后重试")
			}
			for _, line := range append(parsed.Payload, parsed.Rules...) {
				page.line(line)
			}
		}
	}
	return rulePage{Items: page.items, Total: page.total, Offset: offset, More: page.total > offset+len(page.items)}, nil
}

// bindRuleSnapshots accepts only generated, identity-bound content-addressed paths.
// Rebuilding user rules must retain the exact resources of the accepted subscription.
func bindRuleSnapshots(data []byte, info *sub.Info) ([]byte, error) {
	var cfg map[string]any
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	providers, _ := cfg["rule-providers"].(map[string]any)
	for name, value := range providers {
		provider, _ := value.(map[string]any)
		if provider["type"] != "http" {
			continue
		}
		identity := core.RuleCachePath(name, provider)
		if info != nil && validSnapshotPath(identity, info.RuleFiles[identity]) {
			provider["path"] = info.RuleFiles[identity]
		}
	}
	return yaml.Marshal(cfg)
}
func validSnapshotPath(identity, path string) bool {
	suffix, ok := strings.CutPrefix(path, identity+"-")
	if !ok || len(suffix) != 64 {
		return false
	}
	_, err := hex.DecodeString(suffix)
	return err == nil
}

// Rule files are immutable versions. The source-cache transaction commits their
// paths only after activation, so failure or interruption cannot overwrite old rules.
func (a *App) prepareRuleResources(ctx context.Context) error {
	var cfg map[string]any
	if err := yaml.Unmarshal(a.cfgBytes, &cfg); err != nil {
		return err
	}
	providers, _ := cfg["rule-providers"].(map[string]any)
	snapshots := map[string]string{}
	known := a.Info
	if known == nil || len(known.RuleFiles) == 0 {
		_, cached, ok := sub.LoadSourceCache(a.Dir, a.sourceKey())
		if ok {
			known = cached
		}
	}
	for name, value := range providers {
		provider, _ := value.(map[string]any)
		if provider["type"] != "http" {
			continue
		}
		identity := core.RuleCachePath(name, provider)
		behavior, _ := provider["behavior"].(string)
		format, _ := provider["format"].(string)
		headers, err := sub.ProviderHeaders(provider["header"])
		if err != nil {
			return err
		}
		cachedPath := ""
		if known != nil && validSnapshotPath(identity, known.RuleFiles[identity]) {
			cachedPath = known.RuleFiles[identity]
		}
		// Older accepted caches stored one file per identity. Only migrate them
		// when loading that accepted cache, never during a new online transaction.
		if cachedPath == "" && !a.resourceRefresh {
			cachedPath = identity
		}
		var cached []byte
		if cachedPath != "" {
			cached, _ = os.ReadFile(filepath.Join(a.Dir, cachedPath))
			if core.ValidateRuleResource(cached, behavior, format) != nil {
				cached = nil
			}
			if validSnapshotPath(identity, cachedPath) && fmt.Sprintf("%s-%x", identity, sha256.Sum256(cached)) != cachedPath {
				cached = nil
			}
		}
		body := cached
		if a.resourceRefresh || len(cached) == 0 {
			downloadCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
			opt := a.fetchOptions()
			opt.Headers = headers
			rawURL, _ := provider["url"].(string)
			downloaded, _, _, downloadErr := sub.FetchWithOptions(downloadCtx, rawURL, opt)
			cancel()
			if ctx.Err() != nil {
				return fmt.Errorf("规则下载已取消或超时，原配置已保留")
			}
			if downloadErr == nil {
				downloadErr = core.ValidateRuleResource(downloaded, behavior, format)
			}
			if downloadErr != nil {
				if len(cached) == 0 {
					return fmt.Errorf("规则集首次下载或格式验证失败：%s；请设置备用下载代理或使用含内联规则的订阅", sub.Redact(downloadErr.Error()))
				}
				a.setLastErr("规则资源更新失败，已保留该规则集上次可用的缓存：" + sub.Redact(downloadErr.Error()))
			} else {
				body = downloaded
			}
		}
		path := fmt.Sprintf("%s-%x", identity, sha256.Sum256(body))
		absolute := filepath.Join(a.Dir, path)
		if err := os.MkdirAll(filepath.Dir(absolute), 0700); err != nil {
			return fmt.Errorf("规则缓存目录无法创建")
		}
		// Reusing exactly the same immutable bytes does not require a write.
		existing, readErr := os.ReadFile(absolute)
		if readErr != nil || !bytes.Equal(existing, body) {
			if err := storage.WriteFile(absolute, body); err != nil {
				return fmt.Errorf("规则缓存写入失败，请检查磁盘空间/权限")
			}
		}
		provider["path"] = path
		snapshots[identity] = path
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	if a.Info == nil {
		a.Info = &sub.Info{}
	}
	a.Info.RuleFiles = snapshots
	a.cfgBytes = data
	a.resourcesPrepared = true
	return nil
}
