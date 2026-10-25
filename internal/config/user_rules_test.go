package config

import (
	"reflect"
	"testing"
)

func TestUserRulesNormalizeAndKeepFirstMatchOrder(t *testing.T) {
	got, err := NormalizeRules([]UserRule{{Match: "domain_suffix", Value: "https://EXAMPLE.org/path", Action: "direct"}, {Match: "ip_cidr", Value: "10.1.2.3/8", Action: "reject"}, {Match: "domain", Value: "www.example.org", Action: "proxy"}})
	if err != nil {
		t.Fatal(err)
	}
	want := []UserRule{{"domain_suffix", "example.org", "direct"}, {"ip_cidr", "10.0.0.0/8", "reject"}, {"domain", "www.example.org", "proxy"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalization or ordering lost: %#v", got)
	}
	for _, bad := range []UserRule{{"domain", "a,b.com", "proxy"}, {"ip_cidr", "127.0.0.1", "direct"}, {"domain", "example.org", "unknown"}, {"domain_suffix", "*.org/path", "direct"}} {
		if _, err := NormalizeRules([]UserRule{bad}); err == nil {
			t.Fatalf("unsafe rule accepted: %+v", bad)
		}
	}
}
func TestRulesPersistAndRejectDuplicateWithoutOverwriting(t *testing.T) {
	dir := t.TempDir()
	old := []UserRule{{"domain", "example.org", "direct"}}
	if err := SaveRules(dir, old); err != nil {
		t.Fatal(err)
	}
	if err := SaveRules(dir, []UserRule{old[0], old[0]}); err == nil {
		t.Fatal("duplicate accepted")
	}
	got, err := LoadRules(dir)
	if err != nil || !reflect.DeepEqual(got, old) {
		t.Fatalf("invalid save destroyed rules: %v %v", got, err)
	}
}

func TestExplainRespectsRuleOrderAndDomainBoundaries(t *testing.T) {
	rules := []UserRule{{"domain", "api.example.org", "reject"}, {"domain_suffix", "example.org", "direct"}, {"ip_cidr", "10.0.0.0/8", "proxy"}}
	for _, tc := range []struct {
		input string
		index int
	}{{"https://api.example.org/a", 0}, {"www.example.org", 1}, {"notexample.org", -1}, {"10.1.2.3", 2}} {
		index, err := MatchUserRule(rules, tc.input)
		if err != nil || index != tc.index {
			t.Fatalf("%s -> %d %v", tc.input, index, err)
		}
	}
}
