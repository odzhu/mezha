//go:build cgo

package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/BurntSushi/toml"
	msb "github.com/superradcompany/microsandbox/sdk/go"
)

func TestMicrosandboxSecrets_AllParameters(t *testing.T) {
	_ = os.Setenv("TEST_KEY_ENV", "secret-from-env-value")
	defer func() { _ = os.Unsetenv("TEST_KEY_ENV") }()

	headersTrue := true
	headersFalse := false
	requireTLSTrue := true
	requireTLSFalse := false

	spec := MicrosandboxSpec{
		Secrets: []MicrosandboxSecret{
			{
				EnvVar:             "KEY1",
				Value:              "direct-secret-value",
				Allow:              []string{"api.example.com", "*.example.com"},
				Passthrough:        []string{"passthrough.example.com"},
				Placeholder:        "$CUSTOM_PLACEHOLDER",
				RequireTLSIdentity: &requireTLSTrue,
				Substitution: &MicrosandboxSecretSubstitution{
					Headers: &headersFalse,
					Query:   true,
					Body:    true,
				},
				ViolationAction: "block-and-terminate",
			},
			{
				Env:               "KEY2",
				ValueFromEnv:      "TEST_KEY_ENV",
				AllowHosts:        []string{"hosts.example.com"},
				AllowHostPatterns: []string{"*.patterns.com"},
				PassthroughHosts:  []string{"pass-hosts.com"},
				RequireTLS:        &requireTLSFalse,
				Substitution: &MicrosandboxSecretSubstitution{
					Headers: &headersTrue,
				},
				ViolationAction: "block-and-log",
			},
		},
	}

	opts, err := spec.runtimeOptions()
	if err != nil {
		t.Fatalf("runtimeOptions() error = %v", err)
	}

	var sbCfg msb.SandboxConfig
	for _, opt := range opts {
		opt(&sbCfg)
	}

	if len(sbCfg.Secrets) != 2 {
		t.Fatalf("expected 2 secrets, got %d", len(sbCfg.Secrets))
	}

	s1 := sbCfg.Secrets[0]
	if s1.EnvVar != "KEY1" {
		t.Errorf("secret[0] EnvVar: got %q, want KEY1", s1.EnvVar)
	}
	if s1.Value != "direct-secret-value" {
		t.Errorf("secret[0] Value: got %q, want direct-secret-value", s1.Value)
	}
	if len(s1.Allow) != 2 || s1.Allow[0] != "api.example.com" || s1.Allow[1] != "*.example.com" {
		t.Errorf("secret[0] Allow: got %v", s1.Allow)
	}
	if len(s1.Passthrough) != 1 || s1.Passthrough[0] != "passthrough.example.com" {
		t.Errorf("secret[0] Passthrough: got %v", s1.Passthrough)
	}
	if s1.Placeholder != "$CUSTOM_PLACEHOLDER" {
		t.Errorf("secret[0] Placeholder: got %q, want $CUSTOM_PLACEHOLDER", s1.Placeholder)
	}
	if s1.RequireTLSIdentity == nil || !*s1.RequireTLSIdentity {
		t.Errorf("secret[0] RequireTLSIdentity: got %v, want true", s1.RequireTLSIdentity)
	}
	if s1.Substitution.Headers == nil || *s1.Substitution.Headers {
		t.Errorf("secret[0] Substitution.Headers: got %v, want false", s1.Substitution.Headers)
	}
	if !s1.Substitution.Query || !s1.Substitution.Body {
		t.Errorf(
			"secret[0] Substitution: got Query=%v, Body=%v",
			s1.Substitution.Query,
			s1.Substitution.Body,
		)
	}
	if s1.ViolationAction != msb.ViolationActionBlockAndTerminate {
		t.Errorf(
			"secret[0] ViolationAction: got %q, want %q",
			s1.ViolationAction,
			msb.ViolationActionBlockAndTerminate,
		)
	}

	s2 := sbCfg.Secrets[1]
	if s2.EnvVar != "KEY2" {
		t.Errorf("secret[1] EnvVar: got %q, want KEY2", s2.EnvVar)
	}
	if s2.Value != "secret-from-env-value" {
		t.Errorf("secret[1] Value: got %q, want secret-from-env-value", s2.Value)
	}
	if len(s2.Allow) != 2 || s2.Allow[0] != "hosts.example.com" || s2.Allow[1] != "*.patterns.com" {
		t.Errorf("secret[1] Allow: got %v", s2.Allow)
	}
	if len(s2.Passthrough) != 1 || s2.Passthrough[0] != "pass-hosts.com" {
		t.Errorf("secret[1] Passthrough: got %v", s2.Passthrough)
	}
	if s2.RequireTLSIdentity == nil || *s2.RequireTLSIdentity {
		t.Errorf("secret[1] RequireTLSIdentity: got %v, want false", s2.RequireTLSIdentity)
	}
	if s2.ViolationAction != msb.ViolationActionBlockAndLog {
		t.Errorf(
			"secret[1] ViolationAction: got %q, want %q",
			s2.ViolationAction,
			msb.ViolationActionBlockAndLog,
		)
	}
}

func TestMicrosandboxSecrets_Validation(t *testing.T) {
	tests := []struct {
		name    string
		secret  MicrosandboxSecret
		wantErr string
	}{
		{
			name:    "missing env",
			secret:  MicrosandboxSecret{Value: "v", Allow: []string{"example.com"}},
			wantErr: "require env",
		},
		{
			name:    "missing value",
			secret:  MicrosandboxSecret{EnvVar: "K", Allow: []string{"example.com"}},
			wantErr: "requires value or value_from_env",
		},
		{
			name:    "missing allowlist",
			secret:  MicrosandboxSecret{EnvVar: "K", Value: "v"},
			wantErr: "requires an allowlist",
		},
		{
			name: "invalid violation_action",
			secret: MicrosandboxSecret{
				EnvVar:          "K",
				Value:           "v",
				Allow:           []string{"example.com"},
				ViolationAction: "invalid",
			},
			wantErr: "violation_action",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := MicrosandboxSpec{Secrets: []MicrosandboxSecret{tt.secret}}
			_, err := spec.runtimeOptions()
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.wantErr)
			}
		})
	}
}

func TestMicrosandboxNetwork_AllParameters(t *testing.T) {
	strictTrue := true
	rebindTrue := true
	verifyUpstreamTrue := true
	blockQUICTrue := true
	trustCAsTrue := true
	maxTCP := uint(100)
	maxUDP := uint(50)
	dnsTimeout := uint64(5000)

	spec := MicrosandboxSpec{
		Network: MicrosandboxNetwork{
			DefaultEgress:       "deny",
			DefaultIngress:      "allow",
			Strict:              &strictTrue,
			DenyDomains:         []string{"ads.example.com", "tracking.com"},
			DenyDomainSuffixes:  []string{".ads", ".analytics"},
			DNSRebindProtection: &rebindTrue,
			DNS: &MicrosandboxDNS{
				RebindProtection: &rebindTrue,
				Nameservers:      []string{"1.1.1.1:53", "8.8.8.8:53"},
				QueryTimeoutMs:   &dnsTimeout,
			},
			TLS: &MicrosandboxTLS{
				Bypass:           []string{"*.pinned.example.com"},
				VerifyUpstream:   &verifyUpstreamTrue,
				InterceptedPorts: []uint16{443, 8443},
				BlockQUIC:        &blockQUICTrue,
				CACert:           "/path/to/ca.pem",
				CAKey:            "/path/to/ca.key",
				UpstreamCACerts:  []string{"/path/to/extra-ca.pem"},
				ScopedUpstreamCACerts: []MicrosandboxScopedUpstreamCACert{
					{Pattern: "*.corp.internal", Path: "/path/to/corp-ca.pem"},
				},
				ScopedVerifyUpstream: []MicrosandboxScopedVerifyUpstream{
					{Pattern: "selfsigned.local", Verify: false},
				},
			},
			Rules: []MicrosandboxNetworkRule{
				{
					Action:      "allow",
					Direction:   "egress",
					Destination: "10.0.0.0/8",
					Protocol:    "tcp",
					Ports:       []string{"80", "443"},
				},
				{
					Action:      "allow",
					Direction:   "egress",
					Destination: "host",
					Protocols:   []string{"tcp", "udp"},
					Port:        "8000-9000",
				},
				{
					AllowDNS: true,
				},
			},
			IPv4Pool:              "172.20.0.0/16",
			IPv6Pool:              "fd00::/64",
			MaxTCPConnections:     &maxTCP,
			MaxUDPConnections:     &maxUDP,
			SecretViolationAction: "block-and-log",
			TrustHostCAs:          &trustCAsTrue,
			Ports:                 map[string]uint16{"8080": 80},
			PortsUDP:              map[string]uint16{"5353": 53},
			PortBindings: []MicrosandboxPortBinding{
				{
					Bind:      "0.0.0.0",
					HostPort:  9000,
					GuestPort: 9000,
					Protocol:  "tcp",
				},
			},
			RateLimiter: &MicrosandboxNetworkRateLimiter{
				Egress: &MicrosandboxRateLimiter{
					Bandwidth: &MicrosandboxTokenBucket{
						Size:         1048576,
						RefillTime:   "1s",
						OneTimeBurst: 524288,
					},
					Ops: &MicrosandboxTokenBucket{
						Size:         1000,
						RefillTimeMs: 100,
					},
				},
				Ingress: &MicrosandboxRateLimiter{
					Bandwidth: &MicrosandboxTokenBucket{
						Size:       2097152,
						RefillTime: "500ms",
					},
				},
			},
		},
	}

	net, err := spec.networkConfig()
	if err != nil {
		t.Fatalf("networkConfig() error = %v", err)
	}
	if net == nil {
		t.Fatal("expected non-nil NetworkConfig")
	}

	if net.DefaultEgress != msb.PolicyActionDeny {
		t.Errorf("DefaultEgress: got %q, want deny", net.DefaultEgress)
	}
	if net.DefaultIngress != msb.PolicyActionAllow {
		t.Errorf("DefaultIngress: got %q, want allow", net.DefaultIngress)
	}
	if net.DisableStrict {
		t.Errorf("DisableStrict: got true, want false (strict mode)")
	}
	if len(net.DenyDomains) != 2 || net.DenyDomains[0] != "ads.example.com" {
		t.Errorf("DenyDomains: got %v", net.DenyDomains)
	}
	if len(net.DenyDomainSuffixes) != 2 || net.DenyDomainSuffixes[0] != ".ads" {
		t.Errorf("DenyDomainSuffixes: got %v", net.DenyDomainSuffixes)
	}
	if net.DNS == nil || len(net.DNS.Nameservers) != 2 || net.DNS.Nameservers[0] != "1.1.1.1:53" {
		t.Errorf("DNS: got %+v", net.DNS)
	}
	if net.TLS == nil || len(net.TLS.Bypass) != 1 || net.TLS.Bypass[0] != "*.pinned.example.com" {
		t.Errorf("TLS: got %+v", net.TLS)
	}
	if len(net.TLS.InterceptedPorts) != 2 || net.TLS.InterceptedPorts[0] != 443 {
		t.Errorf("TLS.InterceptedPorts: got %v", net.TLS.InterceptedPorts)
	}
	if len(net.TLS.ScopedUpstreamCACerts) != 1 ||
		net.TLS.ScopedUpstreamCACerts[0].Pattern != "*.corp.internal" {
		t.Errorf("TLS.ScopedUpstreamCACerts: got %v", net.TLS.ScopedUpstreamCACerts)
	}
	if len(net.TLS.ScopedVerifyUpstream) != 1 || net.TLS.ScopedVerifyUpstream[0].Verify {
		t.Errorf("TLS.ScopedVerifyUpstream: got %v", net.TLS.ScopedVerifyUpstream)
	}
	if len(net.Rules) != 3 {
		t.Fatalf("expected 3 rules, got %d", len(net.Rules))
	}
	if net.Rules[0].Destination != "10.0.0.0/8" || net.Rules[0].Protocol != msb.PolicyProtocolTCP {
		t.Errorf("Rules[0]: got %+v", net.Rules[0])
	}
	if net.Rules[2].Destination != "host" || net.Rules[2].Port != "53" {
		t.Errorf("Rules[2] (AllowDNS): got %+v", net.Rules[2])
	}
	if net.IPv4Pool != "172.20.0.0/16" || net.IPv6Pool != "fd00::/64" {
		t.Errorf("IP pools: ipv4=%q, ipv6=%q", net.IPv4Pool, net.IPv6Pool)
	}
	if net.MaxTCPConnections == nil || *net.MaxTCPConnections != 100 {
		t.Errorf("MaxTCPConnections: got %v", net.MaxTCPConnections)
	}
	if net.MaxUDPConnections == nil || *net.MaxUDPConnections != 50 {
		t.Errorf("MaxUDPConnections: got %v", net.MaxUDPConnections)
	}
	if net.SecretViolationAction != msb.ViolationActionBlockAndLog {
		t.Errorf("SecretViolationAction: got %q", net.SecretViolationAction)
	}
	if net.TrustHostCAs == nil || !*net.TrustHostCAs {
		t.Errorf("TrustHostCAs: got %v", net.TrustHostCAs)
	}
	if net.Ports[8080] != 80 {
		t.Errorf("Ports[8080]: got %d", net.Ports[8080])
	}
	if len(net.PortBindings) != 1 || net.PortBindings[0].HostPort != 9000 {
		t.Errorf("PortBindings: got %+v", net.PortBindings)
	}
	if net.RateLimiter == nil || net.RateLimiter.Egress == nil ||
		net.RateLimiter.Egress.Bandwidth == nil {
		t.Fatalf("RateLimiter.Egress.Bandwidth: got %+v", net.RateLimiter)
	}
	if net.RateLimiter.Egress.Bandwidth.RefillTime != time.Second {
		t.Errorf(
			"RateLimiter.Egress.Bandwidth.RefillTime: got %v, want 1s",
			net.RateLimiter.Egress.Bandwidth.RefillTime,
		)
	}
	if net.RateLimiter.Egress.Ops.RefillTime != 100*time.Millisecond {
		t.Errorf(
			"RateLimiter.Egress.Ops.RefillTime: got %v, want 100ms",
			net.RateLimiter.Egress.Ops.RefillTime,
		)
	}
}

func TestMicrosandboxNetwork_PolicyAndProfiles(t *testing.T) {
	specNone := MicrosandboxSpec{
		Network: MicrosandboxNetwork{
			Policy: "none",
		},
	}
	netNone, err := specNone.networkConfig()
	if err != nil {
		t.Fatalf("policy=none error = %v", err)
	}
	if netNone.DefaultEgress != msb.PolicyActionDeny ||
		netNone.DefaultIngress != msb.PolicyActionDeny {
		t.Errorf(
			"policy=none: default_egress=%q, default_ingress=%q",
			netNone.DefaultEgress,
			netNone.DefaultIngress,
		)
	}

	specAllowAll := MicrosandboxSpec{
		Network: MicrosandboxNetwork{
			Policy: "allow-all",
		},
	}
	netAllowAll, err := specAllowAll.networkConfig()
	if err != nil {
		t.Fatalf("policy=allow-all error = %v", err)
	}
	if netAllowAll.DefaultEgress != msb.PolicyActionAllow ||
		netAllowAll.DefaultIngress != msb.PolicyActionAllow {
		t.Errorf(
			"policy=allow-all: default_egress=%q, default_ingress=%q",
			netAllowAll.DefaultEgress,
			netAllowAll.DefaultIngress,
		)
	}

	specProfiles := MicrosandboxSpec{
		Network: MicrosandboxNetwork{
			Profiles: []string{"public"},
		},
	}
	netProfiles, err := specProfiles.networkConfig()
	if err != nil {
		t.Fatalf("profiles=[public] error = %v", err)
	}
	if netProfiles.DefaultEgress != msb.PolicyActionDeny ||
		netProfiles.DefaultIngress != msb.PolicyActionAllow {
		t.Errorf(
			"profiles=[public]: default_egress=%q, default_ingress=%q",
			netProfiles.DefaultEgress,
			netProfiles.DefaultIngress,
		)
	}
	if len(netProfiles.Rules) != 2 {
		t.Fatalf(
			"expected 2 rules from public profile (gateway DNS + public egress), got %d",
			len(netProfiles.Rules),
		)
	}
}

func TestTOMLDecoding_KitchenSink(t *testing.T) {
	tomlData := `
[microsandbox]
memory_mib = 2048

[[microsandbox.secrets]]
env = "API_KEY"
value = "secret123"
allow = ["api.example.com"]
passthrough = ["internal.corp"]
placeholder = "REDACTED"
require_tls = true
violation_action = "block-and-log"
[microsandbox.secrets.substitution]
headers = false
body = true
query = true

[microsandbox.network]
default_egress = "deny"
default_ingress = "allow"
strict = true
deny_domains = ["blocked.com"]
deny_domain_suffixes = [".malware"]
ipv4_pool = "172.16.0.0/12"
ipv6_pool = "fd42:6d73:62::/48"
max_tcp_connections = 500
max_udp_connections = 250
secret_violation_action = "block"
trust_host_cas = true

[microsandbox.network.dns]
rebind_protection = true
nameservers = ["1.1.1.1:53"]
query_timeout_ms = 3000

[microsandbox.network.tls]
bypass = ["*.amazonaws.com"]
verify_upstream = true
intercepted_ports = [443, 8443]
block_quic = true
ca_cert = "certs/ca.pem"
ca_key = "certs/ca.key"
upstream_ca_certs = ["certs/upstream.pem"]

[[microsandbox.network.tls.scoped_upstream_ca_certs]]
pattern = "*.corp.internal"
path = "certs/corp.pem"

[[microsandbox.network.tls.scoped_verify_upstream]]
pattern = "untrusted.internal"
verify = false

[microsandbox.network.ports]
8080 = 80

[microsandbox.network.ports_udp]
5353 = 53

[[microsandbox.network.port_bindings]]
bind = "0.0.0.0"
host_port = 3000
guest_port = 3000
protocol = "tcp"

[[microsandbox.network.rules]]
action = "allow"
direction = "egress"
destination = "public"
protocols = ["tcp"]
ports = ["80", "443"]

[microsandbox.network.rate_limiter.egress.bandwidth]
size = 1048576
refill_time = "1s"
one_time_burst = 524288

[microsandbox.network.rate_limiter.egress.ops]
size = 500
refill_time_ms = 200
`

	var cfg MezhaConfig
	if _, err := toml.Decode(tomlData, &cfg); err != nil {
		t.Fatalf("decode toml error = %v", err)
	}

	if cfg.Microsandbox == nil {
		t.Fatal("cfg.Microsandbox is nil")
	}

	baseDir := "/tmp/test-project"
	resolveConfigPaths(&cfg, baseDir)

	tls := cfg.Microsandbox.Network.TLS
	if tls.CACert != filepath.Join(baseDir, "certs/ca.pem") {
		t.Errorf("CACert resolved path: got %q", tls.CACert)
	}
	if tls.CAKey != filepath.Join(baseDir, "certs/ca.key") {
		t.Errorf("CAKey resolved path: got %q", tls.CAKey)
	}
	if tls.UpstreamCACerts[0] != filepath.Join(baseDir, "certs/upstream.pem") {
		t.Errorf("UpstreamCACerts[0] resolved path: got %q", tls.UpstreamCACerts[0])
	}
	if tls.ScopedUpstreamCACerts[0].Path != filepath.Join(baseDir, "certs/corp.pem") {
		t.Errorf(
			"ScopedUpstreamCACerts[0] resolved path: got %q",
			tls.ScopedUpstreamCACerts[0].Path,
		)
	}

	net, err := cfg.Microsandbox.networkConfig()
	if err != nil {
		t.Fatalf("networkConfig() error = %v", err)
	}
	if net == nil {
		t.Fatal("expected non-nil NetworkConfig")
	}

	if net.Ports[8080] != 80 {
		t.Errorf("Ports[8080]: got %d", net.Ports[8080])
	}
	if net.RateLimiter.Egress.Bandwidth.Size != 1048576 {
		t.Errorf("RateLimiter size: got %d", net.RateLimiter.Egress.Bandwidth.Size)
	}
}

func TestPortBindingStringFormat(t *testing.T) {
	var pb1 MicrosandboxPortBinding
	if err := pb1.parseString("8080:80"); err != nil {
		t.Fatalf("parseString(8080:80) error = %v", err)
	}
	if pb1.HostPort != 8080 || pb1.GuestPort != 80 || pb1.Protocol != "tcp" {
		t.Errorf("pb1: %+v", pb1)
	}

	var pb2 MicrosandboxPortBinding
	if err := pb2.parseString("0.0.0.0:8080:80/udp"); err != nil {
		t.Fatalf("parseString(0.0.0.0:8080:80/udp) error = %v", err)
	}
	if pb2.Bind != "0.0.0.0" || pb2.HostPort != 8080 || pb2.GuestPort != 80 ||
		pb2.Protocol != "udp" {
		t.Errorf("pb2: %+v", pb2)
	}
}
