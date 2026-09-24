//go:build !cgo

package app

// These declarations keep configuration parsing available in non-CGO builds.
// Creating a local Microsandbox requires CGO and uses microsandbox_spec.go.
type MicrosandboxSpec struct {
	CPUs      uint8                `toml:"cpus,omitempty"`
	MemoryMiB uint32               `toml:"memory_mib,omitempty"`
	Workdir   string               `toml:"workdir,omitempty"`
	Mounts    []MicrosandboxMount  `toml:"mounts,omitempty"`
	Volumes   []MicrosandboxVolume `toml:"volumes,omitempty"`
	Network   MicrosandboxNetwork  `toml:"network,omitempty"`
	Secrets   []MicrosandboxSecret `toml:"secrets,omitempty"`
	Scripts   map[string]string    `toml:"scripts,omitempty"`
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
	DefaultEgress  string                    `toml:"default_egress,omitempty"`
	DefaultIngress string                    `toml:"default_ingress,omitempty"`
	Strict         bool                      `toml:"strict,omitempty"`
	Rules          []MicrosandboxNetworkRule `toml:"rules,omitempty"`
}
type MicrosandboxNetworkRule struct {
	Action      string   `toml:"action"`
	Direction   string   `toml:"direction"`
	Destination string   `toml:"destination"`
	Protocols   []string `toml:"protocols,omitempty"`
	Ports       []string `toml:"ports,omitempty"`
}
type MicrosandboxSecret struct {
	EnvVar            string   `toml:"env"`
	ValueFromEnv      string   `toml:"value_from_env"`
	AllowHosts        []string `toml:"allow_hosts,omitempty"`
	AllowHostPatterns []string `toml:"allow_host_patterns,omitempty"`
	RequireTLS        *bool    `toml:"require_tls,omitempty"`
}
