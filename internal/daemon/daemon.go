package daemon

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// ChildArg 守护化后重新执行自身时携带的隐藏参数。
const ChildArg = "--daemon-child"

func pidPath(dataDir string) string { return filepath.Join(dataDir, "fly.pid") }

// Running 返回守护进程 pid，0 表示未运行；顺带清理过期 pid 文件。
func Running(dataDir string) int {
	b, err := os.ReadFile(pidPath(dataDir))
	if err != nil {
		return 0
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil || pid <= 0 {
		_ = os.Remove(pidPath(dataDir))
		return 0
	}
	if !alive(pid) {
		_ = os.Remove(pidPath(dataDir))
		return 0
	}
	return pid
}

func alive(pid int) bool {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return p.Signal(syscall.Signal(0)) == nil
}

func WritePID(dataDir string) error {
	return os.WriteFile(pidPath(dataDir), []byte(strconv.Itoa(os.Getpid())), 0o600)
}

func RemovePID(dataDir string) { _ = os.Remove(pidPath(dataDir)) }

func portOpen(addr string) bool {
	conn, err := net.DialTimeout("tcp", addr, time.Second)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// Detach fork 自身进入后台，等待控制台端口就绪。
func Detach(dataDir, adminAddr string) error {
	if portOpen(adminAddr) {
		return fmt.Errorf("管理端口 %s 已被占用", adminAddr)
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	logPath := filepath.Join(dataDir, "fly.log")
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	fmt.Fprintf(f, "\n===== %s 启动 =====\n", time.Now().Format("2006-01-02 15:04:05"))

	cmd := exec.Command(exe, ChildArg)
	cmd.Stdout = f
	cmd.Stderr = f
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	exited := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(exited)
	}()

	deadline := time.Now().Add(3 * time.Minute)
	for time.Now().Before(deadline) {
		select {
		case <-exited:
			return fmt.Errorf("后台进程已退出，请查看日志 %s", logPath)
		default:
		}
		if portOpen(adminAddr) {
			return nil
		}
		time.Sleep(300 * time.Millisecond)
	}
	return fmt.Errorf("启动超时，请查看日志 %s", logPath)
}

// Stop 发送 SIGTERM 并等待进程退出。
func Stop(dataDir string) error {
	pid := Running(dataDir)
	if pid == 0 {
		return fmt.Errorf("fly 没有在运行")
	}
	p, _ := os.FindProcess(pid)
	if err := p.Signal(syscall.SIGTERM); err != nil {
		return err
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if !alive(pid) {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("进程 %d 未在 10 秒内退出，可尝试 kill -9", pid)
}
