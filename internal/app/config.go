package app

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5"
	"gopkg.in/yaml.v3"
)

const defaultDevenvImage = "ghcr.io/cachix/devenv/devenv:latest"

type MezhaConfig struct {
	Version      uint32            `yaml:"version,omitempty"`
	Microsandbox *MicrosandboxSpec `yaml:"microsandbox,omitempty"`
	Sandbox      SandboxConfig     `yaml:"sandbox,omitempty"`
	Services     ServicesConfig    `yaml:"services,omitempty"`
	Files        FilesConfig       `yaml:"files,omitempty"`
	Provision    ProvisionConfig   `yaml:"provision,omitempty"`
	Run          []RunDirective    `yaml:"run,omitempty"`
}

// ServicesConfig controls services available inside the sandbox.
type ServicesConfig struct {
	Docker ServiceConfig `yaml:"docker,omitempty"`
	K3s    ServiceConfig `yaml:"k3s,omitempty"`
}

// ServiceConfig controls an individual sandbox service.
type ServiceConfig struct {
	Enabled bool `yaml:"enabled,omitempty"`
}

// FilesConfig describes local paths to add to the sandbox during a Mezha session.
// Sources are resolved relative to the mezha.yaml that defines them, but may
// use ../ or absolute paths. Targets are sandbox paths; relative targets are resolved
// relative to sandbox.remote_dir.
type FilesConfig struct {
	Add []FileAdd `yaml:"add,omitempty"`
}

// ProvisionConfig describes initialization applied only to a new Microsandbox.
type ProvisionConfig struct {
	Add []FileAdd      `yaml:"add,omitempty"`
	Run []RunDirective `yaml:"run,omitempty"`
}

// FileAdd is equivalent to Dockerfile ADD for local files and directories.
// A file target ending in / is treated as a directory; directory sources copy
// their contents into the target directory.
type FileAdd struct {
	Source string `yaml:"source"`
	Target string `yaml:"target"`
}

// UnmarshalYAML accepts the explicit source/target form, a concise one-key
// mapping, and a two-item sequence. The latter two make simple manifests less
// verbose while retaining an unambiguous representation for paths with spaces.
func (a *FileAdd) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.SequenceNode {
		var values []string
		if err := value.Decode(&values); err != nil {
			return err
		}
		if len(values) != 2 {
			return fmt.Errorf("file add entry must contain source and target")
		}
		a.Source, a.Target = values[0], values[1]
		return nil
	}
	if value.Kind != yaml.MappingNode {
		return fmt.Errorf("file add entry must be a mapping or a two-item sequence")
	}
	var explicit struct {
		Source string `yaml:"source"`
		Src    string `yaml:"src"`
		From   string `yaml:"from"`
		Target string `yaml:"target"`
		Dest   string `yaml:"dest"`
		To     string `yaml:"to"`
	}
	if err := value.Decode(&explicit); err != nil {
		return err
	}
	a.Source = firstNonEmpty(explicit.Source, explicit.Src, explicit.From)
	a.Target = firstNonEmpty(explicit.Target, explicit.Dest, explicit.To)
	if a.Source != "" || a.Target != "" {
		return nil
	}
	var concise map[string]string
	if err := value.Decode(&concise); err != nil {
		return err
	}
	if len(concise) != 1 {
		return fmt.Errorf("file add entry must specify source and target")
	}
	for source, target := range concise {
		a.Source, a.Target = source, target
	}
	return nil
}

// RunDirective is a command executed in the sandbox during a Mezha session. A YAML
// string uses Dockerfile RUN's shell form; a YAML sequence uses its exec form.
type RunDirective struct {
	Command []string
	Shell   bool
}

func (r *RunDirective) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		var command string
		if err := value.Decode(&command); err != nil {
			return err
		}
		if strings.TrimSpace(command) == "" {
			return fmt.Errorf("run directive must not be empty")
		}
		r.Command = []string{command}
		r.Shell = true
		return nil
	case yaml.SequenceNode:
		var command []string
		if err := value.Decode(&command); err != nil {
			return err
		}
		if len(command) == 0 {
			return fmt.Errorf("run directive must not be empty")
		}
		r.Command = command
		r.Shell = false
		return nil
	default:
		return fmt.Errorf("run directive must be a command string or an argument list")
	}
}

type SandboxConfig struct {
	Name      string `yaml:"name,omitempty"`
	RemoteDir string `yaml:"remote_dir,omitempty"`
	Recreate  bool   `yaml:"recreate,omitempty"`
	// Herdr registers the sandbox and synchronizes local Herdr plugins.
	Herdr bool `yaml:"herdr,omitempty"`
	// Upload and Download are retained only for backwards-compatible parsing.
	// Repository content is now synchronized exclusively through Git.
	Upload        *bool  `yaml:"upload,omitempty"`
	Download      *bool  `yaml:"download,omitempty"`
	Editor        string `yaml:"editor,omitempty"`
	PolicyAdvisor *bool  `yaml:"policy_advisor,omitempty"`
	Advisor       *bool  `yaml:"advisor,omitempty"`
	// NoLoginShell skips sourcing shell login/profile startup files
	// (bash -lc) when running the requested command/session in Mezha.
	NoLoginShell bool `yaml:"no_login_shell,omitempty"`
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
	return filepath.Join(dir, "mezha.yaml"), nil
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
		filepath.Join(repoRoot, "mezha.yaml"),
		filepath.Join(repoRoot, "mezha.yml"),
		filepath.Join(homeDir, "sandboxes", slugify(projectName+"-"+gitRef), "mezha.yaml"),
		filepath.Join(homeDir, "worktrees", slugify(projectName+"-"+worktreeName), "mezha.yaml"),
		filepath.Join(homeDir, "projects", slugify(projectName), "mezha.yaml"),
		filepath.Join(homeDir, "mezha.yaml"),
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
	data, err := yaml.Marshal(config)
	if err != nil {
		return nil, "", fmt.Errorf("encode configuration: %w", err)
	}
	var cfg MezhaConfig
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&cfg); err != nil {
		return nil, "", fmt.Errorf("decode merged configuration: %w", err)
	}
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
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, false, fmt.Errorf("decode %s: %w", path, err)
	}
	if config == nil {
		config = make(map[string]any)
	}
	expandConfigEnvValues(config)
	resolveConfigPaths(config, filepath.Dir(path))
	return config, true, nil
}

// expandConfigEnvValues expands environment-variable references in every string
// value in a mezha.yaml document before paths are resolved and layers are merged.
// Mapping keys are deliberately left unchanged: they name configuration fields
// (and, for legacy concise files.add entries, source paths) rather than string values.
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
	}
}

func resolveConfigPaths(config map[string]any, baseDir string) {
	if microsandbox, ok := config["microsandbox"].(map[string]any); ok {
		if dockerfile, ok := microsandbox["dockerfile"].(string); ok {
			microsandbox["dockerfile"] = resolveConfigPath(baseDir, dockerfile)
		}
	}

	resolveAdds := func(section string) {
		sectionConfig, ok := config[section].(map[string]any)
		if !ok {
			return
		}
		adds, ok := sectionConfig["add"].([]any)
		if !ok {
			return
		}
		for _, entry := range adds {
			switch value := entry.(type) {
			case []any:
				if len(value) > 0 {
					if source, ok := value[0].(string); ok {
						value[0] = resolveConfigPath(baseDir, source)
					}
				}
			case map[string]any:
				resolved := false
				for _, key := range []string{"source", "src", "from"} {
					if source, ok := value[key].(string); ok {
						value[key] = resolveConfigPath(baseDir, source)
						resolved = true
						break
					}
				}
				if !resolved && len(value) == 1 {
					for source, target := range value {
						delete(value, source)
						value[resolveConfigPath(baseDir, source)] = target
					}
				}
			}
		}
	}
	resolveAdds("files")
	resolveAdds("provision")
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
version: 1

# Native local Microsandbox configuration
microsandbox:
  # Mezha always uses the native devenv container and runs it as UID 0.

  # Sandbox resources. Nixpkgs evaluation needs more than Microsandbox's
  # 512 MiB default.
  memory_mib: 4096
  # cpus: 2

  # Working directory for commands
  # workdir: "/workspace"

  # Environment variables passed to all commands. GOPATH defaults to
  # /sandbox/go when it is not explicitly configured.
  env:
    MODE: "development"

  # Bind mounts
  # mounts:
  #   - source: "."
  #     target: "/workspace"

  volumes:
    # All persistent state shares one volume. Mezha seeds the Nix store and
    # creates the required symlinks before entering the devenv shell.
    - name: "state"
      target: "/nix"
      mode: "ensure-exists"
      kind: "disk"
      size_mib: 51200

  # Network configuration
  # network:
  #   default_egress: deny
  #   rules:
  #     - action: allow
  #       direction: egress
  #       destination: public

  # Secrets available at the network boundary
  # secrets:
  #   - env: API_KEY
  #     value_from_env: API_KEY
  #     allow_hosts: ["api.example.com"]

# Docker, k3s, Git, Lazygit, and GitHub CLI are declaratively installed through
# the packages and enterShell tasks in .mezha/devenv.nix. Mezha uploads that configuration when
# the sandbox is created.
services:
  docker:
    enabled: true
  # Run a k3s server alongside each Mezha session. kubectl and Docker commands
  # share the primary sandbox's Docker daemon.
  k3s:
    enabled: true

# Set name to reuse one sandbox by default, or select one per command with
# --sandbox.
sandbox:
  # name: shared-dev
  # Register the sandbox with Herdr and synchronize local plugins.
  herdr: false

# Initialization applied only when a new sandbox is provisioned.
provision:
  add:
    - [".mezha/devenv.nix", "/sandbox/devenv.nix"]

# Commands executed in the sandbox before the requested command.
# A string uses a shell; a nested list is an exec-form command.
run: []
  # - apk add --no-cache git
  # - ["git", "config", "--global", "init.defaultBranch", "main"]
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
