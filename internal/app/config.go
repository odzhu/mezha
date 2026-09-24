package app

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/go-git/go-git/v5"
)

const defaultDevenvImage = "ghcr.io/cachix/devenv/devenv:latest"

type MezhaConfig struct {
	Version      uint32            `toml:"version,omitempty"`
	Microsandbox *MicrosandboxSpec `toml:"microsandbox,omitempty"`
	Sandbox      SandboxConfig     `toml:"sandbox,omitempty"`
	Services     ServicesConfig    `toml:"services,omitempty"`
	Files        FilesConfig       `toml:"files,omitempty"`
	Provision    ProvisionConfig   `toml:"provision,omitempty"`
	SecretSpec   SecretSpecConfig  `toml:"secretspec,omitempty"`
}

// ServicesConfig controls services available inside the sandbox.
type ServicesConfig struct {
	Docker ServiceConfig `toml:"docker,omitempty"`
	K3s    ServiceConfig `toml:"k3s,omitempty"`
}

// ServiceConfig controls an individual sandbox service.
type ServiceConfig struct {
	Enabled bool `toml:"enabled,omitempty"`
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
		return filepath.Clean(dir), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, ".mezha"), nil
}

// HomeConfigPath returns the single home-level configuration path.
func HomeConfigPath() (string, error) {
	dir, err := HomeConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "mezha.toml"), nil
}

// LoadConfig loads the most specific available configuration. Configurations do
// not inherit or merge from lower-precedence locations.
func LoadConfig(repoRoot string) (*MezhaConfig, string, error) {
	homeDir, err := HomeConfigDir()
	if err != nil {
		return nil, "", err
	}
	projectName, worktreeName, gitRef, err := configScopeNames(repoRoot)
	if err != nil {
		return nil, "", err
	}

	// Check in reverse precedence order: repository, sandbox, worktree,
	// project, then the general Mezha home configuration.
	paths := []string{
		filepath.Join(repoRoot, "mezha.toml"),
		filepath.Join(homeDir, "sandboxes", slugify(projectName+"-"+gitRef), "mezha.toml"),
		filepath.Join(homeDir, "worktrees", slugify(projectName+"-"+worktreeName), "mezha.toml"),
		filepath.Join(homeDir, "projects", slugify(projectName), "mezha.toml"),
		filepath.Join(homeDir, "mezha.toml"),
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

// configScopeNames identifies the source project, current worktree, and ref.
func configScopeNames(repoRoot string) (projectName, worktreeName, gitRef string, err error) {
	projectRoot := repoRoot
	gitPath := filepath.Join(repoRoot, ".git")
	info, statErr := os.Stat(gitPath)
	if statErr != nil {
		return "", "", "", fmt.Errorf("inspect git metadata: %w", statErr)
	}
	if !info.IsDir() {
		data, readErr := os.ReadFile(gitPath)
		if readErr != nil {
			return "", "", "", fmt.Errorf("read git worktree metadata: %w", readErr)
		}
		gitDir := strings.TrimSpace(strings.TrimPrefix(string(data), "gitdir:"))
		if gitDir == string(data) || gitDir == "" {
			return "", "", "", fmt.Errorf("parse git worktree metadata: %s", gitPath)
		}
		if !filepath.IsAbs(gitDir) {
			gitDir = filepath.Join(repoRoot, gitDir)
		}
		gitDir = filepath.Clean(gitDir)
		if filepath.Base(filepath.Dir(gitDir)) == "worktrees" {
			projectRoot = filepath.Dir(filepath.Dir(filepath.Dir(gitDir)))
		}
	}

	repo, openErr := git.PlainOpenWithOptions(repoRoot, &git.PlainOpenOptions{DetectDotGit: true})
	if openErr != nil {
		return "", "", "", fmt.Errorf("open git repository: %w", openErr)
	}
	gitRef, err = currentGitRef(repo)
	if err != nil {
		return "", "", "", err
	}
	return filepath.Base(projectRoot), filepath.Base(repoRoot), gitRef, nil
}

func decodeConfig(config map[string]any, path string) (*MezhaConfig, string, error) {
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
}

func resolveConfigPath(baseDir, value string) string {
	if value == "" || filepath.IsAbs(value) {
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
	return `# Mezha configuration file
# For details, see: https://github.com/odzhu/mezha
version = 1

# Native local Microsandbox configuration. Mezha always uses the native devenv
# container and runs it as UID 0.
[microsandbox]
# Nixpkgs evaluation needs more than Microsandbox's 512 MiB default.
memory_mib = 4096
# cpus = 2
# workdir = "/workspace"

# Secrets are sourced from the local environment and restricted to an allowlist.
# [[microsandbox.secrets]]
# env = "GITHUB_TOKEN"
# value_from_env = "GITHUB_TOKEN"
# allow_hosts = ["api.github.com"]
# require_tls = true

# Bind mounts use [[microsandbox.mounts]] tables.
# [[microsandbox.mounts]]
# source = "."
# target = "/workspace"

# All persistent state shares one volume. Mezha seeds the Nix store and creates
# the required symlinks before entering the devenv shell.
[[microsandbox.volumes]]
name = "state"
target = "/nix"
mode = "ensure-exists"
kind = "disk"
size_mib = 51200

# [microsandbox.network]
# default_egress = "deny"
# [[microsandbox.network.rules]]
# action = "allow"
# direction = "egress"
# destination = "public"

# Resolve secrets with the SecretSpec SDK before Mezha starts the sandbox. The
# values are exported to Mezha and can be passed into the sandbox with
# [[microsandbox.secrets]] entries above.
# [secretspec]
# enabled = true
# provider = "keyring"
# profile = "devtools"
# path = "secretspec.toml"
# scope = "sandbox"
# reason = "start development sandbox"

# Docker, k3s, Git, Lazygit, and GitHub CLI are provided by .mezha/devenv.nix.
[services.docker]
enabled = true
[services.k3s]
enabled = true

# Set name to reuse one sandbox by default, or select one per command with
# --sandbox. Set herdr to register it and synchronize local plugins.
[sandbox]
herdr = false

# Initialization applied only when a new sandbox is provisioned.
[[provision.add]]
source = ".mezha/devenv.nix"
target = "/root/.config/mezha/services/devenv/user-devenv.nix"
`
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
