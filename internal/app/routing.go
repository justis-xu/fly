package app

import (
	"bytes"
	"context"
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

// Fetch rule resources with the same explicit download policy before asking mihomo to load them.
// A matching on-disk cache permits offline restart; a new broken source cannot replace live routing.
func (a *App) prepareRuleResources(ctx context.Context) error {
	_, providers, err := a.RoutingSource()
	if err != nil {
		return err
	}
	for _, source := range providers {
		if source.Type != "http" {
			continue
		}
		// Read the generated URL privately; RoutingSource intentionally doesn't expose it to the GUI.
		var cfg struct {
			Providers map[string]map[string]any `yaml:"rule-providers"`
		}
		_ = yaml.Unmarshal(a.cfgBytes, &cfg)
		var provider map[string]any
		for _, candidate := range cfg.Providers {
			if candidate["path"] == source.Path {
				provider = candidate
				break
			}
		}
		rawURL, _ := provider["url"].(string)
		path := filepath.Join(a.Dir, source.Path)
		cachedValid := false
		if existing, err := os.ReadFile(path); err == nil && len(bytes.TrimSpace(existing)) > 0 {
			cachedValid = core.ValidateRuleResource(existing, source.Behavior, source.Format) == nil
		}
		if cachedValid && !a.resourceRefresh {
			continue
		}
		downloadCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		body, _, _, err := sub.FetchWithOptions(downloadCtx, rawURL, a.fetchOptions())
		cancel()
		if err != nil {
			if ctx.Err() != nil {
				return fmt.Errorf("规则下载已取消或超时，原配置已保留")
			}
			if cachedValid {
				a.setLastErr("规则资源更新失败，已保留该规则集上次可用的缓存：" + err.Error())
				continue
			}
			return fmt.Errorf("规则集首次下载失败：%w；请设置备用下载代理或使用含内联规则的订阅", err)
		}
		if err = core.ValidateRuleResource(body, source.Behavior, source.Format); err != nil {
			if cachedValid {
				a.setLastErr("规则资源格式无效，继续使用该规则集上次可用的缓存")
				continue
			}
			return fmt.Errorf("规则资源格式无效：%s", sub.Redact(err.Error()))
		}
		if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return fmt.Errorf("规则缓存目录无法创建")
		}
		if err = storage.WriteFile(path, body); err != nil {
			return fmt.Errorf("规则缓存写入失败，请检查磁盘空间/权限")
		}
	}
	a.resourcesPrepared = true
	return nil
}
