package core

import (
	"crypto/sha256"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/hub"
	"github.com/metacubex/mihomo/listener"
	"github.com/metacubex/mihomo/tunnel"
	"gopkg.in/yaml.v3"

	"fly/internal/assets"
)

const (
	GroupProxy = "🚀 Proxy"
	GroupAuto  = "♻️ 自动选择"
)

// Init 设置 mihomo 数据目录（必须在一切解析之前调用），并预置 geo 数据。
func Init(homeDir string) error {
	if err := os.MkdirAll(homeDir, 0o700); err != nil {
		return err
	}
	constant.SetHomeDir(homeDir)
	return extractGeoFiles(homeDir)
}

// Options 生成 mihomo 配置所需的参数。
type Options struct {
	MixedPort int // 本地混合代理端口
	UserRules []string
}

type Routing struct {
	Rules     []string
	Providers map[string]map[string]any
	SubRules  map[string][]string
}

// BuildConfig 生成完整的 mihomo 配置，订阅无规则时使用内置分流。
func BuildConfig(proxies []map[string]any, routing Routing, opt Options) ([]byte, error) {
	if len(proxies) == 0 {
		return nil, fmt.Errorf("没有可用节点")
	}
	rules := routing.Rules
	if len(rules) == 0 {
		rules = []string{
			"GEOSITE,cn,DIRECT",
			"GEOIP,LAN,DIRECT,no-resolve",
			"GEOIP,CN,DIRECT",
			"MATCH," + GroupProxy,
		}
	}
	rules = append(append([]string{}, opt.UserRules...), rules...)
	names := make([]string, 0, len(proxies))
	for _, p := range proxies {
		names = append(names, p["name"].(string))
	}
	cfg := map[string]any{
		"mixed-port":     opt.MixedPort,
		"allow-lan":      false,
		"bind-address":   "127.0.0.1",
		"mode":           "rule",
		"log-level":      "warning",
		"unified-delay":  true,
		"tcp-concurrent": true,
		"profile":        map[string]any{"store-selected": true},
		"proxies":        proxies,
		"proxy-groups": []map[string]any{
			{
				"name":    GroupProxy,
				"type":    "select",
				"proxies": append([]string{GroupAuto, "DIRECT"}, names...),
			},
			{
				"name":            GroupAuto,
				"type":            "url-test",
				"url":             "https://www.gstatic.com/generate_204",
				"interval":        300,
				"timeout":         5000,
				"expected-status": "204",
				"lazy":            false,
				"tolerance":       50,
				"proxies":         names,
			},
		},
		"rules": rules,
	}
	if len(routing.Providers) > 0 {
		providers := map[string]map[string]any{}
		for name, source := range routing.Providers {
			provider := map[string]any{}
			for k, v := range source {
				provider[k] = v
			}
			switch provider["type"] {
			case "http":
				// Bootstrap rule downloads without depending on the selected node.
				provider["proxy"] = "DIRECT"
				// fly refreshes rule resources transactionally with the subscription.
				provider["interval"] = 0
				provider["path"] = RuleCachePath(name, provider)
			case "inline":
			default:
				return nil, fmt.Errorf("规则集 %s 的类型不受支持", name)
			}
			if proxy, ok := provider["proxy"].(string); ok && proxy != "DIRECT" {
				provider["proxy"] = GroupProxy
			}
			providers[name] = provider
		}
		cfg["rule-providers"] = providers
	}
	if len(routing.SubRules) > 0 {
		cfg["sub-rules"] = routing.SubRules
	}
	return yaml.Marshal(cfg)
}

// Start 启动或热重载内核，不开启独立控制端口。
func Start(cfgBytes []byte) error {
	Runtime.Lock()
	defer Runtime.Unlock()
	var expected struct {
		Port int `yaml:"mixed-port"`
	}
	if err := yaml.Unmarshal(cfgBytes, &expected); err != nil {
		return err
	}
	if err := hub.Parse(cfgBytes); err != nil {
		return err
	}
	if expected.Port > 0 && !ready(expected.Port) {
		return fmt.Errorf("代理端口 %d 监听失败，请检查端口占用", expected.Port)
	}
	return nil
}

func Ready(port int) bool {
	Runtime.RLock()
	defer Runtime.RUnlock()
	return ready(port)
}
func ready(port int) bool {
	if port <= 0 || tunnel.Status() != tunnel.Running || listener.GetPorts().MixedPort != port {
		return false
	}
	// mihomo may retain the listener object after closing TCP on a UDP bind failure.
	conn, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", fmt.Sprint(port)), 500*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func extractGeoFiles(homeDir string) error {
	for name, data := range map[string][]byte{
		"geoip.metadb": assets.GeoMetadb,
		"GeoSite.dat":  assets.GeoSiteDat,
		"ASN.mmdb":     assets.ASNMMDB,
	} {
		if data == nil {
			continue
		}
		p := filepath.Join(homeDir, name)
		if st, err := os.Stat(p); err == nil && st.Size() == int64(len(data)) {
			continue
		}
		if err := os.WriteFile(p, data, 0o644); err != nil {
			return fmt.Errorf("释放 %s: %w", name, err)
		}
	}
	return nil
}

func RuleCachePath(name string, provider map[string]any) string {
	return fmt.Sprintf("./rules/%x", sha256.Sum256([]byte(fmt.Sprint(name, "\x00", provider["url"], "\x00", provider["behavior"], "\x00", provider["format"]))))
}
