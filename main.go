package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"fly/internal/app"
	"fly/internal/config"
	"fly/internal/core"
	"fly/internal/daemon"
	"fly/internal/server"
	"fly/internal/sub"
)

const version = "0.1.0"

var localClient = &http.Client{
	Timeout:   8 * time.Second,
	Transport: &http.Transport{Proxy: nil},
}

func main() {
	cmd := "start"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}
	switch cmd {
	case "", "start":
		cmdStart()
	case "run":
		serve(false)
	case daemon.ChildArg:
		serve(true)
	case "stop":
		cmdStop()
	case "restart":
		cmdRestart()
	case "status":
		cmdStatus()
	case "ui":
		cmdUI()
	case "update":
		cmdUpdate()
	case "init":
		cmdInit()
	case "version", "-v", "--version":
		fmt.Println("fly", version)
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "未知命令 %q\n\n", cmd)
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Print(`fly — 配合 Proxy Switcher 使用的本地代理服务器

浏览器流量路径: Proxy Switcher → 127.0.0.1:7888 → fly → 订阅节点
管理网页: http://127.0.0.1:7889（默认）

用法:
  fly            后台启动（守护进程）
  fly run        前台运行（调试）
  fly stop       停止
  fly restart    重启
  fly status     运行状态与当前节点
  fly ui         打开控制台页面
  fly update     立即刷新订阅并热重载
  fly init       查看/修改配置（--url <订阅地址> --port <代理端口> --admin <本机管理地址>）
`)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "错误:", err)
	os.Exit(1)
}

func mustApp() *app.App {
	a, err := app.New()
	if err != nil {
		fatal(err)
	}
	return a
}

func cmdStart() {
	a := mustApp()
	if pid := daemon.Running(a.Dir); pid != 0 {
		fmt.Printf("fly 已在运行（pid %d），控制台 http://%s\n", pid, a.Cfg.AdminAddr)
		return
	}
	if err := daemon.Detach(a.Dir, a.Cfg.AdminAddr); err != nil {
		fatal(err)
	}
	fmt.Printf("fly 已在后台启动\n控制台: http://%s   日志: %s\n", a.Cfg.AdminAddr, filepath.Join(a.Dir, "fly.log"))
}

// serve 是前台/守护共用的服务主体。控制台先就绪（starting 状态），订阅加载完再放代理。
func serve(daemonized bool) {
	a := mustApp()
	if daemonized {
		if err := daemon.WritePID(a.Dir); err != nil {
			fatal(err)
		}
		defer daemon.RemovePID(a.Dir)
	}
	if err := core.Init(a.Dir); err != nil {
		fatal(err)
	}
	a.SetStarting(a.HasSubscription())
	srv := server.New(a, version)
	if err := srv.Start(); err != nil {
		fatal(fmt.Errorf("管理端口启动失败: %w", err))
	}
	if err := a.Initialize(); err != nil {
		fmt.Fprintln(os.Stderr, "订阅未就绪，可在管理网页修改地址:", err)
	}
	go func() {
		for range time.Tick(time.Duration(a.Cfg.RefreshHours) * time.Hour) {
			if !a.HasSubscription() || a.ConfigSnapshot().Imported {
				continue
			}
			if err := a.Refresh(); err != nil {
				fmt.Println("定时刷新订阅失败:", err)
			}
		}
	}()
	if !daemonized {
		info, nodes, skipped, lastErr := a.Snapshot()
		fmt.Printf("订阅解析: %d 个节点（跳过 %d）\n", nodes, skipped)
		if lastErr != "" {
			fmt.Printf("[警告] 订阅未就绪或正在使用缓存: %s\n", lastErr)
		}
		fmt.Printf("代理 http://127.0.0.1:%d · 控制台 http://%s （Ctrl+C 退出）\n", a.Cfg.MixedPort, a.Cfg.AdminAddr)
		if info != nil {
			if t := info.ExpireTime(); !t.IsZero() {
				fmt.Printf("订阅到期: %s\n", t.Format("2006-01-02"))
			}
		}
	}
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	<-sigs
}

func cmdStop() {
	a := mustApp()
	if err := daemon.Stop(a.Dir); err != nil {
		fatal(err)
	}
	fmt.Println("fly 已停止")
}

func cmdRestart() {
	a := mustApp()
	if daemon.Running(a.Dir) != 0 {
		if err := daemon.Stop(a.Dir); err != nil {
			fatal(err)
		}
	}
	cmdStart()
}

func cmdUI() {
	a := mustApp()
	address := "http://" + a.Cfg.AdminAddr
	fmt.Println("管理网页:", address)
	opener := "open"
	if runtime.GOOS != "darwin" {
		opener = "xdg-open"
	}
	if err := exec.Command(opener, address).Start(); err != nil {
		fmt.Println("无法自动打开，请把上方地址复制到浏览器。")
	}
}

func getJSON(url string, out any) error {
	resp, err := localClient.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, out)
}

func cmdUpdate() {
	a := mustApp()
	resp, err := localClient.Post("http://"+a.Cfg.AdminAddr+"/api/update", "", nil)
	if err != nil {
		fatal(fmt.Errorf("无法连接 fly（是否在运行？）: %w", err))
	}
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
		resp.Body.Close()
		fatal(fmt.Errorf("更新失败: %s", body))
	}
	resp.Body.Close()
	fmt.Print("正在刷新订阅")
	for i := 0; i < 60; i++ {
		time.Sleep(2 * time.Second)
		fmt.Print(".")
		var info struct {
			LastErr   string `json:"last_err"`
			Updating  bool   `json:"updating"`
			UpdateErr string `json:"update_err"`
			Nodes     int    `json:"nodes"`
			Current   string `json:"current"`
		}
		if err := getJSON("http://"+a.Cfg.AdminAddr+"/api/info", &info); err == nil {
			if !info.Updating {
				fmt.Println()
				if info.LastErr != "" {
					fmt.Println("刷新失败，继续使用缓存:", info.LastErr)
					os.Exit(1)
				}
				if info.UpdateErr != "" {
					fmt.Println("刷新失败:", info.UpdateErr)
					os.Exit(1)
				}
				fmt.Printf("刷新完成：%d 个节点，当前节点 %s\n", info.Nodes, info.Current)
				return
			}
		}
	}
	fmt.Println("\n仍在刷新，稍后可用 fly status 查看")
}

func cmdStatus() {
	a := mustApp()
	pid := daemon.Running(a.Dir)
	fmt.Printf("fly %s  ", version)
	if pid == 0 {
		fmt.Println("状态: 未运行")
		fmt.Printf("代理端口(未监听): 127.0.0.1:%d\n", a.Cfg.MixedPort)
		return
	}
	fmt.Printf("状态: 运行中（pid %d）\n", pid)
	var info struct {
		ProxyPort    int    `json:"proxy_port"`
		Nodes        int    `json:"nodes"`
		Current      string `json:"current"`
		AutoNow      string `json:"auto_now"`
		LastErr      string `json:"last_err"`
		UpdateErr    string `json:"update_err"`
		Updating     bool   `json:"updating"`
		Starting     bool   `json:"starting"`
		AdminAddr    string `json:"admin_addr"`
		Ready        bool   `json:"ready"`
		Subscription *struct {
			Used   int64 `json:"used"`
			Total  int64 `json:"total"`
			Expire int64 `json:"expire"`
		} `json:"subscription"`
	}
	if err := getJSON("http://"+a.Cfg.AdminAddr+"/api/info", &info); err != nil {
		fmt.Println("控制台无响应:", err)
		return
	}
	fmt.Printf("Proxy Switcher 代理: 127.0.0.1:%d（HTTP / SOCKS5）\n管理网页: http://%s\n", info.ProxyPort, info.AdminAddr)
	if info.Starting {
		fmt.Println("订阅: 正在启动加载（订阅源慢时可能需要 1-2 分钟），稍后再查")
		return
	}
	if !info.Ready {
		fmt.Println("代理尚未就绪，请在管理网页设置或检查订阅")
		if info.LastErr != "" {
			fmt.Println("原因:", info.LastErr)
		}
		return
	}
	fmt.Printf("节点: %d   当前: %s\n", info.Nodes, orNA(info.Current))
	if info.AutoNow != "" {
		fmt.Printf("自动选择最快: %s\n", info.AutoNow)
	}
	if info.Updating {
		fmt.Println("订阅: 正在刷新…")
	} else if info.UpdateErr != "" {
		fmt.Println("订阅: 上次刷新失败 -", info.UpdateErr)
	}
	if info.LastErr != "" {
		fmt.Println("缓存模式（最近一次拉取失败）:", info.LastErr)
	}
	if s := info.Subscription; s != nil {
		if s.Total > 0 {
			fmt.Printf("流量: %.1f / %.1f GB\n", float64(s.Used)/1073741824, float64(s.Total)/1073741824)
		}
		if s.Expire > 0 {
			fmt.Printf("到期: %s\n", time.Unix(s.Expire, 0).Format("2006-01-02"))
		}
	}
}

func orNA(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func cmdInit() {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	source := fs.String("url", "", "验证并更换订阅地址（推荐使用管理网页）")
	port := fs.Int("port", 0, "本地代理端口")
	admin := fs.String("admin", "", "本机管理地址，例如 127.0.0.1:7889")
	_ = fs.Parse(os.Args[2:])
	a := mustApp()
	active := daemon.Running(a.Dir) != 0
	if active && (*port != 0 && *port != a.Cfg.MixedPort || *admin != "" && *admin != a.Cfg.AdminAddr) {
		fatal(fmt.Errorf("请先运行 fly stop，再修改代理或管理端口；修改后运行 fly start"))
	}
	if *port != 0 {
		a.Cfg.MixedPort = *port
	}
	if *admin != "" {
		a.Cfg.AdminAddr = *admin
	}
	if err := a.Cfg.Validate(); err != nil {
		fatal(err)
	}
	if *source != "" {
		*source = strings.TrimSpace(*source)
		if err := sub.ValidateURL(*source); err != nil {
			fatal(err)
		}
		if active {
			data, _ := json.Marshal(map[string]string{"url": *source, "download_proxy": a.Cfg.DownloadProxy})
			client := &http.Client{Transport: localClient.Transport, Timeout: 120 * time.Second}
			resp, err := client.Post("http://"+a.Cfg.AdminAddr+"/api/subscription", "application/json", bytes.NewReader(data))
			if err != nil {
				fatal(fmt.Errorf("无法完成订阅验证，请在管理网页重试"))
			}
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
			resp.Body.Close()
			if resp.StatusCode >= 300 {
				fatal(fmt.Errorf("%s", sub.Redact(string(body))))
			}
			loaded, err := config.Load(a.Dir)
			if err != nil {
				fatal(err)
			}
			a.Cfg = loaded
		} else {
			if err := core.Init(a.Dir); err != nil {
				fatal(err)
			}
			defer core.StopMixed()
			if err := a.ChangeSubscription(context.Background(), *source, a.Cfg.DownloadProxy, nil); err != nil {
				fatal(err)
			}
		}
		fmt.Println("订阅已验证并保存；节点可达性请到管理网页测试。")
	} else if *port != 0 || *admin != "" {
		if err := config.Save(a.Dir, a.Cfg); err != nil {
			fatal(err)
		}
		fmt.Println("配置已保存")
	}
	fmt.Printf("配置文件: %s\n", filepath.Join(a.Dir, "fly.yaml"))
	description := "尚未设置"
	if a.Cfg.Imported {
		description = "本地导入"
	} else if a.Cfg.SubscriptionURL != "" {
		description = "已设置（管理页可查看）"
	}
	fmt.Printf("订阅来源: %s\n代理端口: %d\n管理网页: http://%s\n", description, a.Cfg.MixedPort, a.Cfg.AdminAddr)
}
