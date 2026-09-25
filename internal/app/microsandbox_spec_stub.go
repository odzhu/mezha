//go:build !cgo

package app

import (
	"fmt"
	"strconv"
	"strings"
)

// These declarations keep configuration parsing available in non-CGO builds.
// Creating a local Microsandbox requires CGO and uses microsandbox_spec.go.
type MicrosandboxSpec struct {
	CPUs         uint8                     `toml:"cpus,omitempty"`
	MemoryMiB    uint32                    `toml:"memory_mib,omitempty"`
	Workdir      string                    `toml:"workdir,omitempty"`
	Mounts       []MicrosandboxMount       `toml:"mounts,omitempty"`
	Volumes      []MicrosandboxVolume      `toml:"volumes,omitempty"`
	Network      MicrosandboxNetwork       `toml:"network,omitempty"`
	Secrets      []MicrosandboxSecret      `toml:"secrets,omitempty"`
	Scripts      map[string]string         `toml:"scripts,omitempty"`
	Ports        map[string]uint16         `toml:"ports,omitempty"`
	PortsUDP     map[string]uint16         `toml:"ports_udp,omitempty"`
	PortBindings []MicrosandboxPortBinding `toml:"port_bindings,omitempty"`
}

type MicrosandboxMount struct {
	Source   string `toml:"source"`
	Target   string `toml:"target"`
	ReadOnly bool   `toml:"read_only,omitempty"`
	NoExec   bool   `toml:"noexec,omitempty"`
	NoSUID   bool   `toml:"nosuid,omitempty"`
	NoDev    bool   `toml:"nodev,omitempty"`
}

type MicrosandboxVolume struct {
	Name     string `toml:"name"`
	Target   string `toml:"target"`
	Mode     string `toml:"mode,omitempty"`
	Kind     string `toml:"kind,omitempty"`
	SizeMiB  uint32 `toml:"size_mib,omitempty"`
	QuotaMiB uint32 `toml:"quota_mib,omitempty"`
	ReadOnly bool   `toml:"read_only,omitempty"`
	NoExec   bool   `toml:"noexec,omitempty"`
	NoSUID   bool   `toml:"nosuid,omitempty"`
	NoDev    bool   `toml:"nodev,omitempty"`
}

type MicrosandboxNetwork struct {
	Policy                string                          `toml:"policy,omitempty"`
	Profiles              []string                        `toml:"profiles,omitempty"`
	DefaultEgress         string                          `toml:"default_egress,omitempty"`
	DefaultIngress        string                          `toml:"default_ingress,omitempty"`
	Strict                *bool                           `toml:"strict,omitempty"`
	DisableStrict         *bool                           `toml:"disable_strict,omitempty"`
	Rules                 []MicrosandboxNetworkRule       `toml:"rules,omitempty"`
	DenyDomains           []string                        `toml:"deny_domains,omitempty"`
	DenyDomainSuffixes    []string                        `toml:"deny_domain_suffixes,omitempty"`
	DNS                   *MicrosandboxDNS                `toml:"dns,omitempty"`
	DNSRebindProtection   *bool                           `toml:"dns_rebind_protection,omitempty"`
	TLS                   *MicrosandboxTLS                `toml:"tls,omitempty"`
	Ports                 map[string]uint16               `toml:"ports,omitempty"`
	PortsUDP              map[string]uint16               `toml:"ports_udp,omitempty"`
	PortBindings          []MicrosandboxPortBinding       `toml:"port_bindings,omitempty"`
	IPv4Pool              string                          `toml:"ipv4_pool,omitempty"`
	IPv6Pool              string                          `toml:"ipv6_pool,omitempty"`
	MaxConnections        *uint                           `toml:"max_connections,omitempty"`
	MaxTCPConnections     *uint                           `toml:"max_tcp_connections,omitempty"`
	MaxUDPConnections     *uint                           `toml:"max_udp_connections,omitempty"`
	RateLimiter           *MicrosandboxNetworkRateLimiter `toml:"rate_limiter,omitempty"`
	SecretViolationAction string                          `toml:"secret_violation_action,omitempty"`
	TrustHostCAs          *bool                           `toml:"trust_host_cas,omitempty"`
}

type MicrosandboxNetworkRule struct {
	Action      string   `toml:"action,omitempty"`
	Direction   string   `toml:"direction,omitempty"`
	Destination string   `toml:"destination,omitempty"`
	Protocol    string   `toml:"protocol,omitempty"`
	Protocols   []string `toml:"protocols,omitempty"`
	Port        string   `toml:"port,omitempty"`
	Ports       []string `toml:"ports,omitempty"`
	AllowDNS    bool     `toml:"allow_dns,omitempty"`
	DenyDNS     bool     `toml:"deny_dns,omitempty"`
}

type MicrosandboxDNS struct {
	RebindProtection *bool    `toml:"rebind_protection,omitempty"`
	Nameservers      []string `toml:"nameservers,omitempty"`
	QueryTimeoutMs   *uint64  `toml:"query_timeout_ms,omitempty"`
}

type MicrosandboxTLS struct {
	Bypass                []string                           `toml:"bypass,omitempty"`
	VerifyUpstream        *bool                              `toml:"verify_upstream,omitempty"`
	InterceptedPorts      []uint16                           `toml:"intercepted_ports,omitempty"`
	BlockQUIC             *bool                              `toml:"block_quic,omitempty"`
	CACert                string                             `toml:"ca_cert,omitempty"`
	CAKey                 string                             `toml:"ca_key,omitempty"`
	UpstreamCACerts       []string                           `toml:"upstream_ca_certs,omitempty"`
	ScopedUpstreamCACerts []MicrosandboxScopedUpstreamCACert `toml:"scoped_upstream_ca_certs,omitempty"`
	ScopedVerifyUpstream  []MicrosandboxScopedVerifyUpstream `toml:"scoped_verify_upstream,omitempty"`
}

type MicrosandboxScopedUpstreamCACert struct {
	Pattern string `toml:"pattern"`
	Path    string `toml:"path"`
}

type MicrosandboxScopedVerifyUpstream struct {
	Pattern string `toml:"pattern"`
	Verify  bool   `toml:"verify"`
}

type MicrosandboxPortBinding struct {
	Bind      string `toml:"bind,omitempty"`
	HostPort  uint16 `toml:"host_port"`
	GuestPort uint16 `toml:"guest_port"`
	Protocol  string `toml:"protocol,omitempty"`
}

type MicrosandboxNetworkRateLimiter struct {
	Egress  *MicrosandboxRateLimiter `toml:"egress,omitempty"`
	Ingress *MicrosandboxRateLimiter `toml:"ingress,omitempty"`
}

type MicrosandboxRateLimiter struct {
	Bandwidth *MicrosandboxTokenBucket `toml:"bandwidth,omitempty"`
	Ops       *MicrosandboxTokenBucket `toml:"ops,omitempty"`
}

type MicrosandboxTokenBucket struct {
	Size         uint64 `toml:"size"`
	RefillTime   string `toml:"refill_time,omitempty"`
	RefillTimeMs uint64 `toml:"refill_time_ms,omitempty"`
	OneTimeBurst uint64 `toml:"one_time_burst,omitempty"`
}

type MicrosandboxSecret struct {
	EnvVar             string                          `toml:"env,omitempty"`
	Env                string                          `toml:"env_var,omitempty"`
	Value              string                          `toml:"value,omitempty"`
	ValueFromEnv       string                          `toml:"value_from_env,omitempty"`
	Allow              []string                        `toml:"allow,omitempty"`
	AllowHosts         []string                        `toml:"allow_hosts,omitempty"`
	AllowHostPatterns  []string                        `toml:"allow_host_patterns,omitempty"`
	Passthrough        []string                        `toml:"passthrough,omitempty"`
	PassthroughHosts   []string                        `toml:"passthrough_hosts,omitempty"`
	Placeholder        string                          `toml:"placeholder,omitempty"`
	RequireTLS         *bool                           `toml:"require_tls,omitempty"`
	RequireTLSIdentity *bool                           `toml:"require_tls_identity,omitempty"`
	Substitution       *MicrosandboxSecretSubstitution `toml:"substitution,omitempty"`
	ViolationAction    string                          `toml:"violation_action,omitempty"`
}

type MicrosandboxSecretSubstitution struct {
	Headers *bool `toml:"headers,omitempty"`
	Query   bool  `toml:"query,omitempty"`
	Body    bool  `toml:"body,omitempty"`
}

// UnmarshalTOML supports both tables and string lists for substitution locations.
func (s *MicrosandboxSecretSubstitution) UnmarshalTOML(data any) error {
	switch v := data.(type) {
	case map[string]any:
		if h, ok := v["headers"].(bool); ok {
			s.Headers = &h
		}
		if q, ok := v["query"].(bool); ok {
			s.Query = q
		}
		if b, ok := v["body"].(bool); ok {
			s.Body = b
		}
		return nil
	case []any:
		headers := false
		for _, item := range v {
			if str, ok := item.(string); ok {
				switch strings.ToLower(str) {
				case "headers", "header":
					headers = true
				case "query":
					s.Query = true
				case "body":
					s.Body = true
				default:
					return fmt.Errorf("unknown secret substitution location %q", str)
				}
			}
		}
		s.Headers = &headers
		return nil
	default:
		return fmt.Errorf("substitution must be a table or a list of strings")
	}
}

// UnmarshalTOML supports numeric and string durations for token buckets.
func (b *MicrosandboxTokenBucket) UnmarshalTOML(data any) error {
	m, ok := data.(map[string]any)
	if !ok {
		return fmt.Errorf("token bucket must be a table")
	}
	if size, ok := m["size"]; ok {
		s, err := toUint64Stub(size)
		if err != nil {
			return fmt.Errorf("token bucket size: %w", err)
		}
		b.Size = s
	}
	if burst, ok := m["one_time_burst"]; ok {
		s, err := toUint64Stub(burst)
		if err != nil {
			return fmt.Errorf("token bucket one_time_burst: %w", err)
		}
		b.OneTimeBurst = s
	}
	if rt, ok := m["refill_time"]; ok {
		switch v := rt.(type) {
		case string:
			b.RefillTime = v
		case int64:
			b.RefillTimeMs = uint64(v)
		case uint64:
			b.RefillTimeMs = v
		}
	}
	if rtMs, ok := m["refill_time_ms"]; ok {
		s, err := toUint64Stub(rtMs)
		if err != nil {
			return fmt.Errorf("token bucket refill_time_ms: %w", err)
		}
		b.RefillTimeMs = s
	}
	return nil
}

// UnmarshalTOML supports both tables and [bind:]host:guest[/proto] strings.
func (pb *MicrosandboxPortBinding) UnmarshalTOML(data any) error {
	switch v := data.(type) {
	case map[string]any:
		if b, ok := v["bind"].(string); ok {
			pb.Bind = b
		}
		if hp, ok := v["host_port"]; ok {
			u, err := toUint16Stub(hp)
			if err != nil {
				return fmt.Errorf("host_port: %w", err)
			}
			pb.HostPort = u
		}
		if gp, ok := v["guest_port"]; ok {
			u, err := toUint16Stub(gp)
			if err != nil {
				return fmt.Errorf("guest_port: %w", err)
			}
			pb.GuestPort = u
		}
		if proto, ok := v["protocol"].(string); ok {
			pb.Protocol = proto
		}
		return nil
	case string:
		return pb.parseString(v)
	default:
		return fmt.Errorf("port binding must be a table or string")
	}
}

func (pb *MicrosandboxPortBinding) parseString(s string) error {
	proto := "tcp"
	if idx := strings.Index(s, "/"); idx != -1 {
		proto = s[idx+1:]
		s = s[:idx]
	}
	pb.Protocol = proto
	parts := strings.Split(s, ":")
	switch len(parts) {
	case 2:
		hp, err := strconv.ParseUint(parts[0], 10, 16)
		if err != nil {
			return fmt.Errorf("invalid host port %q: %w", parts[0], err)
		}
		gp, err := strconv.ParseUint(parts[1], 10, 16)
		if err != nil {
			return fmt.Errorf("invalid guest port %q: %w", parts[1], err)
		}
		pb.HostPort = uint16(hp)
		pb.GuestPort = uint16(gp)
	case 3:
		pb.Bind = parts[0]
		hp, err := strconv.ParseUint(parts[1], 10, 16)
		if err != nil {
			return fmt.Errorf("invalid host port %q: %w", parts[1], err)
		}
		gp, err := strconv.ParseUint(parts[2], 10, 16)
		if err != nil {
			return fmt.Errorf("invalid guest port %q: %w", parts[2], err)
		}
		pb.HostPort = uint16(hp)
		pb.GuestPort = uint16(gp)
	default:
		return fmt.Errorf("invalid port binding format %q, expected [bind:]host:guest[/proto]", s)
	}
	return nil
}

func toUint64Stub(val any) (uint64, error) {
	switch v := val.(type) {
	case int64:
		if v < 0 {
			return 0, fmt.Errorf("must be non-negative, got %d", v)
		}
		return uint64(v), nil
	case uint64:
		return v, nil
	case int:
		if v < 0 {
			return 0, fmt.Errorf("must be non-negative, got %d", v)
		}
		return uint64(v), nil
	case float64:
		if v < 0 {
			return 0, fmt.Errorf("must be non-negative, got %f", v)
		}
		return uint64(v), nil
	default:
		return 0, fmt.Errorf("invalid number %v", val)
	}
}

func toUint16Stub(val any) (uint16, error) {
	u, err := toUint64Stub(val)
	if err != nil {
		return 0, err
	}
	if u > 65535 {
		return 0, fmt.Errorf("must be at most 65535, got %d", u)
	}
	return uint16(u), nil
}
