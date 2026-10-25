package sub

import (
	"context"
	"fmt"
	"gopkg.in/yaml.v3"
	"regexp"
	"sort"
	"strings"
)

// ExpandProviders freezes remote node providers into the accepted subscription cache.
// Offline restarts therefore don't require a second successful provider download.
func ExpandProviders(ctx context.Context, body []byte, opt FetchOptions) ([]byte, error) {
	var source map[string]any
	if yaml.Unmarshal(body, &source) != nil || source == nil {
		return body, nil
	}
	raw, ok := source["proxy-providers"].(map[string]any)
	if !ok || len(raw) == 0 {
		return body, nil
	}
	if len(raw) > 32 {
		return nil, fmt.Errorf("节点来源超过 32 个，请精简订阅")
	}
	var proxies []map[string]any
	if p, ok := source["proxies"]; ok {
		b, _ := yaml.Marshal(p)
		if yaml.Unmarshal(b, &proxies) != nil {
			return nil, fmt.Errorf("proxies 格式错误")
		}
	}
	keys := make([]string, 0, len(raw))
	for k := range raw {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, name := range keys {
		b, _ := yaml.Marshal(raw[name])
		var p struct {
			Header   any              `yaml:"header"`
			Type     string           `yaml:"type"`
			URL      string           `yaml:"url"`
			Payload  []map[string]any `yaml:"payload"`
			Filter   string           `yaml:"filter"`
			Exclude  string           `yaml:"exclude-filter"`
			Override map[string]any   `yaml:"override"`
		}
		if yaml.Unmarshal(b, &p) != nil {
			return nil, fmt.Errorf("节点来源配置格式错误")
		}
		var list []map[string]any
		switch p.Type {
		case "inline":
			list = p.Payload
		case "http":
			providerOptions := opt
			headers, err := ProviderHeaders(p.Header)
			if err != nil {
				return nil, err
			}
			providerOptions.Headers = headers
			content, _, _, err := FetchWithOptions(ctx, p.URL, providerOptions)
			if err != nil {
				return nil, fmt.Errorf("节点来源下载失败：%w", err)
			}
			var nested struct {
				Proxies   []map[string]any `yaml:"proxies"`
				Providers map[string]any   `yaml:"proxy-providers"`
			}
			if yaml.Unmarshal(content, &nested) != nil || len(nested.Proxies) == 0 || len(nested.Providers) > 0 {
				return nil, fmt.Errorf("节点来源须返回含 proxies 的 Clash YAML，不支持递归来源")
			}
			list = nested.Proxies
		default:
			return nil, fmt.Errorf("节点来源只支持 http 或 inline；file 来源请先合并为 Clash YAML")
		}
		var include, exclude *regexp.Regexp
		if p.Filter != "" {
			var err error
			include, err = regexp.Compile(p.Filter)
			if err != nil {
				return nil, fmt.Errorf("节点来源 filter 表达式无效")
			}
		}
		if p.Exclude != "" {
			var err error
			exclude, err = regexp.Compile(p.Exclude)
			if err != nil {
				return nil, fmt.Errorf("节点来源 exclude-filter 表达式无效")
			}
		}
		for _, node := range list {
			n, _ := node["name"].(string)
			if include != nil && !include.MatchString(n) || exclude != nil && exclude.MatchString(n) {
				continue
			}
			for key, value := range p.Override {
				switch key {
				case "additional-prefix":
					n = fmt.Sprint(value) + n
				case "additional-suffix":
					n += fmt.Sprint(value)
				case "tfo", "mptcp", "udp", "skip-cert-verify":
					node[key] = value
				default:
					return nil, fmt.Errorf("节点来源含不支持的 override 字段，请改用展开后的 Clash YAML")
				}
			}
			node["name"] = n
			proxies = append(proxies, node)
			if len(proxies) > 2000 {
				return nil, fmt.Errorf("最多支持 2000 个节点，请精简订阅")
			}
		}
	}
	source["proxies"] = proxies
	delete(source, "proxy-providers")
	result, err := yaml.Marshal(source)
	if err != nil {
		return nil, err
	}
	if len(result) > MaxBody {
		return nil, fmt.Errorf("合并后订阅超过 8 MiB")
	}
	return result, nil
}

var secretURL = regexp.MustCompile(`(?i)[a-z][a-z0-9+.-]*://[^\s"'<>]+`)

func Redact(message string) string {
	return secretURL.ReplaceAllString(strings.TrimSpace(message), "[资源地址]")
}
