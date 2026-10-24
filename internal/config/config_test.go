package config

import "testing"

func TestFreshInstallHasNoSubscription(t *testing.T) {
	c, err := Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if c.SubscriptionURL != "" {
		t.Fatal("fresh install must ask user for their own subscription")
	}
}

func TestRejectInvalidServerConfiguration(t *testing.T) {
	for _, change := range []func(*Config){func(c *Config) { c.MixedPort = -1 }, func(c *Config) { c.AdminAddr = "0.0.0.0:7889" }, func(c *Config) { c.AdminAddr = "127.0.0.1:7888" }, func(c *Config) { c.RefreshHours = 0 }} {
		c := Default("")
		change(c)
		if err := Save(t.TempDir(), c); err == nil {
			t.Fatalf("invalid config accepted: %+v", c)
		}
	}
}
