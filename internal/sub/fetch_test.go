package sub

import (
	"reflect"
	"testing"
)

func TestRewritePreservesPoliciesAndModifiers(t *testing.T) {
	got := RewriteRules([]string{"DOMAIN,example.org, PASS-RULE", "IP-CIDR,10.0.0.0/8,OldGroup,no-resolve,src", "DOMAIN,local.test, DIRECT "}, "Selected")
	want := []string{"DOMAIN,example.org,PASS-RULE", "IP-CIDR,10.0.0.0/8,Selected,no-resolve,src", "DOMAIN,local.test,DIRECT", "MATCH,Selected"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q want %q", got, want)
	}
}
