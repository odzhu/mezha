package app

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

const defaultDevenvImage = "ghcr.io/cachix/devenv/devenv:latest"

type MezhaConfig struct {
	Version      uint32            `toml:"version,omitempty"`
	Microsandbox *MicrosandboxSpec `toml:"microsandbox,omitempty"`
	Sandbox      SandboxConfig     `toml:"sandbox,omitempty"`
	Files        FilesConfig       `toml:"files,omitempty"`
	Provision    ProvisionConfig   `toml:"provision,omitempty"`
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

// FilesConfig describes local paths to add to the sandbox during a Mezha session.
// Sources are resolved relative to the mezha.toml that defines them, but may
// use ../ or absolute paths. Targets are sandbox paths; relative targets are resolved
// relative to sandbox.remote_dir.
type FilesConfig struct {
	Add []FileAdd `toml:"add,omitempty"`
}

// ProvisionConfig describes initialization applied only to a new Microsandbox.
type ProvisionConfig struct {
	Add []FileAdd      `toml:"add,omitempty"`
	Run []RunDirective `toml:"run,omitempty"`
}

// FileAdd is equivalent to Dockerfile ADD for local files and directories.
// A file target ending in / is treated as a directory; directory sources copy
// their contents into the target directory.
type FileAdd struct {
	Source string `toml:"source"`
	Target string `toml:"target"`
}

// RunDirective is a command executed in the sandbox during a Mezha session.
// TOML uses an inline table with command and shell fields.
type RunDirective struct {
	Command []string `toml:"command"`
	Shell   bool     `toml:"shell"`
}

// UnmarshalTOML accepts explicit command tables. A string command uses shell
// form; an argument array uses exec form.
func (r *RunDirective) UnmarshalTOML(value any) error {
	entry, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf("run directive must be a table with a command")
	}
	switch command := entry["command"].(type) {
	case string:
		if strings.TrimSpace(command) == "" {
			return fmt.Errorf("run directive must not be empty")
		}
		r.Command = []string{command}
		r.Shell = true
	case []any:
		r.Command = make([]string, len(command))
		for i, arg := range command {
			var ok bool
			if r.Command[i], ok = arg.(string); !ok {
				return fmt.Errorf("run directive arguments must be strings")
			}
		}
		if len(r.Command) == 0 {
			return fmt.Errorf("run directive must not be empty")
		}
	default:
		return fmt.Errorf("run directive command must be a string or an argument list")
	}
	return nil
}

type SandboxConfig struct {
	Name      string `toml:"name,omitempty"`
	RemoteDir string `toml:"remote_dir,omitempty"`
	Recreate  bool   `toml:"recreate,omitempty"`
	// Herdr registers the sandbox and synchronizes local Herdr plugins.
	Herdr bool `toml:"herdr,omitempty"`
	// Upload and Download are retained only for backwards-compatible parsing.
	// Repository content is now synchronized exclusively through Git.
	Upload        *bool  `toml:"upload,omitempty"`
	Download      *bool  `toml:"download,omitempty"`
	Editor        string `toml:"editor,omitempty"`
	PolicyAdvisor *bool  `toml:"policy_advisor,omitempty"`
	Advisor       *bool  `toml:"advisor,omitempty"`
	// NoLoginShell skips sourcing shell login/profile startup files
	// (bash -lc) when running the requested command/session in Mezha.
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

// LoadConfig loads the project configuration when present; otherwise it loads
// the global home configuration.
func LoadConfig(repoRoot string) (*MezhaConfig, string, error) {
	var paths []string
	if repoRoot != "" {
		paths = append(paths, filepath.Join(repoRoot, "mezha.toml"))
	}
	if homeDir, err := HomeConfigDir(); err == nil {
		if projectDir := projectConfigDir(homeDir, repoRoot); projectDir != "" {
			paths = append(paths, filepath.Join(projectDir, "mezha.toml"))
		}
		paths = append(paths, filepath.Join(homeDir, "mezha.toml"))
	}
	for _, path := range paths {
		config, found, err := loadConfigMap(path)
		if err != nil {
			return nil, "", err
		}
		if found {
			return decodeConfig(config, path)
		}
	}
	return nil, "", nil
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
	if _, err := toml.Decode(data.String(), &cfg); err != nil {
		return nil, "", fmt.Errorf("decode configuration: %w", err)
	}
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

// expandConfigEnvValues expands environment-variable references in every string
// value in a mezha.toml document before paths are resolved and layers are merged.
// Mapping keys are deliberately left unchanged because they name configuration
// fields rather than string values.
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
	for _, adds := range [][]FileAdd{config.Files.Add, config.Provision.Add} {
		for i := range adds {
			adds[i].Source = resolveConfigPath(baseDir, adds[i].Source)
		}
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
