package sub

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

var placeholderPatterns = []string{"剩余流量", "到期", "重置", "跳转域名", "请勿连接", "官网", "流量说明", "更新订阅", "失联"}

func isPlaceholder(name string) bool {
	for _, p := range placeholderPatterns {
		if strings.Contains(name, p) {
			return true
		}
	}
	return false
}

func b64Decode(s string) ([]byte, error) {
	s = strings.TrimSpace(s)
	for _, e := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		if b, err := e.DecodeString(s); err == nil {
			return b, nil
		}
	}
	return nil, fmt.Errorf("invalid base64")
}

// ParseLinks 把订阅的分享链接列表（每行一条）解析为 mihomo proxies 条目。
func ParseLinks(text string) (proxies []map[string]any, skipped int) {
	seen := map[string]int{}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		i := strings.Index(line, "://")
		if i < 0 {
			continue
		}
		var (
			m   map[string]any
			err error
		)
		switch line[:i] {
		case "ss":
			m, err = parseSS(line)
		case "vless":
			m, err = parseVLESS(line)
		case "vmess":
			m, err = parseVMess(line)
		case "trojan":
			m, err = parseTrojan(line)
		case "hysteria2", "hy2":
			m, err = parseHysteria2(line)
		default:
			err = fmt.Errorf("unsupported scheme %q", line[:i])
		}
		if err != nil {
			skipped++
			continue
		}
		name, _ := m["name"].(string)
		if name == "" || isPlaceholder(name) {
			skipped++
			continue
		}
		if n, dup := seen[name]; dup {
			seen[name] = n + 1
			m["name"] = fmt.Sprintf("%s #%d", name, n+1)
		} else {
			seen[name] = 1
		}
		proxies = append(proxies, m)
	}
	return proxies, skipped
}

// FilterProxies 过滤 Clash YAML 订阅自带的 proxies：去占位节点、校验必备字段、去重名。
func FilterProxies(list []map[string]any) []map[string]any {
	seen := map[string]int{}
	out := make([]map[string]any, 0, len(list))
	for _, p := range list {
		name, _ := p["name"].(string)
		server, _ := p["server"].(string)
		_, hasType := p["type"]
		port := 0
		switch v := p["port"].(type) {
		case int:
			port = v
		case float64:
			port = int(v)
		}
		if name == "" || server == "" || !hasType || port == 0 || isPlaceholder(name) {
			continue
		}
		if n, dup := seen[name]; dup {
			seen[name] = n + 1
			p["name"] = fmt.Sprintf("%s #%d", name, n+1)
		} else {
			seen[name] = 1
		}
		out = append(out, p)
	}
	return out
}

func splitHostPort(hostport string) (host string, port int, err error) {
	host, portStr, err := net.SplitHostPort(hostport)
	if err != nil {
		return "", 0, err
	}
	port, err = strconv.Atoi(portStr)
	if err != nil || port <= 0 || port > 65535 {
		return "", 0, fmt.Errorf("bad port %q", portStr)
	}
	return host, port, nil
}

func parseSS(line string) (map[string]any, error) {
	body := line[len("ss://"):]
	name := ""
	if i := strings.LastIndex(body, "#"); i >= 0 {
		name, _ = url.PathUnescape(body[i+1:])
		body = body[:i]
	}
	if i := strings.Index(body, "?"); i >= 0 { // plugin 等参数，忽略
		body = body[:i]
	}
	userinfo, hostport := "", ""
	if i := strings.LastIndex(body, "@"); i >= 0 {
		userinfo, hostport = body[:i], body[i+1:]
	} else {
		b, err := b64Decode(body)
		if err != nil {
			return nil, fmt.Errorf("bad ss link: %w", err)
		}
		s := string(b)
		i := strings.LastIndex(s, "@")
		if i < 0 {
			return nil, fmt.Errorf("bad legacy ss link")
		}
		userinfo, hostport = s[:i], s[i+1:]
	}
	host, port, err := splitHostPort(hostport)
	if err != nil {
		return nil, err
	}
	method, password := "", ""
	if b, err := b64Decode(userinfo); err == nil && strings.Contains(string(b), ":") {
		method, password, _ = strings.Cut(string(b), ":")
	} else if strings.Contains(userinfo, ":") {
		method, password, _ = strings.Cut(userinfo, ":")
	} else {
		return nil, fmt.Errorf("bad ss userinfo")
	}
	if method == "" || password == "" || host == "" {
		return nil, fmt.Errorf("incomplete ss link")
	}
	return map[string]any{
		"name": name, "type": "ss", "server": host, "port": port,
		"cipher": method, "password": password, "udp": true,
	}, nil
}

func applyTransport(m map[string]any, q url.Values) {
	headers := func() map[string]any {
		if h := q.Get("host"); h != "" {
			return map[string]any{"Host": h}
		}
		return nil
	}
	switch q.Get("type") {
	case "ws":
		m["network"] = "ws"
		ws := map[string]any{}
		path := q.Get("path")
		if i := strings.Index(path, "?"); i >= 0 {
			for _, kv := range strings.Split(path[i+1:], "&") {
				k, v, _ := strings.Cut(kv, "=")
				if k == "ed" {
					ws["max-early-data"], _ = strconv.Atoi(v)
					ws["early-data-header-name"] = "Sec-WebSocket-Protocol"
				}
			}
			path = path[:i]
		}
		if path != "" {
			ws["path"] = path
		}
		if h := headers(); h != nil {
			ws["headers"] = h
		}
		if len(ws) > 0 {
			m["ws-opts"] = ws
		}
	case "grpc":
		m["network"] = "grpc"
		if sn := q.Get("serviceName"); sn != "" {
			m["grpc-opts"] = map[string]any{"grpc-service-name": sn}
		}
	case "httpupgrade":
		m["network"] = "httpupgrade"
		hu := map[string]any{}
		if h := headers(); h != nil {
			hu["host"] = q.Get("host")
		}
		if path := q.Get("path"); path != "" {
			hu["path"] = path
		}
		m["httpupgrade-opts"] = hu
	}
}

func applyTLS(m map[string]any, q url.Values) {
	switch q.Get("security") {
	case "tls", "reality":
		m["tls"] = true
		sni := q.Get("sni")
		if sni == "" {
			sni = q.Get("peer")
		}
		if sni != "" {
			m["servername"] = sni
		}
		if alpn := q.Get("alpn"); alpn != "" {
			m["alpn"] = strings.Split(alpn, ",")
		}
		if fp := q.Get("fp"); fp != "" {
			m["client-fingerprint"] = fp
		}
		if q.Get("security") == "reality" {
			ro := map[string]any{}
			if pbk := q.Get("pbk"); pbk != "" {
				ro["public-key"] = pbk
			}
			if sid := q.Get("sid"); sid != "" {
				ro["short-id"] = sid
			}
			if len(ro) > 0 {
				m["reality-opts"] = ro
			}
		}
	}
	if q.Get("allowInsecure") == "1" || q.Get("insecure") == "1" || q.Get("verify_cert") == "false" {
		m["skip-cert-verify"] = true
	}
}

func parseVLESS(line string) (map[string]any, error) {
	u, err := url.Parse(line)
	if err != nil {
		return nil, err
	}
	uuid := u.User.Username()
	host := u.Hostname()
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		return nil, fmt.Errorf("bad vless port")
	}
	if uuid == "" || host == "" {
		return nil, fmt.Errorf("incomplete vless link")
	}
	m := map[string]any{
		"name": u.Fragment, "type": "vless", "server": host, "port": port,
		"uuid": uuid, "udp": true,
	}
	applyTransport(m, u.Query())
	if flow := u.Query().Get("flow"); flow != "" {
		m["flow"] = flow
	}
	applyTLS(m, u.Query())
	return m, nil
}

func parseVMess(line string) (map[string]any, error) {
	b, err := b64Decode(line[len("vmess://"):])
	if err != nil {
		return nil, err
	}
	var v struct {
		PS   string `json:"ps"`
		Add  string `json:"add"`
		Port any    `json:"port"`
		ID   string `json:"id"`
		Aid  any    `json:"aid"`
		Scy  string `json:"scy"`
		Net  string `json:"net"`
		Type string `json:"type"`
		Host string `json:"host"`
		Path string `json:"path"`
		TLS  string `json:"tls"`
		SNI  string `json:"sni"`
		FP   string `json:"fp"`
	}
	if err := json.Unmarshal(b, &v); err != nil {
		return nil, err
	}
	port := 0
	switch p := v.Port.(type) {
	case string:
		port, _ = strconv.Atoi(p)
	case float64:
		port = int(p)
	}
	if v.Add == "" || v.ID == "" || port == 0 {
		return nil, fmt.Errorf("incomplete vmess link")
	}
	m := map[string]any{
		"name": v.PS, "type": "vmess", "server": v.Add, "port": port,
		"uuid": v.ID, "udp": true,
	}
	if v.Scy != "" {
		m["cipher"] = v.Scy
	}
	switch aid := v.Aid.(type) {
	case float64:
		if aid > 0 {
			m["alterId"] = int(aid)
		}
	case string:
		if n, err := strconv.Atoi(aid); err == nil && n > 0 {
			m["alterId"] = n
		}
	}
	q := url.Values{}
	for k, val := range map[string]string{
		"type": v.Net, "host": v.Host, "path": v.Path, "serviceName": v.Path,
	} {
		if val != "" {
			q.Set(k, val)
		}
	}
	applyTransport(m, q)
	if v.TLS == "tls" || v.TLS == "reality" {
		q2 := url.Values{"security": {v.TLS}}
		if v.SNI != "" {
			q2.Set("sni", v.SNI)
		}
		if v.FP != "" {
			q2.Set("fp", v.FP)
		}
		applyTLS(m, q2)
	}
	return m, nil
}

func parseTrojan(line string) (map[string]any, error) {
	u, err := url.Parse(line)
	if err != nil {
		return nil, err
	}
	host := u.Hostname()
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		return nil, fmt.Errorf("bad trojan port")
	}
	password, _ := u.User.Password()
	if password == "" {
		password = u.User.Username()
	}
	if host == "" || password == "" {
		return nil, fmt.Errorf("incomplete trojan link")
	}
	m := map[string]any{
		"name": u.Fragment, "type": "trojan", "server": host, "port": port,
		"password": password, "udp": true,
	}
	if sni := u.Query().Get("sni"); sni != "" {
		m["servername"] = sni
	}
	applyTransport(m, u.Query())
	applyTLS(m, u.Query())
	return m, nil
}

func parseHysteria2(line string) (map[string]any, error) {
	u, err := url.Parse(line)
	if err != nil {
		return nil, err
	}
	host := u.Hostname()
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		return nil, fmt.Errorf("bad hysteria2 port")
	}
	auth := u.User.Username()
	if p, ok := u.User.Password(); ok {
		auth = auth + ":" + p
	}
	if host == "" || auth == "" {
		return nil, fmt.Errorf("incomplete hysteria2 link")
	}
	m := map[string]any{
		"name": u.Fragment, "type": "hysteria2", "server": host, "port": port,
		"password": auth,
	}
	if sni := u.Query().Get("sni"); sni != "" {
		m["servername"] = sni
	}
	if u.Query().Get("insecure") == "1" {
		m["skip-cert-verify"] = true
	}
	if ob := u.Query().Get("obfs"); ob != "" {
		m["obfs"] = ob
		if obpw := u.Query().Get("obfs-password"); obpw != "" {
			m["obfs-password"] = obpw
		}
	}
	return m, nil
}
