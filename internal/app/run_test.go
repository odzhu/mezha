package app

import (
	"strings"
	"testing"
)

func TestInteractiveTTYEnabledExplicitValue(t *testing.T) {
	interactive := true
	if !interactiveTTYEnabled(&interactive) {
		t.Error("interactiveTTYEnabled(true) = false, want true")
	}

	interactive = false
	if interactiveTTYEnabled(&interactive) {
		t.Error("interactiveTTYEnabled(false) = true, want false")
	}
}

func TestShellBootstrapTLSConfig(t *testing.T) {
	script := shellBootstrap()
	expectedVars := []string{
		"SSL_CERT_FILE=/etc/ssl/certs/ca-certificates.crt",
		"NIX_SSL_CERT_FILE=/etc/ssl/certs/ca-certificates.crt",
		"CURL_CA_BUNDLE=/etc/ssl/certs/ca-certificates.crt",
		"REQUESTS_CA_BUNDLE=/etc/ssl/certs/ca-certificates.crt",
	}
	for _, v := range expectedVars {
		if !strings.Contains(script, v) {
			t.Errorf("shellBootstrap() missing %q in %s", v, script)
		}
	}
}
