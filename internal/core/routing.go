package core

import (
	"fmt"
	P "github.com/metacubex/mihomo/constant/provider"
	R "github.com/metacubex/mihomo/rules"
	"github.com/metacubex/mihomo/rules/common"
	RP "github.com/metacubex/mihomo/rules/provider"
	"gopkg.in/yaml.v3"
	"io"
	"strings"
)

func ValidateRuleResource(content []byte, behavior, format string) error {
	b, err := P.ParseBehavior(behavior)
	if err != nil {
		return fmt.Errorf("规则集 behavior 无效")
	}
	if format == "" {
		format = "yaml"
	}
	f, err := P.ParseRuleFormat(format)
	if err != nil {
		return fmt.Errorf("规则集 format 无效")
	}
	if b != P.Classical {
		return RP.ConvertToMrs(content, b, f, io.Discard)
	}
	if f == P.MrsRule {
		return fmt.Errorf("classical 规则不支持 MRS")
	}
	var lines []string
	if f == P.TextRule {
		lines = strings.Split(string(content), "\n")
	} else {
		var payload struct {
			Payload []string `yaml:"payload"`
			Rules   []string `yaml:"rules"`
		}
		if yaml.Unmarshal(content, &payload) != nil || payload.Payload == nil && payload.Rules == nil {
			return fmt.Errorf("规则集须是 payload/rules YAML 或指定 text 格式")
		}
		lines = append(payload.Payload, payload.Rules...)
	}
	count := 0
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}
		typ, payload, target, params := common.ParseRulePayload(line, false)
		if typ == "MATCH" || typ == "RULE-SET" || typ == "SUB-RULE" {
			return fmt.Errorf("classical 规则集包含不支持的规则类型")
		}
		if _, err := R.ParseRule(typ, payload, target, params, nil); err != nil {
			return fmt.Errorf("规则集包含无效条目，请更新规则源")
		}
		count++
	}
	if count == 0 {
		return fmt.Errorf("规则资源为空")
	}
	return nil
}
