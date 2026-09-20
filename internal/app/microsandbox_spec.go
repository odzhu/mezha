//go:build cgo

package app

import (
	"fmt"
	"os"
	"path/filepath"

	msb "github.com/superradcompany/microsandbox/sdk/go"
)

// MicrosandboxSpec is Mezha's local Microsandbox declaration. It intentionally
// follows Microsandbox's resource model. Relative bind paths are resolved
// from the declaring mezha.yaml.
type MicrosandboxSpec struct {
	CPUs        uint8                `yaml:"cpus,omitempty"`
	MemoryMiB   uint32               `yaml:"memory_mib,omitempty"`
	Workdir     string               `yaml:"workdir,omitempty"`
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

// MicrosandboxVolume declares a named persistent volume mounted in the sandbox.
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

// sandboxOptions converts a declarative Mezha sandbox to native SDK options.
// Named volumes are isolated by sandbox name. Secrets deliberately source values
// from the host environment at creation; their guest values remain placeholders.
func (s MicrosandboxSpec) sandboxOptions(
	configDir, sandboxName string,
) ([]msb.SandboxOption, error) {
	image := defaultDevenvImage
	opts := []msb.SandboxOption{msb.WithImage(image), msb.WithDetached()}
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
	environment := make(map[string]string, len(s.Environment)+1)
	for name, value := range s.Environment {
		environment[name] = value
	}
	if _, exists := environment["GOPATH"]; !exists {
		environment["GOPATH"] = "/sandbox/go"
	}
	opts = append(opts, msb.WithEnv(environment))
	if len(s.Scripts) != 0 {
		opts = append(opts, msb.WithScripts(s.Scripts))
	}

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

	network, err := s.networkConfig()
	if err != nil {
		return nil, err
	}
	if network != nil {
		opts = append(opts, msb.WithNetwork(network))
	}

	secrets := make([]msb.SecretEntry, 0, len(s.Secrets))
	for _, secret := range s.Secrets {
		if secret.EnvVar == "" || secret.ValueFromEnv == "" {
			return nil, fmt.Errorf("sandbox.secrets require env and value_from_env")
		}
		if len(secret.AllowHosts) == 0 && len(secret.AllowHostPatterns) == 0 {
			return nil, fmt.Errorf("sandbox secret %q requires an allowlist", secret.EnvVar)
		}
		secrets = append(
			secrets,
			msb.Secret.Env(
				secret.EnvVar,
				os.Getenv(secret.ValueFromEnv),
				msb.SecretEnvOptions{
					AllowHosts:        secret.AllowHosts,
					AllowHostPatterns: secret.AllowHostPatterns,
					RequireTLS:        secret.RequireTLS,
				},
			),
		)
	}
	if len(secrets) != 0 {
		opts = append(opts, msb.WithSecrets(secrets...))
	}
	return opts, nil
}

func (s MicrosandboxSpec) networkConfig() (*msb.NetworkConfig, error) {
	if s.Network.DefaultEgress == "" && s.Network.DefaultIngress == "" && !s.Network.Strict &&
		len(s.Network.Rules) == 0 {
		return nil, nil
	}
	n := &msb.NetworkConfig{Strict: s.Network.Strict}
	var err error
	if n.DefaultEgress, err = microsandboxAction(s.Network.DefaultEgress); err != nil {
		return nil, fmt.Errorf("sandbox.network.default_egress: %w", err)
	}
	if n.DefaultIngress, err = microsandboxAction(s.Network.DefaultIngress); err != nil {
		return nil, fmt.Errorf("sandbox.network.default_ingress: %w", err)
	}
	for _, rule := range s.Network.Rules {
		a, err := microsandboxAction(rule.Action)
		if err != nil {
			return nil, fmt.Errorf("sandbox.network.rules action: %w", err)
		}
		d := msb.PolicyDirection(rule.Direction)
		if d != msb.PolicyDirectionEgress && d != msb.PolicyDirectionIngress &&
			d != msb.PolicyDirectionAny {
			return nil, fmt.Errorf("sandbox.network.rules has invalid direction %q", rule.Direction)
		}
		protocols := make([]msb.PolicyProtocol, len(rule.Protocols))
		for i, p := range rule.Protocols {
			protocols[i] = msb.PolicyProtocol(p)
		}
		n.Rules = append(
			n.Rules,
			msb.PolicyRule{
				Action:      a,
				Direction:   d,
				Destination: rule.Destination,
				Protocols:   protocols,
				Ports:       rule.Ports,
			},
		)
	}
	return n, nil
}

func persistentSymlinkTarget(target string) bool {
	switch filepath.Clean(target) {
	case "/nix/store", "/root", "/root/.cache/go-build", "/root/.cache/nix",
		"/sandbox", "/sandbox/.devenv", "/var/lib/docker", "/var/lib/rancher/k3s":
		return true
	default:
		return false
	}
}

func microsandboxAction(value string) (msb.PolicyAction, error) {
	if value == "" {
		return "", nil
	}
	if value != string(msb.PolicyActionAllow) && value != string(msb.PolicyActionDeny) {
		return "", fmt.Errorf("must be allow or deny, got %q", value)
	}
	return msb.PolicyAction(value), nil
}
