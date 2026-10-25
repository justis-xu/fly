package sub

import (
	"reflect"
	"strings"
	"testing"
)

func TestRewritePreservesPoliciesAndModifiers(t *testing.T) {
	got := RewriteRules([]string{"DOMAIN,example.org, PASS-RULE", "IP-CIDR,10.0.0.0/8,OldGroup,no-resolve,src", "DOMAIN,local.test, DIRECT "}, "Selected")
	want := []string{"DOMAIN,example.org,PASS-RULE", "IP-CIDR,10.0.0.0/8,Selected,no-resolve,src", "DOMAIN,local.test,DIRECT", "MATCH,Selected"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestGlobalGroupRulesUseTheSelectedNode(t *testing.T) {
	got := RewriteRules([]string{"DOMAIN,example.org,GLOBAL", "MATCH,GLOBAL"}, "Selected")
	want := []string{"DOMAIN,example.org,Selected", "MATCH,Selected"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("GLOBAL bypassed the fly selection: %q", got)
	}
}

func TestErrorsRedactNodeSharingCredentialsAsWellAsHTTPURLs(t *testing.T) {
	for _, uri := range []string{"vless://private-uuid@example.org:443?token=secret", "ss://private-password@example.org:443", "https://example.org/token?key=secret"} {
		if got := Redact("规则无效 " + uri); strings.Contains(got, "private") || strings.Contains(got, "secret") || strings.Contains(got, "example.org") {
			t.Fatalf("credential leaked: %s", got)
		}
	}
}
