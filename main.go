package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"fly/internal/app"
	"fly/internal/config"
	"fly/internal/core"
	"fly/internal/daemon"
	"fly/internal/server"
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
	fmt.Print(`fly — 轻量本地代理

用法:
  fly            后台启动（守护进程）
  fly run        前台运行（调试）
  fly stop       停止
  fly restart    重启
  fly status     运行状态与当前节点
  fly ui         打开控制台页面
  fly update     立即刷新订阅并热重载
  fly init       查看/修改配置（--url <订阅地址> --port <代理端口>）
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
	proxyAddr := fmt.Sprintf("127.0.0.1:%d", a.Cfg.MixedPort)
	if err := daemon.Detach(a.Dir, proxyAddr); err != nil {
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
	a.SetStarting(true)
	srv := server.New(a, version)
	go func() {
		if err := srv.ListenAndServe(); err != nil {
			fmt.Fprintln(os.Stderr, "控制台启动失败:", err)
		}
	}()
	if err := a.LoadSubscription(); err != nil {
		fatal(err)
	}
	if err := a.StartCore(); err != nil {
		fatal(err)
	}
	a.SetStarting(false)
	go func() {
		for range time.Tick(time.Duration(a.Cfg.RefreshHours) * time.Hour) {
			if err := a.Refresh(); err != nil {
				fmt.Println("定时刷新订阅失败:", err)
			}
		}
	}()
	if !daemonized {
		info, nodes, skipped, lastErr := a.Snapshot()
		fmt.Printf("订阅解析: %d 个节点（跳过 %d）\n", nodes, skipped)
		if lastErr != "" {
			fmt.Printf("[警告] 使用本地缓存: %s\n", lastErr)
		}
		fmt.Printf("代理 http://127.0.0.1:%d · 控制台 http://%s （Ctrl+C 退出）\n", a.Cfg.MixedPort, a.Cfg.AdminAddr)
		if t := info.ExpireTime(); !t.IsZero() {
			fmt.Printf("订阅到期: %s\n", t.Format("2006-01-02"))
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
	_ = exec.Command("open", "http://"+a.Cfg.AdminAddr).Start()
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
	resp.Body.Close()
	fmt.Print("正在刷新订阅")
	for i := 0; i < 60; i++ {
		time.Sleep(2 * time.Second)
		fmt.Print(".")
		var info struct {
			Updating  bool   `json:"updating"`
			UpdateErr string `json:"update_err"`
			Nodes     int    `json:"nodes"`
			Current   string `json:"current"`
		}
		if err := getJSON("http://"+a.Cfg.AdminAddr+"/api/info", &info); err == nil {
			if !info.Updating {
				fmt.Println()
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
		ProxyPort  int    `json:"proxy_port"`
		Nodes      int    `json:"nodes"`
		Current    string `json:"current"`
		AutoNow    string `json:"auto_now"`
		LastErr    string `json:"last_err"`
		UpdateErr  string `json:"update_err"`
		Updating   bool   `json:"updating"`
		Starting   bool   `json:"starting"`
		Controller struct {
			Host string `json:"host"`
			Port int    `json:"port"`
		} `json:"controller"`
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
	fmt.Printf("代理: http://127.0.0.1:%d   控制台: http://127.0.0.1:%d\n", info.ProxyPort, info.Controller.Port)
	if info.Starting {
		fmt.Println("订阅: 正在启动加载（订阅源慢时可能需要 1-2 分钟），稍后再查")
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
	url := fs.String("url", "", "订阅地址")
	port := fs.Int("port", 0, "本地代理端口")
	_ = fs.Parse(os.Args[2:])

	a := mustApp()
	if *url != "" {
		a.Cfg.SubscriptionURL = *url
	}
	if *port != 0 {
		a.Cfg.MixedPort = *port
	}
	if *url != "" || *port != 0 {
		if err := config.Save(a.Dir, a.Cfg); err != nil {
			fatal(err)
		}
		fmt.Println("配置已保存")
	}
	fmt.Printf("配置文件: %s\n", filepath.Join(a.Dir, "fly.yaml"))
	fmt.Printf("订阅地址: %s\n", a.Cfg.SubscriptionURL)
	fmt.Printf("代理端口: %d\n", a.Cfg.MixedPort)
}
