package app

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
)

const defaultDebianImage = "debian"

type MezhaConfig struct {
	Version      uint32            `toml:"version,omitempty"`
	Microsandbox *MicrosandboxSpec `toml:"microsandbox,omitempty"`
	Sandbox      SandboxConfig     `toml:"sandbox,omitempty"`
	Files        FilesConfig       `toml:"files,omitempty"`
	SecretSpec   SecretSpecConfig  `toml:"secretspec,omitempty"`
}

// SecretSpecConfig configures host-side SecretSpec resolution for Mezha sessions.
type SecretSpecConfig struct {
	Enabled  bool   `toml:"enabled,omitempty"`
	Path     string `toml:"path,omitempty"`
	Provider string `toml:"provider,omitempty"`
	Profile  string `toml:"profile,omitempty"`
	Scope    string `toml:"scope,omitempty"`
	Reason   string `toml:"reason,omitempty"`
}

// FilesConfig describes local paths to add to the sandbox during a session.
type FilesConfig struct {
	Add []FileAdd `toml:"add,omitempty"`
}

// FileAdd specifies a source path on the host and target path in the sandbox.
type FileAdd struct {
	Source string `toml:"source"`
	Target string `toml:"target"`
}

type SandboxConfig struct {
	Name      string `toml:"name,omitempty"`
	RemoteDir string `toml:"remote_dir,omitempty"`
	Recreate  bool   `toml:"recreate,omitempty"`
	// Herdr registers the sandbox and synchronizes local Herdr plugins.
	Herdr bool `toml:"herdr,omitempty"`
	// Upload and Download are retained only for backwards-compatible parsing.
	Upload   *bool `toml:"upload,omitempty"`
	Download *bool `toml:"download,omitempty"`
	// NoLoginShell skips sourcing shell login/profile startup files.
	NoLoginShell bool `toml:"no_login_shell,omitempty"`
}

// HomeConfigDir returns MEZHA_HOME or the default mezha home directory (~/.mezha).
func HomeConfigDir() (string, error) {
	if dir := os.Getenv("MEZHA_HOME"); dir != "" {
		if dir == "~" {
			if home, err := os.UserHomeDir(); err == nil {
				return home, nil
			}
		} else if strings.HasPrefix(dir, "~/") || strings.HasPrefix(dir, `~\`) {
			if home, err := os.UserHomeDir(); err == nil {
				return filepath.Join(home, dir[2:]), nil
			}
		}
		return filepath.Clean(dir), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, ".mezha"), nil
}

// HomeConfigPath returns the home-level configuration path.
func HomeConfigPath() (string, error) {
	dir, err := HomeConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "mezha.toml"), nil
}

// projectConfigDir returns the project configuration directory under homeDir.
func projectConfigDir(homeDir, repoRoot string) string {
	if homeDir == "" || repoRoot == "" {
		return ""
	}
	primaryRepoRoot, _, err := linkedWorktreePrimaryRepoRoot(repoRoot)
	if err != nil {
		primaryRepoRoot = repoRoot
	}
	name := slugify(filepath.Base(primaryRepoRoot))
	if name == "" {
		return ""
	}
	return filepath.Join(homeDir, "projects", name)
}

// DefaultConfig returns a valid default MezhaConfig for repoRoot.
func DefaultConfig(repoRoot string) *MezhaConfig {
	primaryRepoRoot := repoRoot
	if repoRoot != "" {
		if root, _, err := linkedWorktreePrimaryRepoRoot(repoRoot); err == nil && root != "" {
			primaryRepoRoot = root
		}
	}
	var name string
	if primaryRepoRoot != "" {
		name = slugify(filepath.Base(primaryRepoRoot))
	}
	return &MezhaConfig{
		Sandbox: SandboxConfig{
			Name: name,
		},
		Microsandbox: &MicrosandboxSpec{
			MemoryMiB: 4096,
			Volumes: []MicrosandboxVolume{
				{
					Name:    "state",
					Target:  "/nix",
					Mode:    "ensure-exists",
					Kind:    "disk",
					SizeMiB: 51200,
				},
			},
		},
	}
}

// CandidateConfigPaths returns candidate configuration file paths in precedence order (lowest to highest).
func CandidateConfigPaths(repoRoot string) []string {
	var paths []string
	if homeDir, err := HomeConfigDir(); err == nil && homeDir != "" {
		paths = append(paths, filepath.Join(homeDir, "mezha.toml"))
		if projDir := projectConfigDir(homeDir, repoRoot); projDir != "" {
			paths = append(paths, filepath.Join(projDir, "mezha.toml"))
		}
	}
	if repoRoot != "" {
		if primaryRepoRoot, isLinked, err := linkedWorktreePrimaryRepoRoot(
			repoRoot,
		); err == nil && isLinked && primaryRepoRoot != "" &&
			primaryRepoRoot != repoRoot {
			paths = append(paths, filepath.Join(primaryRepoRoot, "mezha.toml"))
		}
		paths = append(paths, filepath.Join(repoRoot, "mezha.toml"))
	}
	seen := make(map[string]bool, len(paths))
	var unique []string
	for _, p := range paths {
		clean := filepath.Clean(p)
		if !seen[clean] {
			seen[clean] = true
			unique = append(unique, clean)
		}
	}
	return unique
}

// LoadConfigLayers loads and merges configuration layers across candidate paths.
func LoadConfigLayers(repoRoot string) (*MezhaConfig, string, []string, error) {
	effective := DefaultConfig(repoRoot)
	candidates := CandidateConfigPaths(repoRoot)
	var loadedPaths []string
	var activeConfigPath string

	for _, path := range candidates {
		layer, found, err := loadConfigFile(path)
		if err != nil {
			return nil, "", nil, err
		}
		if !found {
			continue
		}
		loadedPaths = append(loadedPaths, path)
		activeConfigPath = path
		mergeConfig(effective, layer)
	}

	applyConfigDefaults(effective, repoRoot)

	if len(loadedPaths) == 0 {
		return effective, "", nil, nil
	}
	return effective, activeConfigPath, loadedPaths, nil
}

// LoadConfig loads and merges configurations across candidate layers in precedence order.
func LoadConfig(repoRoot string) (*MezhaConfig, string, error) {
	effective, activeConfigPath, _, err := LoadConfigLayers(repoRoot)
	if err != nil {
		return nil, "", err
	}
	return effective, activeConfigPath, nil
}

// loadConfigFile decodes a TOML file directly into MezhaConfig and resolves paths.
func loadConfigFile(path string) (*MezhaConfig, bool, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("read %s: %w", path, err)
	}

	var cfg MezhaConfig
	meta, err := toml.Decode(string(data), &cfg)
	if err != nil {
		return nil, false, fmt.Errorf("decode %s: %w", path, err)
	}
	if meta.IsDefined("services") {
		return nil, false, fmt.Errorf(
			"services configuration is no longer supported; manage provisioning parameters in devenv.nix",
		)
	}
	if meta.IsDefined("microsandbox", "workdir") {
		return nil, false, fmt.Errorf(
			"microsandbox.workdir is no longer supported",
		)
	}

	expandStructEnv(reflect.ValueOf(&cfg))
	resolveConfigPaths(&cfg, filepath.Dir(path))
	return &cfg, true, nil
}

func decodeConfig(config map[string]any, path string) (*MezhaConfig, string, error) {
	if _, ok := config["services"]; ok {
		return nil, "", fmt.Errorf(
			"services configuration is no longer supported; manage provisioning parameters in devenv.nix",
		)
	}
	if msb, ok := config["microsandbox"].(map[string]any); ok {
		if _, ok := msb["workdir"]; ok {
			return nil, "", fmt.Errorf(
				"microsandbox.workdir is no longer supported",
			)
		}
	}
	var data bytes.Buffer
	if err := toml.NewEncoder(&data).Encode(config); err != nil {
		return nil, "", fmt.Errorf("encode configuration: %w", err)
	}
	var cfg MezhaConfig
	meta, err := toml.Decode(data.String(), &cfg)
	if err != nil {
		return nil, "", fmt.Errorf("decode configuration: %w", err)
	}
	if meta.IsDefined("services") {
		return nil, "", fmt.Errorf(
			"services configuration is no longer supported; manage provisioning parameters in devenv.nix",
		)
	}
	if meta.IsDefined("microsandbox", "workdir") {
		return nil, "", fmt.Errorf(
			"microsandbox.workdir is no longer supported",
		)
	}
	expandStructEnv(reflect.ValueOf(&cfg))
	resolveConfigPaths(&cfg, filepath.Dir(path))
	return &cfg, path, nil
}

func loadConfigMap(path string) (map[string]any, bool, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("read %s: %w", path, err)
	}
	var config map[string]any
	if _, err := toml.Decode(string(data), &config); err != nil {
		return nil, false, fmt.Errorf("decode %s: %w", path, err)
	}
	if config == nil {
		config = make(map[string]any)
	}
	expandConfigEnvValues(config)
	return config, true, nil
}

// expandConfigEnvValues recursively expands environment variables in map values.
func expandConfigEnvValues(value any) {
	switch value := value.(type) {
	case map[string]any:
		for key, child := range value {
			switch child := child.(type) {
			case string:
				value[key] = expandEnvValue(child)
			default:
				expandConfigEnvValues(child)
			}
		}
	case []any:
		for i, child := range value {
			switch child := child.(type) {
			case string:
				value[i] = expandEnvValue(child)
			default:
				expandConfigEnvValues(child)
			}
		}
	case []map[string]any:
		for _, child := range value {
			expandConfigEnvValues(child)
		}
	}
}

func resolveConfigPaths(config *MezhaConfig, baseDir string) {
	config.SecretSpec.Path = resolveConfigPath(baseDir, config.SecretSpec.Path)
	for i := range config.Files.Add {
		config.Files.Add[i].Source = resolveConfigPath(baseDir, config.Files.Add[i].Source)
	}
	if config.Microsandbox != nil {
		for i := range config.Microsandbox.Mounts {
			config.Microsandbox.Mounts[i].Source = resolveConfigPath(
				baseDir,
				config.Microsandbox.Mounts[i].Source,
			)
		}
		if config.Microsandbox.Network.TLS != nil {
			tls := config.Microsandbox.Network.TLS
			tls.CACert = resolveConfigPath(baseDir, tls.CACert)
			tls.CAKey = resolveConfigPath(baseDir, tls.CAKey)
			for i := range tls.UpstreamCACerts {
				tls.UpstreamCACerts[i] = resolveConfigPath(baseDir, tls.UpstreamCACerts[i])
			}
			for i := range tls.ScopedUpstreamCACerts {
				tls.ScopedUpstreamCACerts[i].Path = resolveConfigPath(
					baseDir,
					tls.ScopedUpstreamCACerts[i].Path,
				)
			}
		}
	}
}

func resolveConfigPath(baseDir, value string) string {
	if value == "" {
		return value
	}
	if value == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			return home
		}
	} else if strings.HasPrefix(value, "~/") || strings.HasPrefix(value, `~\`) {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, value[2:])
		}
	}
	if filepath.IsAbs(value) {
		return value
	}
	return filepath.Join(baseDir, value)
}

func DefaultConfigTemplate() string {
	return ProjectDefaultConfigTemplate()
}

func ProjectDefaultConfigTemplate() string {
	return defaultConfigTemplate()
}

func defaultConfigTemplate() string {
	return defaultMezhaToml
}

func expandEnvValue(val string) string {
	if strings.HasPrefix(val, "env://") {
		varName := strings.TrimPrefix(val, "env://")
		return os.Getenv(varName)
	}
	return os.Expand(val, func(key string) string {
		if strings.Contains(key, ":-") {
			parts := strings.SplitN(key, ":-", 2)
			if v := os.Getenv(parts[0]); v != "" {
				return v
			}
			return parts[1]
		}
		return os.Getenv(key)
	})
}

// expandStructEnv traverses struct string fields and expands environment variables.
func expandStructEnv(v reflect.Value) {
	if !v.IsValid() {
		return
	}
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface:
		if !v.IsNil() {
			expandStructEnv(v.Elem())
		}
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			if v.Type().Field(i).PkgPath != "" {
				continue
			}
			expandStructEnv(v.Field(i))
		}
	case reflect.Slice, reflect.Array:
		for i := 0; i < v.Len(); i++ {
			expandStructEnv(v.Index(i))
		}
	case reflect.Map:
		if v.IsNil() {
			return
		}
		for _, key := range v.MapKeys() {
			val := v.MapIndex(key)
			if val.Kind() == reflect.String {
				v.SetMapIndex(key, reflect.ValueOf(expandEnvValue(val.String())))
			}
		}
	case reflect.String:
		if v.CanSet() {
			v.SetString(expandEnvValue(v.String()))
		}
	}
}

func applyConfigDefaults(cfg *MezhaConfig, repoRoot string) {
	if cfg.Sandbox.Name == "" && repoRoot != "" {
		primary := repoRoot
		if root, _, err := linkedWorktreePrimaryRepoRoot(repoRoot); err == nil && root != "" {
			primary = root
		}
		cfg.Sandbox.Name = slugify(filepath.Base(primary))
	}
	if cfg.Microsandbox == nil {
		cfg.Microsandbox = &MicrosandboxSpec{
			MemoryMiB: 4096,
			Volumes: []MicrosandboxVolume{
				{
					Name:    "state",
					Target:  "/nix",
					Mode:    "ensure-exists",
					Kind:    "disk",
					SizeMiB: 51200,
				},
			},
		}
		return
	}
	if cfg.Microsandbox.MemoryMiB == 0 {
		cfg.Microsandbox.MemoryMiB = 4096
	}
	hasNix := false
	for _, v := range cfg.Microsandbox.Volumes {
		if filepath.Clean(v.Target) == "/nix" {
			hasNix = true
			break
		}
	}
	if !hasNix {
		cfg.Microsandbox.Volumes = append(cfg.Microsandbox.Volumes, MicrosandboxVolume{
			Name:    "state",
			Target:  "/nix",
			Mode:    "ensure-exists",
			Kind:    "disk",
			SizeMiB: 51200,
		})
	}
}

func mergeConfig(base, overlay *MezhaConfig) {
	if overlay == nil {
		return
	}
	if overlay.Version != 0 {
		base.Version = overlay.Version
	}
	if overlay.Sandbox.Name != "" {
		base.Sandbox.Name = overlay.Sandbox.Name
	}
	if overlay.Sandbox.RemoteDir != "" {
		base.Sandbox.RemoteDir = overlay.Sandbox.RemoteDir
	}
	if overlay.Sandbox.Recreate {
		base.Sandbox.Recreate = true
	}
	if overlay.Sandbox.Herdr {
		base.Sandbox.Herdr = true
	}
	if overlay.Sandbox.NoLoginShell {
		base.Sandbox.NoLoginShell = true
	}
	if overlay.Sandbox.Upload != nil {
		base.Sandbox.Upload = overlay.Sandbox.Upload
	}
	if overlay.Sandbox.Download != nil {
		base.Sandbox.Download = overlay.Sandbox.Download
	}

	if len(overlay.Files.Add) > 0 {
		base.Files.Add = mergeFilesAdd(base.Files.Add, overlay.Files.Add)
	}

	mergeSecretSpec(&base.SecretSpec, overlay.SecretSpec)

	if overlay.Microsandbox != nil {
		if base.Microsandbox == nil {
			base.Microsandbox = &MicrosandboxSpec{}
		}
		if overlay.Microsandbox.CPUs != 0 {
			base.Microsandbox.CPUs = overlay.Microsandbox.CPUs
		}
		if overlay.Microsandbox.MemoryMiB != 0 {
			base.Microsandbox.MemoryMiB = overlay.Microsandbox.MemoryMiB
		}
		if len(overlay.Microsandbox.Mounts) > 0 {
			base.Microsandbox.Mounts = mergeMounts(
				base.Microsandbox.Mounts,
				overlay.Microsandbox.Mounts,
			)
		}
		if len(overlay.Microsandbox.Volumes) > 0 {
			base.Microsandbox.Volumes = mergeVolumes(
				base.Microsandbox.Volumes,
				overlay.Microsandbox.Volumes,
			)
		}
		if len(overlay.Microsandbox.Secrets) > 0 {
			base.Microsandbox.Secrets = mergeSecrets(
				base.Microsandbox.Secrets,
				overlay.Microsandbox.Secrets,
			)
		}
		if len(overlay.Microsandbox.Scripts) > 0 {
			if base.Microsandbox.Scripts == nil {
				base.Microsandbox.Scripts = make(map[string]string)
			}
			for k, v := range overlay.Microsandbox.Scripts {
				base.Microsandbox.Scripts[k] = v
			}
		}
		if len(overlay.Microsandbox.Ports) > 0 {
			if base.Microsandbox.Ports == nil {
				base.Microsandbox.Ports = make(map[string]uint16)
			}
			for k, v := range overlay.Microsandbox.Ports {
				base.Microsandbox.Ports[k] = v
			}
		}
		if len(overlay.Microsandbox.PortsUDP) > 0 {
			if base.Microsandbox.PortsUDP == nil {
				base.Microsandbox.PortsUDP = make(map[string]uint16)
			}
			for k, v := range overlay.Microsandbox.PortsUDP {
				base.Microsandbox.PortsUDP[k] = v
			}
		}
		if len(overlay.Microsandbox.PortBindings) > 0 {
			base.Microsandbox.PortBindings = mergePortBindings(
				base.Microsandbox.PortBindings,
				overlay.Microsandbox.PortBindings,
			)
		}
		mergeNetwork(&base.Microsandbox.Network, overlay.Microsandbox.Network)
	}
}

func mergeMounts(base, overlay []MicrosandboxMount) []MicrosandboxMount {
	res := append([]MicrosandboxMount(nil), base...)
	for _, o := range overlay {
		replaced := false
		for i, b := range res {
			if filepath.Clean(b.Target) == filepath.Clean(o.Target) {
				res[i] = o
				replaced = true
				break
			}
		}
		if !replaced {
			res = append(res, o)
		}
	}
	return res
}

func mergeVolumes(base, overlay []MicrosandboxVolume) []MicrosandboxVolume {
	res := append([]MicrosandboxVolume(nil), base...)
	for _, o := range overlay {
		replaced := false
		for i, b := range res {
			if filepath.Clean(b.Target) == filepath.Clean(o.Target) {
				res[i] = o
				replaced = true
				break
			}
		}
		if !replaced {
			res = append(res, o)
		}
	}
	return res
}

func secretEnvName(s MicrosandboxSecret) string {
	if s.EnvVar != "" {
		return s.EnvVar
	}
	return s.Env
}

func mergeSecrets(base, overlay []MicrosandboxSecret) []MicrosandboxSecret {
	res := append([]MicrosandboxSecret(nil), base...)
	for _, o := range overlay {
		oEnv := secretEnvName(o)
		replaced := false
		if oEnv != "" {
			for i, b := range res {
				if secretEnvName(b) == oEnv {
					res[i] = o
					replaced = true
					break
				}
			}
		}
		if !replaced {
			res = append(res, o)
		}
	}
	return res
}

func mergeFilesAdd(base, overlay []FileAdd) []FileAdd {
	res := append([]FileAdd(nil), base...)
	for _, o := range overlay {
		replaced := false
		for i, b := range res {
			if filepath.Clean(b.Target) == filepath.Clean(o.Target) {
				res[i] = o
				replaced = true
				break
			}
		}
		if !replaced {
			res = append(res, o)
		}
	}
	return res
}

func mergePortBindings(base, overlay []MicrosandboxPortBinding) []MicrosandboxPortBinding {
	res := append([]MicrosandboxPortBinding(nil), base...)
	for _, o := range overlay {
		oProto := strings.ToLower(o.Protocol)
		if oProto == "" {
			oProto = "tcp"
		}
		replaced := false
		for i, b := range res {
			bProto := strings.ToLower(b.Protocol)
			if bProto == "" {
				bProto = "tcp"
			}
			if b.HostPort == o.HostPort && bProto == oProto {
				res[i] = o
				replaced = true
				break
			}
		}
		if !replaced {
			res = append(res, o)
		}
	}
	return res
}

func unionStrings(base, overlay []string) []string {
	seen := make(map[string]bool, len(base)+len(overlay))
	var res []string
	for _, s := range base {
		if !seen[s] {
			seen[s] = true
			res = append(res, s)
		}
	}
	for _, s := range overlay {
		if !seen[s] {
			seen[s] = true
			res = append(res, s)
		}
	}
	return res
}

func mergeNetwork(base *MicrosandboxNetwork, overlay MicrosandboxNetwork) {
	if overlay.Policy != "" {
		base.Policy = overlay.Policy
	}
	if overlay.DefaultEgress != "" {
		base.DefaultEgress = overlay.DefaultEgress
	}
	if overlay.DefaultIngress != "" {
		base.DefaultIngress = overlay.DefaultIngress
	}
	if overlay.Strict != nil {
		base.Strict = overlay.Strict
	}
	if overlay.DisableStrict != nil {
		base.DisableStrict = overlay.DisableStrict
	}
	if overlay.DNSRebindProtection != nil {
		base.DNSRebindProtection = overlay.DNSRebindProtection
	}
	if overlay.IPv4Pool != "" {
		base.IPv4Pool = overlay.IPv4Pool
	}
	if overlay.IPv6Pool != "" {
		base.IPv6Pool = overlay.IPv6Pool
	}
	if overlay.MaxConnections != nil {
		base.MaxConnections = overlay.MaxConnections
	}
	if overlay.MaxTCPConnections != nil {
		base.MaxTCPConnections = overlay.MaxTCPConnections
	}
	if overlay.MaxUDPConnections != nil {
		base.MaxUDPConnections = overlay.MaxUDPConnections
	}
	if overlay.RateLimiter != nil {
		base.RateLimiter = overlay.RateLimiter
	}
	if overlay.SecretViolationAction != "" {
		base.SecretViolationAction = overlay.SecretViolationAction
	}
	if overlay.TrustHostCAs != nil {
		base.TrustHostCAs = overlay.TrustHostCAs
	}

	base.Profiles = unionStrings(base.Profiles, overlay.Profiles)
	base.DenyDomains = unionStrings(base.DenyDomains, overlay.DenyDomains)
	base.DenyDomainSuffixes = unionStrings(base.DenyDomainSuffixes, overlay.DenyDomainSuffixes)

	base.Rules = append(base.Rules, overlay.Rules...)
	base.PortBindings = mergePortBindings(base.PortBindings, overlay.PortBindings)

	if len(overlay.Ports) > 0 {
		if base.Ports == nil {
			base.Ports = make(map[string]uint16)
		}
		for k, v := range overlay.Ports {
			base.Ports[k] = v
		}
	}
	if len(overlay.PortsUDP) > 0 {
		if base.PortsUDP == nil {
			base.PortsUDP = make(map[string]uint16)
		}
		for k, v := range overlay.PortsUDP {
			base.PortsUDP[k] = v
		}
	}

	if overlay.DNS != nil {
		if base.DNS == nil {
			base.DNS = overlay.DNS
		} else {
			if overlay.DNS.RebindProtection != nil {
				base.DNS.RebindProtection = overlay.DNS.RebindProtection
			}
			if len(overlay.DNS.Nameservers) > 0 {
				base.DNS.Nameservers = unionStrings(base.DNS.Nameservers, overlay.DNS.Nameservers)
			}
			if overlay.DNS.QueryTimeoutMs != nil {
				base.DNS.QueryTimeoutMs = overlay.DNS.QueryTimeoutMs
			}
		}
	}

	if overlay.TLS != nil {
		if base.TLS == nil {
			base.TLS = overlay.TLS
		} else {
			if overlay.TLS.CACert != "" {
				base.TLS.CACert = overlay.TLS.CACert
			}
			if overlay.TLS.CAKey != "" {
				base.TLS.CAKey = overlay.TLS.CAKey
			}
			if overlay.TLS.VerifyUpstream != nil {
				base.TLS.VerifyUpstream = overlay.TLS.VerifyUpstream
			}
			if overlay.TLS.BlockQUIC != nil {
				base.TLS.BlockQUIC = overlay.TLS.BlockQUIC
			}
			if len(overlay.TLS.Bypass) > 0 {
				base.TLS.Bypass = unionStrings(base.TLS.Bypass, overlay.TLS.Bypass)
			}
			if len(overlay.TLS.InterceptedPorts) > 0 {
				base.TLS.InterceptedPorts = overlay.TLS.InterceptedPorts
			}
			if len(overlay.TLS.UpstreamCACerts) > 0 {
				base.TLS.UpstreamCACerts = unionStrings(
					base.TLS.UpstreamCACerts,
					overlay.TLS.UpstreamCACerts,
				)
			}
			if len(overlay.TLS.ScopedUpstreamCACerts) > 0 {
				base.TLS.ScopedUpstreamCACerts = append(
					base.TLS.ScopedUpstreamCACerts,
					overlay.TLS.ScopedUpstreamCACerts...)
			}
			if len(overlay.TLS.ScopedVerifyUpstream) > 0 {
				base.TLS.ScopedVerifyUpstream = append(
					base.TLS.ScopedVerifyUpstream,
					overlay.TLS.ScopedVerifyUpstream...)
			}
		}
	}
}

func mergeSecretSpec(base *SecretSpecConfig, overlay SecretSpecConfig) {
	if overlay.Enabled {
		base.Enabled = true
	}
	if overlay.Path != "" {
		base.Path = overlay.Path
	}
	if overlay.Provider != "" {
		base.Provider = overlay.Provider
	}
	if overlay.Profile != "" {
		base.Profile = overlay.Profile
	}
	if overlay.Scope != "" {
		base.Scope = overlay.Scope
	}
	if overlay.Reason != "" {
		base.Reason = overlay.Reason
	}
}

// Validate performs fail-fast configuration validation.
func (cfg *MezhaConfig) Validate() error {
	if cfg == nil {
		return fmt.Errorf("config is nil")
	}

	if cfg.SecretSpec.Enabled && cfg.SecretSpec.Path != "" {
		if _, err := os.Stat(cfg.SecretSpec.Path); err != nil {
			return fmt.Errorf("secretspec path %q: %w", cfg.SecretSpec.Path, err)
		}
	}

	for _, fa := range cfg.Files.Add {
		if fa.Source == "" {
			return fmt.Errorf("file add source cannot be empty")
		}
		if fa.Target == "" {
			return fmt.Errorf("file add target cannot be empty")
		}
		if _, err := os.Stat(fa.Source); err != nil {
			return fmt.Errorf("file add source %q: %w", fa.Source, err)
		}
	}

	if cfg.Microsandbox == nil {
		return nil
	}

	seenMountTargets := make(map[string]bool)
	for _, m := range cfg.Microsandbox.Mounts {
		if m.Source == "" {
			return fmt.Errorf("mount source cannot be empty")
		}
		if m.Target == "" {
			return fmt.Errorf("mount target cannot be empty")
		}
		if !filepath.IsAbs(m.Target) {
			return fmt.Errorf("mount target %q must be absolute", m.Target)
		}
		if _, err := os.Stat(m.Source); err != nil {
			return fmt.Errorf("mount source %q: %w", m.Source, err)
		}
		cleanTarget := filepath.Clean(m.Target)
		if seenMountTargets[cleanTarget] {
			return fmt.Errorf("duplicate mount target %q", m.Target)
		}
		seenMountTargets[cleanTarget] = true
	}

	seenVolumeTargets := make(map[string]bool)
	for _, v := range cfg.Microsandbox.Volumes {
		if v.Name == "" {
			return fmt.Errorf("volume name cannot be empty")
		}
		if v.Target == "" {
			return fmt.Errorf("volume target cannot be empty")
		}
		if !filepath.IsAbs(v.Target) {
			return fmt.Errorf("volume target %q must be absolute", v.Target)
		}
		switch v.Kind {
		case "", "disk", "tmpfs":
		default:
			return fmt.Errorf("invalid volume kind %q: must be disk or tmpfs", v.Kind)
		}
		switch v.Mode {
		case "", "ensure-exists", "create-new":
		default:
			return fmt.Errorf("invalid volume mode %q: must be ensure-exists or create-new", v.Mode)
		}
		cleanTarget := filepath.Clean(v.Target)
		if seenVolumeTargets[cleanTarget] {
			return fmt.Errorf("duplicate volume target %q", v.Target)
		}
		if seenMountTargets[cleanTarget] {
			return fmt.Errorf("volume target %q conflicts with mount target", v.Target)
		}
		seenVolumeTargets[cleanTarget] = true
	}

	if err := validatePorts(cfg.Microsandbox.Ports, "ports"); err != nil {
		return err
	}
	if err := validatePorts(cfg.Microsandbox.PortsUDP, "ports_udp"); err != nil {
		return err
	}
	if err := validatePorts(cfg.Microsandbox.Network.Ports, "network.ports"); err != nil {
		return err
	}
	if err := validatePorts(cfg.Microsandbox.Network.PortsUDP, "network.ports_udp"); err != nil {
		return err
	}

	seenBindings := make(map[string]bool)
	if err := validatePortBindings(
		cfg.Microsandbox.PortBindings,
		"port_bindings",
		seenBindings,
	); err != nil {
		return err
	}
	if err := validatePortBindings(
		cfg.Microsandbox.Network.PortBindings,
		"network.port_bindings",
		seenBindings,
	); err != nil {
		return err
	}

	for _, rule := range cfg.Microsandbox.Network.Rules {
		switch strings.ToLower(rule.Action) {
		case "", "allow", "deny":
		default:
			return fmt.Errorf(
				"invalid network rule action %q: must be allow, deny, or empty",
				rule.Action,
			)
		}
		switch strings.ToLower(rule.Direction) {
		case "", "egress", "ingress", "any":
		default:
			return fmt.Errorf(
				"invalid network rule direction %q: must be egress, ingress, any, or empty",
				rule.Direction,
			)
		}
		switch strings.ToLower(rule.Protocol) {
		case "", "tcp", "udp", "icmp":
		default:
			return fmt.Errorf(
				"invalid network rule protocol %q: must be tcp, udp, icmp, or empty",
				rule.Protocol,
			)
		}
		for _, p := range rule.Protocols {
			switch strings.ToLower(p) {
			case "", "tcp", "udp", "icmp":
			default:
				return fmt.Errorf(
					"invalid network rule protocol %q: must be tcp, udp, icmp, or empty",
					p,
				)
			}
		}
	}

	return nil
}

func validatePorts(ports map[string]uint16, fieldName string) error {
	for hostStr, guestPort := range ports {
		hp, err := strconv.ParseUint(hostStr, 10, 16)
		if err != nil || hp == 0 {
			return fmt.Errorf("invalid host port %q in %s: must be 1..65535", hostStr, fieldName)
		}
		if guestPort == 0 {
			return fmt.Errorf("invalid guest port %d in %s: must be 1..65535", guestPort, fieldName)
		}
	}
	return nil
}

func validatePortBindings(
	bindings []MicrosandboxPortBinding,
	fieldName string,
	seen map[string]bool,
) error {
	for _, pb := range bindings {
		if pb.HostPort == 0 {
			return fmt.Errorf("invalid host port %d in %s: must be > 0", pb.HostPort, fieldName)
		}
		if pb.GuestPort == 0 {
			return fmt.Errorf("invalid guest port %d in %s: must be > 0", pb.GuestPort, fieldName)
		}
		proto := strings.ToLower(pb.Protocol)
		switch proto {
		case "", "tcp", "udp":
		default:
			return fmt.Errorf(
				"invalid protocol %q in %s: must be tcp, udp, or empty",
				pb.Protocol,
				fieldName,
			)
		}
		if proto == "" {
			proto = "tcp"
		}
		key := fmt.Sprintf("%s/%d", proto, pb.HostPort)
		if seen[key] {
			return fmt.Errorf(
				"duplicate port binding for host port %d/%s in %s",
				pb.HostPort,
				proto,
				fieldName,
			)
		}
		seen[key] = true
	}
	return nil
}

// Redacted returns a copy of the configuration with secrets masked for display.
func (cfg *MezhaConfig) Redacted() *MezhaConfig {
	if cfg == nil {
		return nil
	}
	cp := cloneConfig(cfg)
	if cp.SecretSpec.Profile != "" {
		cp.SecretSpec.Profile = "[REDACTED]"
	}
	if cp.SecretSpec.Scope != "" {
		cp.SecretSpec.Scope = "[REDACTED]"
	}
	if cp.SecretSpec.Reason != "" {
		cp.SecretSpec.Reason = "[REDACTED]"
	}
	if cp.Microsandbox != nil {
		for i := range cp.Microsandbox.Secrets {
			if cp.Microsandbox.Secrets[i].Value != "" {
				cp.Microsandbox.Secrets[i].Value = "[REDACTED]"
			}
		}
		if cp.Microsandbox.Network.TLS != nil {
			if cp.Microsandbox.Network.TLS.CAKey != "" {
				cp.Microsandbox.Network.TLS.CAKey = "[REDACTED]"
			}
		}
	}
	return cp
}

func cloneConfig(cfg *MezhaConfig) *MezhaConfig {
	if cfg == nil {
		return nil
	}
	cp := *cfg
	if cfg.Sandbox.Upload != nil {
		u := *cfg.Sandbox.Upload
		cp.Sandbox.Upload = &u
	}
	if cfg.Sandbox.Download != nil {
		d := *cfg.Sandbox.Download
		cp.Sandbox.Download = &d
	}
	if len(cfg.Files.Add) > 0 {
		cp.Files.Add = append([]FileAdd(nil), cfg.Files.Add...)
	}
	if cfg.Microsandbox != nil {
		msb := *cfg.Microsandbox
		if len(cfg.Microsandbox.Mounts) > 0 {
			msb.Mounts = append([]MicrosandboxMount(nil), cfg.Microsandbox.Mounts...)
		}
		if len(cfg.Microsandbox.Volumes) > 0 {
			msb.Volumes = append([]MicrosandboxVolume(nil), cfg.Microsandbox.Volumes...)
		}
		if len(cfg.Microsandbox.Secrets) > 0 {
			msb.Secrets = append([]MicrosandboxSecret(nil), cfg.Microsandbox.Secrets...)
		}
		if len(cfg.Microsandbox.Scripts) > 0 {
			msb.Scripts = make(map[string]string, len(cfg.Microsandbox.Scripts))
			for k, v := range cfg.Microsandbox.Scripts {
				msb.Scripts[k] = v
			}
		}
		if len(cfg.Microsandbox.Ports) > 0 {
			msb.Ports = make(map[string]uint16, len(cfg.Microsandbox.Ports))
			for k, v := range cfg.Microsandbox.Ports {
				msb.Ports[k] = v
			}
		}
		if len(cfg.Microsandbox.PortsUDP) > 0 {
			msb.PortsUDP = make(map[string]uint16, len(cfg.Microsandbox.PortsUDP))
			for k, v := range cfg.Microsandbox.PortsUDP {
				msb.PortsUDP[k] = v
			}
		}
		if len(cfg.Microsandbox.PortBindings) > 0 {
			msb.PortBindings = append(
				[]MicrosandboxPortBinding(nil),
				cfg.Microsandbox.PortBindings...)
		}

		net := cfg.Microsandbox.Network
		if len(net.Profiles) > 0 {
			net.Profiles = append([]string(nil), net.Profiles...)
		}
		if len(net.Rules) > 0 {
			net.Rules = append([]MicrosandboxNetworkRule(nil), net.Rules...)
		}
		if len(net.DenyDomains) > 0 {
			net.DenyDomains = append([]string(nil), net.DenyDomains...)
		}
		if len(net.DenyDomainSuffixes) > 0 {
			net.DenyDomainSuffixes = append([]string(nil), net.DenyDomainSuffixes...)
		}
		if len(net.Ports) > 0 {
			net.Ports = make(map[string]uint16, len(net.Ports))
			for k, v := range net.Ports {
				net.Ports[k] = v
			}
		}
		if len(net.PortsUDP) > 0 {
			net.PortsUDP = make(map[string]uint16, len(net.PortsUDP))
			for k, v := range net.PortsUDP {
				net.PortsUDP[k] = v
			}
		}
		if len(net.PortBindings) > 0 {
			net.PortBindings = append([]MicrosandboxPortBinding(nil), net.PortBindings...)
		}
		if net.Strict != nil {
			s := *net.Strict
			net.Strict = &s
		}
		if net.DisableStrict != nil {
			d := *net.DisableStrict
			net.DisableStrict = &d
		}
		if net.DNSRebindProtection != nil {
			p := *net.DNSRebindProtection
			net.DNSRebindProtection = &p
		}
		if net.DNS != nil {
			dns := *net.DNS
			if len(net.DNS.Nameservers) > 0 {
				dns.Nameservers = append([]string(nil), net.DNS.Nameservers...)
			}
			net.DNS = &dns
		}
		if net.TLS != nil {
			tls := *net.TLS
			if len(net.TLS.Bypass) > 0 {
				tls.Bypass = append([]string(nil), net.TLS.Bypass...)
			}
			if len(net.TLS.InterceptedPorts) > 0 {
				tls.InterceptedPorts = append([]uint16(nil), net.TLS.InterceptedPorts...)
			}
			if len(net.TLS.UpstreamCACerts) > 0 {
				tls.UpstreamCACerts = append([]string(nil), net.TLS.UpstreamCACerts...)
			}
			if len(net.TLS.ScopedUpstreamCACerts) > 0 {
				tls.ScopedUpstreamCACerts = append(
					[]MicrosandboxScopedUpstreamCACert(nil),
					net.TLS.ScopedUpstreamCACerts...)
			}
			if len(net.TLS.ScopedVerifyUpstream) > 0 {
				tls.ScopedVerifyUpstream = append(
					[]MicrosandboxScopedVerifyUpstream(nil),
					net.TLS.ScopedVerifyUpstream...)
			}
			net.TLS = &tls
		}
		msb.Network = net
		cp.Microsandbox = &msb
	}
	return &cp
}
