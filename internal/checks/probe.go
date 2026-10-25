package checks

import (
	"context"
	"errors"
	"fly/internal/core"
	"fmt"
	C "github.com/metacubex/mihomo/constant"
	"net"
	"net/http"
	"net/url"
	"time"
)

const LatencyURL = "https://www.gstatic.com/generate_204"

var Websites = []Task{
	{Name: "Google", URL: LatencyURL},
	{Name: "YouTube", URL: "https://www.youtube.com/robots.txt"},
	{Name: "GitHub", URL: "https://github.com/robots.txt"},
	{Name: "ChatGPT", URL: "https://chatgpt.com/"},
	{Name: "Bing", URL: "https://www.bing.com/robots.txt"},
	{Name: "百度", URL: "https://www.baidu.com/robots.txt"},
}

func NodeClient(p C.Proxy) *http.Client {
	tr := &http.Transport{Proxy: nil, DialContext: core.ProxyDial(p), TLSHandshakeTimeout: 6 * time.Second, ResponseHeaderTimeout: 8 * time.Second, DisableKeepAlives: true}
	return client(tr)
}
func MixedClient(port int) *http.Client {
	proxy, _ := url.Parse("http://" + net.JoinHostPort("127.0.0.1", fmt.Sprint(port)))
	return client(&http.Transport{Proxy: http.ProxyURL(proxy), DialContext: (&net.Dialer{Timeout: 3 * time.Second}).DialContext, TLSHandshakeTimeout: 6 * time.Second, DisableKeepAlives: true})
}
func client(tr *http.Transport) *http.Client {
	return &http.Client{Transport: tr, Timeout: 8 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 3 {
			return http.ErrUseLastResponse
		}
		return nil
	}}
}
func networkResult(ctx context.Context, err error) Result {
	if errors.Is(ctx.Err(), context.Canceled) {
		return Result{Status: "canceled", Message: "已取消"}
	}
	var timeout net.Error
	if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.As(err, &timeout) && timeout.Timeout() {
		return Result{Status: "timeout", Message: "连接超时，请换节点或检查本机网络"}
	}
	var dns *net.DNSError
	if errors.As(err, &dns) {
		return Result{Status: "failed", Message: "DNS 解析失败，请检查本机网络和节点地址"}
	}
	return Result{Status: "failed", Message: "连接失败，请换节点或检查本机网络；未证明网站可达"}
}
func WebsiteProbe(ctx context.Context, task Task, c *http.Client) Result {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, task.URL, nil)
	if err != nil {
		return Result{Status: "failed", Message: "测试地址无效"}
	}
	req.Header.Set("User-Agent", "fly-diagnostic/1.0")
	started := time.Now()
	resp, err := c.Do(req)
	if err != nil {
		return networkResult(ctx, err)
	}
	defer resp.Body.Close()
	// A GET response header is enough for a reachability diagnostic; do not download page bundles.
	delay := int(time.Since(started).Milliseconds())
	if delay < 1 {
		delay = 1
	}
	result := Result{HTTPStatus: resp.StatusCode, Delay: delay, Status: "ok", Message: "测试地址响应正常"}
	switch {
	case resp.StatusCode == 401 || resp.StatusCode == 403 || resp.StatusCode == 429:
		result.Status = "restricted"
		result.Message = "已收到网站响应，但访问受限；不代表节点完全断网"
	case resp.StatusCode >= 300 && resp.StatusCode < 400:
		result.Status = "restricted"
		result.Message = "网站仍在重定向，请在浏览器确认"
	case resp.StatusCode < 200 || resp.StatusCode >= 400:
		result.Status = "site_error"
		result.Message = "已收到网站响应，但测试地址返回错误；请在浏览器确认"
	}
	return result
}
func LatencyProbe(ctx context.Context, p C.Proxy, target string) Result {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	c := NodeClient(p)
	c.Timeout = 5 * time.Second
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, target, nil)
	if err != nil {
		return Result{Status: "failed", Message: "延迟测试地址无效"}
	}
	started := time.Now()
	resp, err := c.Do(req)
	if err != nil {
		return networkResult(ctx, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 204 {
		return Result{Status: "failed", HTTPStatus: resp.StatusCode, Message: "延迟目标响应不是 HTTP 204，请另测常用网站确认"}
	}
	delay := int(time.Since(started).Milliseconds())
	if delay < 1 {
		delay = 1
	}
	return Result{Status: "ok", HTTPStatus: 204, Delay: delay, Message: "HEAD 到响应头（含连接握手）；其他网站需单独测试"}
}
