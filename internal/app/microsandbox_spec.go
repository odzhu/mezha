//go:build cgo

package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	msb "github.com/superradcompany/microsandbox/sdk/go"
)

// MicrosandboxSpec is Mezha's local Microsandbox declaration. It intentionally
// follows Microsandbox's resource model. Relative bind paths are resolved
// from the declaring mezha.toml.
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

// MicrosandboxVolume declares a named persistent volume mounted in the sandbox.
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
		s, err := toUint64(size)
		if err != nil {
			return fmt.Errorf("token bucket size: %w", err)
		}
		b.Size = s
	}
	if burst, ok := m["one_time_burst"]; ok {
		s, err := toUint64(burst)
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
		s, err := toUint64(rtMs)
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
			u, err := toUint16(hp)
			if err != nil {
				return fmt.Errorf("host_port: %w", err)
			}
			pb.HostPort = u
		}
		if gp, ok := v["guest_port"]; ok {
			u, err := toUint16(gp)
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

// sandboxOptions converts a declarative Mezha sandbox to native SDK options.
// Named volumes are isolated by sandbox name. Secrets deliberately source values
// from the host environment at creation; their guest values remain placeholders.
func (s MicrosandboxSpec) sandboxOptions(
	configDir, sandboxName string,
) ([]msb.SandboxOption, error) {
	image := defaultDevenvImage
	opts := []msb.SandboxOption{
		msb.WithImage(image),
		msb.WithDetached(),
		msb.WithPullPolicy(msb.PullPolicyIfMissing),
	}
	if s.CPUs != 0 {
		opts = append(opts, msb.WithCPUs(s.CPUs))
	}
	if s.MemoryMiB != 0 {
		opts = append(opts, msb.WithMemory(s.MemoryMiB))
	}
	if s.Workdir != "" {
		opts = append(opts, msb.WithWorkdir(s.Workdir))
	}
	// The native devenv image declares its user as "1000:100", which the
	// Microsandbox guest-user resolver cannot resolve. Docker and k3s also
	// require root privileges, so always use root's numeric UID.
	opts = append(opts, msb.WithUser("0"))
	runtimeOpts, err := s.runtimeOptions()
	if err != nil {
		return nil, err
	}
	opts = append(opts, runtimeOpts...)

	mounts := make(map[string]msb.MountConfig, len(s.Mounts)+len(s.Volumes))
	for _, mount := range s.Mounts {
		if mount.Source == "" || mount.Target == "" {
			return nil, fmt.Errorf("sandbox.mounts require source and target")
		}
		if !filepath.IsAbs(mount.Source) {
			mount.Source = filepath.Join(configDir, mount.Source)
		}
		mounts[mount.Target] = msb.Mount.Bind(
			mount.Source,
			msb.MountOptions{
				Readonly: mount.ReadOnly,
				Noexec:   mount.NoExec,
				Nosuid:   mount.NoSUID,
				Nodev:    mount.NoDev,
			},
		)
	}
	for _, volume := range s.Volumes {
		if persistentSymlinkTarget(volume.Target) {
			// These paths are directories in the shared /nix volume.
			continue
		}
		if volume.Name == "" || volume.Target == "" {
			return nil, fmt.Errorf("sandbox.volumes require name and target")
		}
		if _, exists := mounts[volume.Target]; exists {
			return nil, fmt.Errorf("sandbox volume and mount share target %q", volume.Target)
		}
		mounts[volume.Target] = msb.Mount.NamedWith(
			sandboxName+"-"+volume.Name,
			msb.MountOptions{
				Readonly: volume.ReadOnly,
				Noexec:   volume.NoExec,
				Nosuid:   volume.NoSUID,
				Nodev:    volume.NoDev,
			},
			msb.NamedVolumeOptions{
				Mode:     volume.Mode,
				Kind:     volume.Kind,
				SizeMiB:  volume.SizeMiB,
				QuotaMiB: volume.QuotaMiB,
			},
		)
	}
	if _, exists := mounts["/nix"]; !exists {
		mounts["/nix"] = msb.Mount.NamedWith(
			sandboxName+"-state",
			msb.MountOptions{},
			msb.NamedVolumeOptions{Mode: "ensure-exists", Kind: "disk", SizeMiB: 51200},
		)
	}
	if len(mounts) != 0 {
		opts = append(opts, msb.WithMounts(mounts))
	}

	return opts, nil
}

// runtimeOptions returns configuration that must be present in every sandbox
// that can execute provisioning commands, including the state-volume bootstrap.
func (s MicrosandboxSpec) runtimeOptions() ([]msb.SandboxOption, error) {
	var opts []msb.SandboxOption
	if len(s.Scripts) != 0 {
		opts = append(opts, msb.WithScripts(s.Scripts))
	}

	network, err := s.networkConfig()
	if err != nil {
		return nil, err
	}
	if network != nil {
		opts = append(opts, msb.WithNetwork(network))
	}

	ports := s.effectivePorts()
	if len(ports) > 0 {
		tcpMap := make(map[uint16]uint16, len(ports))
		for hpStr, gp := range ports {
			hp, err := strconv.ParseUint(hpStr, 10, 16)
			if err != nil {
				return nil, fmt.Errorf("invalid host port %q in sandbox ports: %w", hpStr, err)
			}
			tcpMap[uint16(hp)] = gp
		}
		opts = append(opts, msb.WithPorts(tcpMap))
	}

	portsUDP := s.effectivePortsUDP()
	if len(portsUDP) > 0 {
		udpMap := make(map[uint16]uint16, len(portsUDP))
		for hpStr, gp := range portsUDP {
			hp, err := strconv.ParseUint(hpStr, 10, 16)
			if err != nil {
				return nil, fmt.Errorf("invalid host port %q in sandbox udp ports: %w", hpStr, err)
			}
			udpMap[uint16(hp)] = gp
		}
		opts = append(opts, msb.WithPortsUDP(udpMap))
	}

	portBindings := s.effectivePortBindings()
	if len(portBindings) > 0 {
		bindings := make([]msb.PortBinding, len(portBindings))
		for i, pb := range portBindings {
			proto := msb.PortProtocol(strings.ToLower(pb.Protocol))
			if proto == "" {
				proto = msb.PortProtocolTCP
			} else if proto != msb.PortProtocolTCP && proto != msb.PortProtocolUDP {
				return nil, fmt.Errorf(
					"invalid port binding protocol %q: must be tcp or udp",
					pb.Protocol,
				)
			}
			bindings[i] = msb.PortBinding{
				Bind:      pb.Bind,
				HostPort:  pb.HostPort,
				GuestPort: pb.GuestPort,
				Protocol:  proto,
			}
		}
		opts = append(opts, msb.WithPortBindings(bindings...))
	}

	secrets := make([]msb.SecretEntry, 0, len(s.Secrets))
	for _, secret := range s.Secrets {
		envVar := secret.EnvVar
		if envVar == "" {
			envVar = secret.Env
		}
		if envVar == "" {
			return nil, fmt.Errorf("sandbox.secrets require env")
		}

		val := secret.Value
		if val == "" && secret.ValueFromEnv != "" {
			val = os.Getenv(secret.ValueFromEnv)
		}
		if val == "" && secret.Value == "" && secret.ValueFromEnv == "" {
			return nil, fmt.Errorf("sandbox secret %q requires value or value_from_env", envVar)
		}

		allow := append([]string{}, secret.Allow...)
		allow = append(allow, secret.AllowHosts...)
		allow = append(allow, secret.AllowHostPatterns...)
		if len(allow) == 0 {
			return nil, fmt.Errorf("sandbox secret %q requires an allowlist", envVar)
		}

		passthrough := append([]string{}, secret.Passthrough...)
		passthrough = append(passthrough, secret.PassthroughHosts...)

		requireTLS := secret.RequireTLSIdentity
		if requireTLS == nil {
			requireTLS = secret.RequireTLS
		}

		var sub msb.SecretSubstitution
		if secret.Substitution != nil {
			sub = msb.SecretSubstitution{
				Headers: secret.Substitution.Headers,
				Query:   secret.Substitution.Query,
				Body:    secret.Substitution.Body,
			}
		}

		var violationAction msb.ViolationAction
		if secret.ViolationAction != "" {
			va, err := microsandboxViolationAction(secret.ViolationAction)
			if err != nil {
				return nil, fmt.Errorf("sandbox secret %q violation_action: %w", envVar, err)
			}
			violationAction = va
		}

		secrets = append(secrets, msb.Secret.Env(
			envVar,
			val,
			msb.SecretEnvOptions{
				Allow:              allow,
				Passthrough:        passthrough,
				Placeholder:        secret.Placeholder,
				RequireTLSIdentity: requireTLS,
				Substitution:       sub,
				ViolationAction:    violationAction,
			},
		))
	}
	if len(secrets) != 0 {
		opts = append(opts, msb.WithSecrets(secrets...))
	}
	return opts, nil
}

func (s MicrosandboxSpec) networkConfig() (*msb.NetworkConfig, error) {
	if !s.Network.isConfigured() && len(s.Ports) == 0 && len(s.PortBindings) == 0 {
		return nil, nil
	}

	var n *msb.NetworkConfig
	if len(s.Network.Profiles) > 0 {
		profiles := make([]msb.NetworkProfile, len(s.Network.Profiles))
		for i, p := range s.Network.Profiles {
			profiles[i] = msb.NetworkProfile(strings.ToLower(p))
		}
		cfg, err := msb.NetworkPolicy.FromProfilesChecked(profiles...)
		if err != nil {
			return nil, fmt.Errorf("sandbox.network.profiles: %w", err)
		}
		n = cfg
	} else if s.Network.Policy != "" {
		switch strings.ToLower(s.Network.Policy) {
		case "none":
			n = msb.NetworkPolicy.None()
		case "allow-all", "allow_all", "all":
			n = msb.NetworkPolicy.AllowAll()
		default:
			return nil, fmt.Errorf(
				"invalid sandbox.network.policy %q: must be none or allow-all",
				s.Network.Policy,
			)
		}
	} else {
		n = &msb.NetworkConfig{}
	}

	if s.Network.DisableStrict != nil {
		n.DisableStrict = *s.Network.DisableStrict
	} else if s.Network.Strict != nil {
		n.DisableStrict = !*s.Network.Strict
	}

	if s.Network.DefaultEgress != "" {
		action, err := microsandboxAction(s.Network.DefaultEgress)
		if err != nil {
			return nil, fmt.Errorf("sandbox.network.default_egress: %w", err)
		}
		n.DefaultEgress = action
	}
	if s.Network.DefaultIngress != "" {
		action, err := microsandboxAction(s.Network.DefaultIngress)
		if err != nil {
			return nil, fmt.Errorf("sandbox.network.default_ingress: %w", err)
		}
		n.DefaultIngress = action
	}

	for _, rule := range s.Network.Rules {
		pr, err := rule.toSDK()
		if err != nil {
			return nil, fmt.Errorf("sandbox.network.rules: %w", err)
		}
		n.Rules = append(n.Rules, pr)
	}

	if len(s.Network.DenyDomains) > 0 {
		n.DenyDomains = append(n.DenyDomains, s.Network.DenyDomains...)
	}
	if len(s.Network.DenyDomainSuffixes) > 0 {
		n.DenyDomainSuffixes = append(n.DenyDomainSuffixes, s.Network.DenyDomainSuffixes...)
	}

	if s.Network.DNS != nil {
		n.DNS = &msb.DNSConfig{
			RebindProtection: s.Network.DNS.RebindProtection,
			Nameservers:      append([]string(nil), s.Network.DNS.Nameservers...),
			QueryTimeoutMs:   s.Network.DNS.QueryTimeoutMs,
		}
	}
	if s.Network.DNSRebindProtection != nil {
		n.DNSRebindProtection = s.Network.DNSRebindProtection
	}

	if s.Network.TLS != nil {
		tlsCfg := &msb.TLSConfig{
			Bypass:           append([]string(nil), s.Network.TLS.Bypass...),
			VerifyUpstream:   s.Network.TLS.VerifyUpstream,
			InterceptedPorts: append([]uint16(nil), s.Network.TLS.InterceptedPorts...),
			BlockQUIC:        s.Network.TLS.BlockQUIC,
			CACert:           s.Network.TLS.CACert,
			CAKey:            s.Network.TLS.CAKey,
			UpstreamCACerts:  append([]string(nil), s.Network.TLS.UpstreamCACerts...),
		}
		if len(s.Network.TLS.ScopedUpstreamCACerts) > 0 {
			tlsCfg.ScopedUpstreamCACerts = make(
				[]msb.ScopedUpstreamCACert,
				len(s.Network.TLS.ScopedUpstreamCACerts),
			)
			for i, sc := range s.Network.TLS.ScopedUpstreamCACerts {
				tlsCfg.ScopedUpstreamCACerts[i] = msb.ScopedUpstreamCACert{
					Pattern: sc.Pattern,
					Path:    sc.Path,
				}
			}
		}
		if len(s.Network.TLS.ScopedVerifyUpstream) > 0 {
			tlsCfg.ScopedVerifyUpstream = make(
				[]msb.ScopedVerifyUpstream,
				len(s.Network.TLS.ScopedVerifyUpstream),
			)
			for i, sv := range s.Network.TLS.ScopedVerifyUpstream {
				tlsCfg.ScopedVerifyUpstream[i] = msb.ScopedVerifyUpstream{
					Pattern: sv.Pattern,
					Verify:  sv.Verify,
				}
			}
		}
		n.TLS = tlsCfg
	}

	if s.Network.IPv4Pool != "" {
		n.IPv4Pool = s.Network.IPv4Pool
	}
	if s.Network.IPv6Pool != "" {
		n.IPv6Pool = s.Network.IPv6Pool
	}

	if s.Network.MaxConnections != nil {
		n.MaxConnections = s.Network.MaxConnections
	}
	if s.Network.MaxTCPConnections != nil {
		n.MaxTCPConnections = s.Network.MaxTCPConnections
	}
	if s.Network.MaxUDPConnections != nil {
		n.MaxUDPConnections = s.Network.MaxUDPConnections
	}

	if s.Network.RateLimiter != nil {
		rl, err := s.Network.RateLimiter.toSDK()
		if err != nil {
			return nil, fmt.Errorf("sandbox.network.rate_limiter: %w", err)
		}
		n.RateLimiter = rl
	}

	if s.Network.SecretViolationAction != "" {
		va, err := microsandboxViolationAction(s.Network.SecretViolationAction)
		if err != nil {
			return nil, fmt.Errorf("sandbox.network.secret_violation_action: %w", err)
		}
		n.SecretViolationAction = va
	}

	if s.Network.TrustHostCAs != nil {
		n.TrustHostCAs = s.Network.TrustHostCAs
	}

	ports := s.effectivePorts()
	if len(ports) > 0 {
		n.Ports = make(map[uint16]uint16, len(ports))
		for hpStr, gp := range ports {
			hp, err := strconv.ParseUint(hpStr, 10, 16)
			if err != nil {
				return nil, fmt.Errorf("invalid host port %q in sandbox ports: %w", hpStr, err)
			}
			n.Ports[uint16(hp)] = gp
		}
	}

	portBindings := s.effectivePortBindings()
	if len(portBindings) > 0 {
		n.PortBindings = make([]msb.PortBinding, len(portBindings))
		for i, pb := range portBindings {
			proto := msb.PortProtocol(strings.ToLower(pb.Protocol))
			if proto == "" {
				proto = msb.PortProtocolTCP
			} else if proto != msb.PortProtocolTCP && proto != msb.PortProtocolUDP {
				return nil, fmt.Errorf(
					"invalid port binding protocol %q: must be tcp or udp",
					pb.Protocol,
				)
			}
			n.PortBindings[i] = msb.PortBinding{
				Bind:      pb.Bind,
				HostPort:  pb.HostPort,
				GuestPort: pb.GuestPort,
				Protocol:  proto,
			}
		}
	}

	return n, nil
}

func (n MicrosandboxNetwork) isConfigured() bool {
	return n.Policy != "" ||
		len(n.Profiles) > 0 ||
		n.DefaultEgress != "" ||
		n.DefaultIngress != "" ||
		n.Strict != nil ||
		n.DisableStrict != nil ||
		len(n.Rules) > 0 ||
		len(n.DenyDomains) > 0 ||
		len(n.DenyDomainSuffixes) > 0 ||
		n.DNS != nil ||
		n.DNSRebindProtection != nil ||
		n.TLS != nil ||
		len(n.Ports) > 0 ||
		len(n.PortBindings) > 0 ||
		n.IPv4Pool != "" ||
		n.IPv6Pool != "" ||
		n.MaxConnections != nil ||
		n.MaxTCPConnections != nil ||
		n.MaxUDPConnections != nil ||
		n.RateLimiter != nil ||
		n.SecretViolationAction != "" ||
		n.TrustHostCAs != nil
}

func (s MicrosandboxSpec) effectivePorts() map[string]uint16 {
	if len(s.Ports) == 0 && len(s.Network.Ports) == 0 {
		return nil
	}
	res := make(map[string]uint16, len(s.Ports)+len(s.Network.Ports))
	for k, v := range s.Network.Ports {
		res[k] = v
	}
	for k, v := range s.Ports {
		res[k] = v
	}
	return res
}

func (s MicrosandboxSpec) effectivePortsUDP() map[string]uint16 {
	if len(s.PortsUDP) == 0 && len(s.Network.PortsUDP) == 0 {
		return nil
	}
	res := make(map[string]uint16, len(s.PortsUDP)+len(s.Network.PortsUDP))
	for k, v := range s.Network.PortsUDP {
		res[k] = v
	}
	for k, v := range s.PortsUDP {
		res[k] = v
	}
	return res
}

func (s MicrosandboxSpec) effectivePortBindings() []MicrosandboxPortBinding {
	if len(s.PortBindings) == 0 && len(s.Network.PortBindings) == 0 {
		return nil
	}
	var res []MicrosandboxPortBinding
	res = append(res, s.Network.PortBindings...)
	res = append(res, s.PortBindings...)
	return res
}

func (r MicrosandboxNetworkRule) toSDK() (msb.PolicyRule, error) {
	if r.AllowDNS || (r.Action == "" && strings.EqualFold(r.Destination, "dns")) ||
		(strings.EqualFold(r.Action, "allow") && strings.EqualFold(r.Destination, "dns")) {
		return msb.Rule.AllowDNS(), nil
	}
	if r.DenyDNS ||
		(strings.EqualFold(r.Action, "deny") && strings.EqualFold(r.Destination, "dns")) {
		return msb.Rule.DenyDNS(), nil
	}

	action, err := microsandboxAction(r.Action)
	if err != nil {
		return msb.PolicyRule{}, fmt.Errorf("action: %w", err)
	}

	var direction msb.PolicyDirection
	switch strings.ToLower(strings.TrimSpace(r.Direction)) {
	case "", "egress", "outbound":
		direction = msb.PolicyDirectionEgress
	case "ingress", "inbound":
		direction = msb.PolicyDirectionIngress
	case "any":
		direction = msb.PolicyDirectionAny
	default:
		return msb.PolicyRule{}, fmt.Errorf(
			"invalid direction %q: must be egress, ingress, or any",
			r.Direction,
		)
	}

	rule := msb.PolicyRule{
		Action:      action,
		Direction:   direction,
		Destination: r.Destination,
		Port:        r.Port,
		Ports:       append([]string(nil), r.Ports...),
	}
	if r.Protocol != "" {
		rule.Protocol = msb.PolicyProtocol(strings.ToLower(r.Protocol))
	}
	if len(r.Protocols) > 0 {
		rule.Protocols = make([]msb.PolicyProtocol, len(r.Protocols))
		for i, p := range r.Protocols {
			rule.Protocols[i] = msb.PolicyProtocol(strings.ToLower(p))
		}
	}
	return rule, nil
}

func (rl *MicrosandboxNetworkRateLimiter) toSDK() (*msb.NetworkRateLimiterConfig, error) {
	if rl == nil {
		return nil, nil
	}
	res := &msb.NetworkRateLimiterConfig{}
	if rl.Egress != nil {
		eg, err := rl.Egress.toSDK()
		if err != nil {
			return nil, fmt.Errorf("egress: %w", err)
		}
		res.Egress = eg
	}
	if rl.Ingress != nil {
		in, err := rl.Ingress.toSDK()
		if err != nil {
			return nil, fmt.Errorf("ingress: %w", err)
		}
		res.Ingress = in
	}
	return res, nil
}

func (r *MicrosandboxRateLimiter) toSDK() (*msb.RateLimiterConfig, error) {
	if r == nil {
		return nil, nil
	}
	res := &msb.RateLimiterConfig{}
	if r.Bandwidth != nil {
		bw, err := r.Bandwidth.toSDK()
		if err != nil {
			return nil, fmt.Errorf("bandwidth: %w", err)
		}
		res.Bandwidth = &bw
	}
	if r.Ops != nil {
		ops, err := r.Ops.toSDK()
		if err != nil {
			return nil, fmt.Errorf("ops: %w", err)
		}
		res.Ops = &ops
	}
	return res, nil
}

func (b MicrosandboxTokenBucket) toSDK() (msb.TokenBucketConfig, error) {
	if b.Size == 0 {
		return msb.TokenBucketConfig{}, fmt.Errorf("token bucket size must be greater than zero")
	}
	cfg := msb.TokenBucketConfig{
		Size:         b.Size,
		OneTimeBurst: b.OneTimeBurst,
	}
	if b.RefillTime != "" {
		d, err := time.ParseDuration(b.RefillTime)
		if err != nil {
			return msb.TokenBucketConfig{}, fmt.Errorf(
				"invalid refill_time %q: %w",
				b.RefillTime,
				err,
			)
		}
		cfg.RefillTime = d
	} else if b.RefillTimeMs != 0 {
		cfg.RefillTime = time.Duration(b.RefillTimeMs) * time.Millisecond
	} else {
		return msb.TokenBucketConfig{}, fmt.Errorf(
			"token bucket requires refill_time or refill_time_ms",
		)
	}
	return cfg, nil
}

func persistentSymlinkTarget(target string) bool {
	switch filepath.Clean(target) {
	case "/nix/store", "/home", "/root", "/root/.cache/go-build", "/root/.cache/nix",
		"/var/lib/docker", "/var/lib/rancher/k3s":
		return true
	default:
		return false
	}
}

func microsandboxAction(value string) (msb.PolicyAction, error) {
	if value == "" {
		return "", nil
	}
	switch strings.ToLower(value) {
	case string(msb.PolicyActionAllow):
		return msb.PolicyActionAllow, nil
	case string(msb.PolicyActionDeny):
		return msb.PolicyActionDeny, nil
	default:
		return "", fmt.Errorf("must be allow or deny, got %q", value)
	}
}

func microsandboxViolationAction(value string) (msb.ViolationAction, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "default":
		return msb.ViolationActionDefault, nil
	case "block":
		return msb.ViolationActionBlock, nil
	case "block-and-log", "block_and_log", "log":
		return msb.ViolationActionBlockAndLog, nil
	case "block-and-terminate", "block_and_terminate", "terminate":
		return msb.ViolationActionBlockAndTerminate, nil
	default:
		return "", fmt.Errorf("must be block, block-and-log, or block-and-terminate, got %q", value)
	}
}

func toUint64(val any) (uint64, error) {
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

func toUint16(val any) (uint16, error) {
	u, err := toUint64(val)
	if err != nil {
		return 0, err
	}
	if u > 65535 {
		return 0, fmt.Errorf("must be at most 65535, got %d", u)
	}
	return uint16(u), nil
}
