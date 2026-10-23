package config

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// DefaultSubscriptionURL 测试用订阅（NCloud）。
const DefaultSubscriptionURL = "https://api-a.microconsub.com/sd/fb5f64b4603d003ae4768b2c00fd6db5"

type Config struct {
	SubscriptionURL string `yaml:"subscription-url"`
	MixedPort       int    `yaml:"mixed-port"`
	ControllerAddr  string `yaml:"controller-addr"`
	AdminAddr       string `yaml:"admin-addr"`
	Secret          string `yaml:"secret"`
	RefreshHours    int    `yaml:"refresh-hours"`
}

func Default(dataDir string) *Config {
	return &Config{
		SubscriptionURL: DefaultSubscriptionURL,
		MixedPort:       7888,
		ControllerAddr:  "127.0.0.1:9090",
		AdminAddr:       "127.0.0.1:7889",
		Secret:          randomSecret(),
		RefreshHours:    24,
	}
}

func randomSecret() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
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
	if c.Secret == "" {
		c.Secret = randomSecret()
	}
	return c, nil
}

func Save(dataDir string, c *Config) error {
	b, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dataDir, "fly.yaml"), b, 0o600)
}
