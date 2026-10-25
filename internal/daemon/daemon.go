package daemon

import (
	"encoding/json"
	"fly/internal/storage"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// ChildArg 守护化后重新执行自身时携带的隐藏参数。
const ChildArg = "--daemon-child"

func pidPath(dataDir string) string { return filepath.Join(dataDir, "fly.pid") }

type processRecord struct {
	PID     int    `json:"pid"`
	Started string `json:"started"`
	Command string `json:"command"`
}

var ownership struct {
	sync.Mutex
	file *os.File
}

func identity(pid int) (processRecord, error) {
	read := func(field string) (string, error) {
		cmd := exec.Command("ps", "-ww", "-p", strconv.Itoa(pid), "-o", field+"=")
		cmd.Env = append(os.Environ(), "LC_ALL=C")
		b, err := cmd.Output()
		return strings.TrimSpace(string(b)), err
	}
	started, err := read("lstart")
	if err != nil {
		return processRecord{}, err
	}
	command, err := read("command")
	if err != nil || started == "" || command == "" {
		return processRecord{}, fmt.Errorf("进程身份不可用")
	}
	return processRecord{pid, started, command}, nil
}

// Running checks process identity, not only PID existence. A stale PID must never authorize a kill.
func Running(dataDir string) int {
	b, err := os.ReadFile(pidPath(dataDir))
	if err != nil {
		return 0
	}
	var record processRecord
	if json.Unmarshal(b, &record) != nil {
		// Legacy versions used only a PID and had no custom data-directory support.
		pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
		if err != nil || pid <= 0 {
			_ = os.Remove(pidPath(dataDir))
			return 0
		}
		actual, err := identity(pid)
		if err != nil {
			return 0
		}
		suffix := " " + ChildArg
		if !strings.HasSuffix(actual.Command, suffix) {
			return 0
		}
		executable := strings.TrimSpace(strings.TrimSuffix(actual.Command, suffix))
		current, _ := os.Executable()
		if filepath.Base(executable) != "fly" && filepath.Base(executable) != filepath.Base(current) {
			return 0
		}
		return pid
	}
	if record.PID <= 0 {
		return 0
	}
	actual, err := identity(record.PID)
	if err != nil || actual.Started != record.Started || actual.Command != record.Command {
		_ = os.Remove(pidPath(dataDir))
		return 0
	}
	return record.PID
}

func alive(pid int) bool {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return p.Signal(syscall.Signal(0)) == nil
}

func WritePID(dataDir string) error {
	ownership.Lock()
	defer ownership.Unlock()
	if ownership.file != nil {
		return fmt.Errorf("后台进程已持有启动锁")
	}
	f, err := os.OpenFile(filepath.Join(dataDir, "fly.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	if err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return fmt.Errorf("已有 fly 正在启动或运行")
	}
	record, err := identity(os.Getpid())
	if err == nil {
		var b []byte
		b, err = json.Marshal(record)
		if err == nil {
			err = storage.WriteFile(pidPath(dataDir), b)
		}
	}
	if err != nil {
		f.Close()
		return fmt.Errorf("记录后台进程身份失败：请确认 ps 可用且数据目录可写")
	}
	ownership.file = f
	return nil
}
func RemovePID(dataDir string) {
	ownership.Lock()
	defer ownership.Unlock()
	b, _ := os.ReadFile(pidPath(dataDir))
	var record processRecord
	if json.Unmarshal(b, &record) == nil && record.PID == os.Getpid() {
		_ = os.Remove(pidPath(dataDir))
	}
	if ownership.file != nil {
		_ = ownership.file.Close()
		ownership.file = nil
	}
}

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

	cmd := exec.Command(exe, ChildArg, fmt.Sprintf("--instance=%d", time.Now().UnixNano()))
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
