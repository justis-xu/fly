package sub

import (
	"bytes"
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/http/httpguts"
	"gopkg.in/yaml.v3"
)

const fetchUA = "clash.meta/fly"

// Info 来自订阅响应头 subscription-userinfo。
type Info struct {
	Upload      int64     `json:"upload"`
	Download    int64     `json:"download"`
	Total       int64     `json:"total"`
	Expire      int64     `json:"expire"`
	FetchedAt   time.Time `json:"fetched_at"`
	LegacyCache bool      `json:"legacy_cache,omitempty"`
}

func (i *Info) Used() int64 { return i.Upload + i.Download }
func (i *Info) ExpireTime() time.Time {
	if i.Expire <= 0 {
		return time.Time{}
	}
	return time.Unix(i.Expire, 0)
}

// Fetch downloads directly; it never borrows HTTP_PROXY or a global transport.
func Fetch(rawURL string) ([]byte, *Info, error) {
	b, i, _, err := FetchWithOptions(context.Background(), rawURL, FetchOptions{})
	return b, i, err
}

const MaxBody = 8 << 20

type FetchOptions struct {
	Headers      http.Header
	ProxyURL     string
	FallbackDial func(context.Context, string, string) (net.Conn, error)
}

func ValidateURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return fmt.Errorf("订阅地址须是完整的 http:// 或 https:// 链接")
	}
	return nil
}
func ValidateProxy(raw string) error {
	if raw == "" {
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https" && u.Scheme != "socks5" && u.Scheme != "socks5h") {
		return fmt.Errorf("备用下载代理须是 http(s):// 或 socks5:// 地址（含端口）")
	}
	if _, err := strconv.Atoi(u.Port()); err != nil {
		return fmt.Errorf("备用下载代理需要有效端口")
	}
	n, _ := strconv.Atoi(u.Port())
	if n < 1 || n > 65535 {
		return fmt.Errorf("备用下载代理端口须在 1–65535 之间")
	}
	return nil
}

type responseError struct{ code int }

func (e responseError) Error() string {
	switch e.code {
	case 401, 403:
		return fmt.Sprintf("下载返回 HTTP %d：可能是订阅过期、凭据无效或访问被限制，请向服务商确认", e.code)
	case 404:
		return "下载返回 HTTP 404：地址不存在，请复制完整订阅链接"
	case 413:
		return "下载内容超过 8 MiB，请使用精简后的订阅"
	case 204:
		return "订阅内容为空，请检查服务商提供的链接"
	case 429:
		return "下载返回 HTTP 429：请求过于频繁，请稍后再试"
	default:
		return fmt.Sprintf("下载返回 HTTP %d，请检查服务商状态", e.code)
	}
}
func safeDownloadError(err error) string {
	var status responseError
	if errors.As(err, &status) {
		return status.Error()
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "连接超时，请检查网络或尝试备用下载代理/本地文件"
	}
	var dns *net.DNSError
	if errors.As(err, &dns) {
		return "域名解析失败，请检查订阅域名和本机 DNS"
	}
	var cert x509.UnknownAuthorityError
	if errors.As(err, &cert) {
		return "证书无法验证，请检查系统时间和证书环境"
	}
	// Never include URLs, userinfo, node passwords or token-bearing paths in errors.
	return "连接失败，请检查网络、服务地址或备用下载代理"
}
func FetchWithOptions(ctx context.Context, rawURL string, opt FetchOptions) ([]byte, *Info, string, error) {
	if err := ValidateURL(rawURL); err != nil {
		return nil, nil, "", err
	}
	if err := ValidateHeaders(opt.Headers); err != nil {
		return nil, nil, "", err
	}
	if err := ValidateProxy(opt.ProxyURL); err != nil {
		return nil, nil, "", err
	}
	ctx, cancel := context.WithTimeout(ctx, 50*time.Second)
	defer cancel()
	type route struct {
		name  string
		proxy string
		dial  func(context.Context, string, string) (net.Conn, error)
	}
	routes := []route{{name: "直连"}}
	if opt.ProxyURL != "" {
		routes = append(routes, route{name: "备用代理", proxy: opt.ProxyURL})
	}
	if opt.FallbackDial != nil {
		routes = append(routes, route{name: "当前节点", dial: opt.FallbackDial})
	}
	failures := []string{}
	for _, r := range routes {
		if ctx.Err() != nil {
			break
		}
		tr := &http.Transport{Proxy: nil, DialContext: (&net.Dialer{Timeout: 6 * time.Second}).DialContext, TLSHandshakeTimeout: 6 * time.Second, ResponseHeaderTimeout: 8 * time.Second, DisableKeepAlives: true}
		if r.proxy != "" {
			u, _ := url.Parse(r.proxy)
			tr.Proxy = http.ProxyURL(u)
		}
		if r.dial != nil {
			tr.DialContext = r.dial
		}
		client := &http.Client{Transport: tr, Timeout: 12 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) > 0 && req.URL.Host != via[0].URL.Host {
				for key := range opt.Headers {
					if !strings.EqualFold(key, "User-Agent") && !strings.EqualFold(key, "Accept") {
						req.Header.Del(key)
					}
				}
			}
			if len(via) >= 5 {
				return fmt.Errorf("重定向过多")
			}
			return nil
		}}
		b, i, err := fetchClient(ctx, client, rawURL, opt.Headers)
		tr.CloseIdleConnections()
		if err == nil {
			return b, i, r.name, nil
		}
		failures = append(failures, r.name+"："+safeDownloadError(err))
		var status responseError
		if errors.As(err, &status) && (status.code == 401 || status.code == 404) {
			break
		}
	}
	return nil, nil, "", fmt.Errorf("%s", strings.Join(failures, "；"))
}
func fetchClient(ctx context.Context, client *http.Client, rawURL string, headers http.Header) ([]byte, *Info, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("User-Agent", fetchUA)
	for k, values := range headers {
		req.Header.Del(k)
		for _, value := range values {
			req.Header.Add(k, value)
		}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, nil, responseError{resp.StatusCode}
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, MaxBody+1))
	if err != nil {
		return nil, nil, err
	}
	if len(b) > MaxBody {
		return nil, nil, responseError{413}
	}
	if len(bytes.TrimSpace(b)) == 0 {
		return nil, nil, responseError{204}
	}
	info := parseUserInfoHeader(resp.Header.Get("Subscription-Userinfo"))
	info.FetchedAt = time.Now()
	return b, info, nil
}

func parseUserInfoHeader(h string) *Info {
	info := &Info{}
	for _, kv := range strings.Split(h, ";") {
		k, v, ok := strings.Cut(strings.TrimSpace(kv), "=")
		if !ok {
			continue
		}
		n, _ := strconv.ParseInt(v, 10, 64)
		if n < 0 {
			n = 0
		}
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
	t = strings.TrimPrefix(t, "\xef\xbb\xbf")
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
	"PASS": true, "PASS-RULE": true, "COMPATIBLE": true,
}

// RewriteRules 采用订阅自带规则：把指向订阅分组的目标改写为主选择组，
// 保留 RULE-SET/SUB-RULE，并保证以 MATCH 结尾。
func RewriteRules(rules []string, target string) []string {
	return rewriteRules(rules, target, true)
}

func RewriteSubRules(rules []string, target string) []string {
	return rewriteRules(rules, target, false)
}

func rewriteRules(rules []string, target string, catchAll bool) []string {
	out := make([]string, 0, len(rules)+1)
	hasMatch := false
	for _, r := range rules {
		parts := strings.Split(strings.TrimSpace(r), ",")
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
		}
		if len(parts) == 0 || parts[0] == "" {
			continue
		}
		typ := strings.ToUpper(strings.TrimSpace(parts[0]))
		switch typ {
		case "SUB-RULE":
			out = append(out, strings.Join(parts, ","))
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
		for last > 0 && (parts[last] == "no-resolve" || parts[last] == "src") {
			last--
		}
		if last > 0 && !builtinPolicies[parts[last]] {
			parts[last] = target
		}
		out = append(out, strings.Join(parts, ","))
	}
	if catchAll && !hasMatch {
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
	info.LegacyCache = true
	return body, info, true
}

func ValidateHeaders(headers http.Header) error {
	if len(headers) > 32 {
		return fmt.Errorf("下载请求头过多")
	}
	size := 0
	for key, values := range headers {
		if !httpguts.ValidHeaderFieldName(key) {
			return fmt.Errorf("下载请求头名称无效")
		}
		switch strings.ToLower(key) {
		case "host", "connection", "proxy-authorization", "transfer-encoding", "content-length":
			return fmt.Errorf("订阅来源包含不支持的下载请求头")
		}
		for _, value := range values {
			size += len(key) + len(value)
			if !httpguts.ValidHeaderFieldValue(value) || size > 32768 {
				return fmt.Errorf("下载请求头无效或过长")
			}
		}
	}
	return nil
}
func ProviderHeaders(value any) (http.Header, error) {
	if value == nil {
		return nil, nil
	}
	data, err := yaml.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("下载请求头格式无效")
	}
	var raw map[string]any
	if yaml.Unmarshal(data, &raw) != nil {
		return nil, fmt.Errorf("下载请求头格式无效")
	}
	headers := http.Header{}
	for key, value := range raw {
		switch v := value.(type) {
		case string:
			headers.Set(key, v)
		case []any:
			for _, item := range v {
				s, ok := item.(string)
				if !ok {
					return nil, fmt.Errorf("下载请求头须使用文本")
				}
				headers.Add(key, s)
			}
		default:
			return nil, fmt.Errorf("下载请求头须使用文本列表")
		}
	}
	return headers, ValidateHeaders(headers)
}
