package config

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"

	"fly/internal/storage"
	"fly/internal/sub"
	"gopkg.in/yaml.v3"
)

type Config struct {
	SubscriptionURL string `yaml:"subscription-url"`
	DownloadProxy   string `yaml:"download-proxy,omitempty"`
	Imported        bool   `yaml:"imported,omitempty"`
	MixedPort       int    `yaml:"mixed-port"`
	AdminAddr       string `yaml:"admin-addr"`
	RefreshHours    int    `yaml:"refresh-hours"`
}

func Default(dataDir string) *Config {
	return &Config{
		SubscriptionURL: "",
		MixedPort:       7888,
		AdminAddr:       "127.0.0.1:7889",
		RefreshHours:    24,
	}
}

// Load 读取 dataDir/fly.yaml；不存在则写入默认配置。
func Load(dataDir string) (*Config, error) {
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, err
	}
	path := filepath.Join(dataDir, "fly.yaml")
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		c := Default(dataDir)
		if err := Save(dataDir, c); err != nil {
			return nil, err
		}
		return c, nil
	}
	if err != nil {
		return nil, err
	}
	c := Default(dataDir)
	if err := yaml.Unmarshal(b, c); err != nil {
		return nil, err
	}
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return c, nil
}

func Save(dataDir string, c *Config) error {
	if err := c.Validate(); err != nil {
		return err
	}
	b, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	return storage.WriteFile(filepath.Join(dataDir, "fly.yaml"), b)
}

func (c *Config) Validate() error {
	if err := sub.ValidateProxy(c.DownloadProxy); err != nil {
		return err
	}
	if c.MixedPort < 1 || c.MixedPort > 65535 {
		return fmt.Errorf("代理端口须在 1–65535 之间")
	}
	host, port, err := net.SplitHostPort(c.AdminAddr)
	ip := net.ParseIP(host)
	if err != nil || ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("管理地址须使用本机回环 IP 和端口，例如 127.0.0.1:7889")
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 || n == c.MixedPort {
		return fmt.Errorf("管理端口无效或与代理端口重复")
	}
	if c.RefreshHours < 1 || c.RefreshHours > 8760 {
		return fmt.Errorf("订阅更新间隔须在 1–8760 小时之间")
	}
	return nil
}
