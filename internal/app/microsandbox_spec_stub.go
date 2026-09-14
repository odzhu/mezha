//go:build !cgo

package app

// These declarations keep configuration parsing available in non-CGO builds.
// Creating a local Microsandbox requires CGO and uses microsandbox_spec.go.
type MicrosandboxSpec struct {
	Image       string               `yaml:"image,omitempty"`
	Dockerfile  string               `yaml:"dockerfile,omitempty"`
	CPUs        uint8                `yaml:"cpus,omitempty"`
	MemoryMiB   uint32               `yaml:"memory_mib,omitempty"`
	Workdir     string               `yaml:"workdir,omitempty"`
	User        string               `yaml:"user,omitempty"`
	Environment map[string]string    `yaml:"env,omitempty"`
	Mounts      []MicrosandboxMount  `yaml:"mounts,omitempty"`
	Volumes     []MicrosandboxVolume `yaml:"volumes,omitempty"`
	Network     MicrosandboxNetwork  `yaml:"network,omitempty"`
	Secrets     []MicrosandboxSecret `yaml:"secrets,omitempty"`
	Scripts     map[string]string    `yaml:"scripts,omitempty"`
}
type MicrosandboxMount struct {
	Source   string `yaml:"source"`
	Target   string `yaml:"target"`
	ReadOnly bool   `yaml:"read_only,omitempty"`
	NoExec   bool   `yaml:"noexec,omitempty"`
	NoSUID   bool   `yaml:"nosuid,omitempty"`
	NoDev    bool   `yaml:"nodev,omitempty"`
}

type MicrosandboxVolume struct {
	Name     string `yaml:"name"`
	Target   string `yaml:"target"`
	Mode     string `yaml:"mode,omitempty"`
	Kind     string `yaml:"kind,omitempty"`
	SizeMiB  uint32 `yaml:"size_mib,omitempty"`
	QuotaMiB uint32 `yaml:"quota_mib,omitempty"`
	ReadOnly bool   `yaml:"read_only,omitempty"`
	NoExec   bool   `yaml:"noexec,omitempty"`
	NoSUID   bool   `yaml:"nosuid,omitempty"`
	NoDev    bool   `yaml:"nodev,omitempty"`
}
type MicrosandboxNetwork struct {
	DefaultEgress  string                    `yaml:"default_egress,omitempty"`
	DefaultIngress string                    `yaml:"default_ingress,omitempty"`
	Strict         bool                      `yaml:"strict,omitempty"`
	Rules          []MicrosandboxNetworkRule `yaml:"rules,omitempty"`
}
type MicrosandboxNetworkRule struct {
	Action      string   `yaml:"action"`
	Direction   string   `yaml:"direction"`
	Destination string   `yaml:"destination"`
	Protocols   []string `yaml:"protocols,omitempty"`
	Ports       []string `yaml:"ports,omitempty"`
}
type MicrosandboxSecret struct {
	EnvVar            string   `yaml:"env"`
	ValueFromEnv      string   `yaml:"value_from_env"`
	AllowHosts        []string `yaml:"allow_hosts,omitempty"`
	AllowHostPatterns []string `yaml:"allow_host_patterns,omitempty"`
	RequireTLS        *bool    `yaml:"require_tls,omitempty"`
}
