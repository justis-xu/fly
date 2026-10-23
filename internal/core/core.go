package core

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/hub"
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
	MixedPort      int    // 本地混合代理端口
	ControllerAddr string // REST 控制器监听地址
	Secret         string // 控制器鉴权
}

// BuildConfig 生成完整的 mihomo 配置。rules 为空时用内置默认规则。
func BuildConfig(proxies []map[string]any, rules []string, opt Options) ([]byte, error) {
	if len(proxies) == 0 {
		return nil, fmt.Errorf("没有可用节点")
	}
	if len(rules) == 0 {
		rules = []string{
			"GEOSITE,cn,DIRECT",
			"GEOIP,LAN,DIRECT,no-resolve",
			"GEOIP,CN,DIRECT",
			"MATCH," + GroupProxy,
		}
	}
	names := make([]string, 0, len(proxies))
	for _, p := range proxies {
		names = append(names, p["name"].(string))
	}
	cfg := map[string]any{
		"mixed-port":          opt.MixedPort,
		"allow-lan":           false,
		"bind-address":        "127.0.0.1",
		"mode":                "rule",
		"log-level":           "warning",
		"unified-delay":       true,
		"tcp-concurrent":      true,
		"external-controller": opt.ControllerAddr,
		"secret":              opt.Secret,
		"profile":             map[string]any{"store-selected": true},
		"proxies":             proxies,
		"proxy-groups": []map[string]any{
			{
				"name":    GroupProxy,
				"type":    "select",
				"proxies": append([]string{GroupAuto, "DIRECT"}, names...),
			},
			{
				"name":       GroupAuto,
				"type":       "url-test",
				"url":        "https://www.gstatic.com/generate_204",
				"interval":   300,
				"tolerance":  50,
				"proxies":    names,
			},
		},
		"rules":               rules,
	}
	return yaml.Marshal(cfg)
}

// Start 启动内核（也是热重载入口：重新 Parse 等价于全量重载，控制器地址不变则无缝）。
func Start(cfgBytes []byte, controllerAddr, secret string) error {
	return hub.Parse(cfgBytes,
		hub.WithExternalController(controllerAddr),
		hub.WithSecret(secret),
	)
}

func extractGeoFiles(homeDir string) error {
	for name, data := range map[string][]byte{
		"geoip.metadb": assets.GeoMetadb,
		"GeoSite.dat":  assets.GeoSiteDat,
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
