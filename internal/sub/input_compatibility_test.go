package sub

import (
	"context"
	"testing"
)

func TestRegressionProviderUnsupportedFields(t *testing.T) {

	for _, field := range []string{"dialer-proxy: required-hop", "exclude-type: http", "unexpected-transport: x"} {
		body := []byte("proxy-providers:\n  sample:\n    type: inline\n    " + field + "\n    payload:\n    - {name: only, type: http, server: 127.0.0.1, port: 1}\n")
		if _, err := ExpandProviders(context.Background(), body, FetchOptions{}); err == nil {
			t.Fatalf("unsupported provider field accepted: %s", field)
		}
	}
}

func TestRegressionYAMLFirstLineURL(t *testing.T) {
	body := []byte("# Subscription homepage https://example.org\nproxies:\n- {name: valid, type: http, server: 127.0.0.1, port: 1}\nrules:\n- MATCH,DIRECT\n")
	links, proxies, _ := BodyToLinks(body)
	if proxies == nil {
		t.Fatalf("valid YAML treated as URI list: %q", links)
	}
}

func TestRegressionUnsupportedHTTPUpgradeIsRejected(t *testing.T) {
	nodes, skipped := ParseLinks("vless://11111111-1111-4111-8111-111111111111@127.0.0.1:1?type=httpupgrade&path=%2Fupgrade#test")
	if len(nodes) != 0 || skipped != 1 {
		t.Fatalf("unsupported HTTP Upgrade accepted: %v, skipped %d", nodes, skipped)
	}
}
