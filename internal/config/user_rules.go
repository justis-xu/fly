package config

import (
	"fly/internal/storage"
	"fmt"
	"golang.org/x/net/idna"
	"gopkg.in/yaml.v3"
	"net"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

type UserRule struct {
	Match  string `json:"match" yaml:"match"`
	Value  string `json:"value" yaml:"value"`
	Action string `json:"action" yaml:"action"`
}

func NormalizeRules(input []UserRule) ([]UserRule, error) {
	if len(input) > 200 {
		return nil, fmt.Errorf("自定义规则最多 200 条")
	}
	out := make([]UserRule, 0, len(input))
	seen := map[string]bool{}
	for i, r := range input {
		r.Value = strings.TrimSpace(r.Value)
		if r.Action != "proxy" && r.Action != "direct" && r.Action != "reject" {
			return nil, fmt.Errorf("第 %d 条规则：请选择代理、直连或拒绝", i+1)
		}
		switch r.Match {
		case "domain", "domain_suffix":
			if strings.Contains(r.Value, "://") {
				u, err := url.Parse(r.Value)
				if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") {
					return nil, fmt.Errorf("第 %d 条规则：网址无效", i+1)
				}
				r.Value = u.Hostname()
			}
			r.Value = strings.TrimSuffix(strings.ToLower(r.Value), ".")
			if r.Match == "domain_suffix" {
				r.Value = strings.TrimPrefix(strings.TrimPrefix(r.Value, "*."), ".")
			}
			domain, err := idna.Lookup.ToASCII(r.Value)
			if err != nil || domain == "" || len(domain) > 253 || net.ParseIP(domain) != nil || strings.ContainsAny(domain, "/:, *()?#@\\") || strings.IndexFunc(domain, unicode.IsSpace) >= 0 {
				return nil, fmt.Errorf("第 %d 条规则：请输入域名，例如 example.org；IP 请选 IP 网段", i+1)
			}
			for _, part := range strings.Split(domain, ".") {
				if len(part) == 0 || len(part) > 63 || part[0] == '-' || part[len(part)-1] == '-' {
					return nil, fmt.Errorf("第 %d 条规则：域名格式无效", i+1)
				}
			}
			r.Value = domain
		case "ip_cidr":
			prefix, err := netip.ParsePrefix(r.Value)
			if err != nil {
				return nil, fmt.Errorf("第 %d 条规则：请输入 IP 网段，例如 192.168.1.0/24", i+1)
			}
			r.Value = prefix.Masked().String()
		default:
			return nil, fmt.Errorf("第 %d 条规则：请选择域名、域名及子域名或 IP 网段", i+1)
		}
		key := r.Match + "\x00" + r.Value
		if seen[key] {
			return nil, fmt.Errorf("第 %d 条规则重复，请修改原有规则", i+1)
		}
		seen[key] = true
		out = append(out, r)
	}
	return out, nil
}
func LoadRules(dir string) ([]UserRule, error) {
	b, err := os.ReadFile(filepath.Join(dir, "rules.yaml"))
	if os.IsNotExist(err) {
		return []UserRule{}, nil
	}
	if err != nil {
		return nil, err
	}
	var rules []UserRule
	if err = yaml.Unmarshal(b, &rules); err != nil {
		return nil, fmt.Errorf("自定义规则文件无效，请检查 rules.yaml")
	}
	return NormalizeRules(rules)
}
func SaveRules(dir string, rules []UserRule) error {
	normalized, err := NormalizeRules(rules)
	if err != nil {
		return err
	}
	b, err := yaml.Marshal(normalized)
	if err != nil {
		return err
	}
	return storage.WriteFile(filepath.Join(dir, "rules.yaml"), b)
}
func RuleLines(rules []UserRule, target string) []string {
	out := make([]string, 0, len(rules))
	for _, r := range rules {
		typ := map[string]string{"domain": "DOMAIN", "domain_suffix": "DOMAIN-SUFFIX", "ip_cidr": "IP-CIDR"}[r.Match]
		action := map[string]string{"direct": "DIRECT", "reject": "REJECT", "proxy": target}[r.Action]
		line := typ + "," + r.Value + "," + action
		if r.Match == "ip_cidr" {
			line += ",no-resolve"
		}
		out = append(out, line)
	}
	return out
}

// MatchUserRule explains only user overrides. Subscription rules can depend on DNS and rule sets.
func MatchUserRule(rules []UserRule, input string) (int, error) {
	value := strings.TrimSpace(input)
	if strings.Contains(value, "://") {
		u, err := url.Parse(value)
		if err != nil || u.Hostname() == "" {
			return -1, fmt.Errorf("请输入域名、IP 或完整网址")
		}
		value = u.Hostname()
	}
	value = strings.TrimSuffix(strings.ToLower(value), ".")
	ip, ipErr := netip.ParseAddr(value)
	if ipErr != nil {
		normalized, err := NormalizeRules([]UserRule{{Match: "domain", Value: value, Action: "direct"}})
		if err != nil {
			return -1, err
		}
		value = normalized[0].Value
	}
	for i, r := range rules {
		switch r.Match {
		case "domain":
			if value == r.Value {
				return i, nil
			}
		case "domain_suffix":
			if value == r.Value || strings.HasSuffix(value, "."+r.Value) {
				return i, nil
			}
		case "ip_cidr":
			if ipErr == nil {
				prefix, err := netip.ParsePrefix(r.Value)
				if err == nil && prefix.Contains(ip) {
					return i, nil
				}
			}
		}
	}
	return -1, nil
}
