package sub

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const fetchUA = "clash.meta/fly"

// Info 来自订阅响应头 subscription-userinfo。
type Info struct {
	Upload   int64     `json:"upload"`
	Download int64     `json:"download"`
	Total    int64     `json:"total"`
	Expire   int64     `json:"expire"`
	FetchedAt time.Time `json:"fetched_at"`
}

func (i *Info) Used() int64     { return i.Upload + i.Download }
func (i *Info) ExpireTime() time.Time {
	if i.Expire <= 0 {
		return time.Time{}
	}
	return time.Unix(i.Expire, 0)
}

// Fetch 拉取订阅，返回原始响应体与流量信息。订阅域名直连偶发被重置，带重试。
func Fetch(rawURL string) (body []byte, info *Info, err error) {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * 2 * time.Second)
		}
		body, info, err = fetchOnce(rawURL)
		if err == nil {
			return body, info, nil
		}
		lastErr = err
	}
	return nil, nil, lastErr
}

func fetchOnce(rawURL string) (body []byte, info *Info, err error) {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("User-Agent", fetchUA)
	client := &http.Client{Timeout: 45 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("订阅返回 %s", resp.Status)
	}
	body, err = io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, nil, err
	}
	info = parseUserInfoHeader(resp.Header.Get("Subscription-Userinfo"))
	info.FetchedAt = time.Now()
	return body, info, nil
}

func parseUserInfoHeader(h string) *Info {
	info := &Info{}
	for _, kv := range strings.Split(h, ";") {
		k, v, ok := strings.Cut(strings.TrimSpace(kv), "=")
		if !ok {
			continue
		}
		n, _ := strconv.ParseInt(v, 10, 64)
		switch k {
		case "upload":
			info.Upload = n
		case "download":
			info.Download = n
		case "total":
			info.Total = n
		case "expire":
			info.Expire = n
		}
	}
	return info
}

// BodyToLinks 把订阅响应体转成节点来源。兼容三种形态：
// base64 编码链接列表 / 明文链接列表 / Clash YAML（取其 proxies 与 rules）。
// yamlProxies 非 nil 时表示源是 Clash 配置，同时可能带 yamlRules。
func BodyToLinks(body []byte) (linksText string, yamlProxies []map[string]any, yamlRules []string) {
	t := strings.TrimSpace(string(body))
	t = strings.TrimLeft(t, "\xef\xbb\xbf")
	if t == "" {
		return "", nil, nil
	}
	if b, err := b64Decode(t); err == nil && strings.Contains(string(b), "://") {
		return string(b), nil, nil
	}
	if first := t[:min(200, len(t))]; strings.Contains(strings.SplitN(first, "\n", 2)[0], "://") {
		return t, nil, nil
	}
	if strings.Contains(t, "proxies:") {
		var cfg struct {
			Proxies []map[string]any `yaml:"proxies"`
			Rules   []string         `yaml:"rules"`
		}
		if err := yaml.Unmarshal([]byte(t), &cfg); err == nil && len(cfg.Proxies) > 0 {
			return "", cfg.Proxies, cfg.Rules
		}
	}
	return t, nil, nil
}

var builtinPolicies = map[string]bool{
	"DIRECT": true, "REJECT": true, "REJECT-DROP": true,
	"PASS": true, "COMPATIBLE": true, "GLOBAL": true,
}

// RewriteRules 采用订阅自带规则：把指向订阅分组的目标改写为主选择组，
// 跳过需要外部资源的 RULE-SET/SUB-RULE，并保证以 MATCH 结尾。
func RewriteRules(rules []string, target string) []string {
	out := make([]string, 0, len(rules)+1)
	hasMatch := false
	for _, r := range rules {
		parts := strings.Split(strings.TrimSpace(r), ",")
		if len(parts) == 0 || parts[0] == "" {
			continue
		}
		typ := strings.ToUpper(strings.TrimSpace(parts[0]))
		switch typ {
		case "RULE-SET", "SUB-RULE":
			continue
		case "MATCH":
			if len(parts) == 2 && !builtinPolicies[parts[1]] {
				parts[1] = target
			}
			out = append(out, strings.Join(parts, ","))
			hasMatch = true
			continue
		}
		if len(parts) < 3 {
			continue
		}
		last := len(parts) - 1
		if parts[last] == "no-resolve" || parts[last] == "src" {
			last--
		}
		if last > 0 && !builtinPolicies[parts[last]] {
			parts[last] = target
		}
		out = append(out, strings.Join(parts, ","))
	}
	if !hasMatch {
		out = append(out, "MATCH,"+target)
	}
	return out
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// SaveCache / LoadCache 订阅本地缓存，拉取失败时兜底。
func SaveCache(dir string, body []byte, info *Info) {
	_ = os.WriteFile(filepath.Join(dir, "sub-cache.txt"), body, 0o600)
	if info != nil {
		b, _ := yaml.Marshal(info)
		_ = os.WriteFile(filepath.Join(dir, "sub-meta.json"), b, 0o600)
	}
}

func LoadCache(dir string) (body []byte, info *Info, ok bool) {
	body, err := os.ReadFile(filepath.Join(dir, "sub-cache.txt"))
	if err != nil || len(body) == 0 {
		return nil, nil, false
	}
	info = &Info{}
	if b, err := os.ReadFile(filepath.Join(dir, "sub-meta.json")); err == nil {
		_ = yaml.Unmarshal(b, info)
	}
	return body, info, true
}
