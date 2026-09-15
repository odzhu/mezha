package app

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const defaultDevenvImage = "ghcr.io/cachix/devenv/devenv:latest"

type MezhaConfig struct {
	Version      uint32            `yaml:"version,omitempty"`
	Microsandbox *MicrosandboxSpec `yaml:"microsandbox,omitempty"`
	Sandbox      SandboxConfig     `yaml:"sandbox,omitempty"`
	Files        FilesConfig       `yaml:"files,omitempty"`
	Create       CreateConfig      `yaml:"create,omitempty"`
	Run          []RunDirective    `yaml:"run,omitempty"`
	Kubernetes   *bool             `yaml:"kubernetes,omitempty"`
	Docker       DockerConfig      `yaml:"docker,omitempty"`
}

// DockerConfig controls the Docker daemon available inside the sandbox.
type DockerConfig struct {
	Enabled bool `yaml:"enabled,omitempty"`
}

// FilesConfig describes local paths to add to the sandbox during mezha run.
// Sources are resolved relative to the mezha.yaml that defines them, but may
// use ../ or absolute paths. Targets are sandbox paths; relative targets are resolved
// relative to sandbox.remote_dir.
type FilesConfig struct {
	Add []FileAdd `yaml:"add,omitempty"`
}

// CreateConfig describes initialization applied only to a new Microsandbox.
type CreateConfig struct {
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

// RunDirective is a command executed in the sandbox during mezha run. A YAML
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
	// Kubernetes runs k3s alongside each requested command in this sandbox.
	Kubernetes bool `yaml:"kubernetes,omitempty"`
	// Upload and Download are retained only for backwards-compatible parsing.
	// Repository content is now synchronized exclusively through Git.
	Upload        *bool  `yaml:"upload,omitempty"`
	Download      *bool  `yaml:"download,omitempty"`
	Editor        string `yaml:"editor,omitempty"`
	PolicyAdvisor *bool  `yaml:"policy_advisor,omitempty"`
	Advisor       *bool  `yaml:"advisor,omitempty"`
	// NoLoginShell skips sourcing shell login/profile startup files
	// (bash -lc) when running the requested command/session in `mezha run`.
	NoLoginShell bool `yaml:"no_login_shell,omitempty"`
}

// HomeConfigDir returns the mezha home configuration directory (~/.mezha).
func HomeConfigDir() (string, error) {
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

// LoadConfig loads a project configuration when present; otherwise it loads
// the home-level configuration. Project configuration never inherits or merges
// values from the home-level configuration.
func LoadConfig(repoRoot string) (*MezhaConfig, string, error) {

	projectPaths := []string{
		filepath.Join(repoRoot, "mezha.yaml"),
		filepath.Join(repoRoot, "mezha.yml"),
	}
	var project map[string]any
	var projectPath string
	for _, path := range projectPaths {
		var projectFound bool
		var err error
		project, projectFound, err = loadConfigMap(path)
		if err != nil {
			return nil, "", err
		}
		if projectFound {
			projectPath = path
			break
		}
	}

	if projectPath != "" {
		return decodeConfig(project, projectPath)
	}

	homePath, err := HomeConfigPath()
	if err != nil {
		return nil, "", err
	}
	home, homeFound, err := loadConfigMap(homePath)
	if err != nil {
		return nil, "", err
	}
	if !homeFound {
		return nil, "", nil
	}
	return decodeConfig(home, homePath)
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
	resolveAdds("create")
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

  # Environment variables passed to all commands
  env:
    MODE: "development"

  # Bind mounts
  # mounts:
  #   - source: "."
  #     target: "/workspace"

  volumes:
    # Persistent Nix package store. Mezha seeds it from the native image
    # before mounting it at /nix/store.
    - name: "nix-packages"
      target: "/nix/store"
      mode: "ensure-exists"
      kind: "disk"
      size_mib: 20480

    # Keep images, containers, and Docker volumes independent of the sandbox
    # filesystem. This is used when docker.enabled is true.
    - name: "docker-data"
      target: "/var/lib/docker"
      mode: "ensure-exists"
      kind: "disk"
      size_mib: 20480

    # Keep k3s cluster data when Kubernetes is enabled; its images use Docker.
    - name: "k3s-data"
      target: "/var/lib/rancher/k3s"
      mode: "ensure-exists"
      kind: "disk"
      size_mib: 20480

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

# Docker and k3s are declaratively installed by .mezha/devenv.nix. Mezha
# uploads that configuration when the sandbox is created and enters it before
# starting either service.
docker:
  enabled: true

# Run a k3s server alongside each Mezha session. kubectl and Docker commands
# share the primary sandbox's Docker daemon.
sandbox:
  kubernetes: true

# Initialization applied only when a new sandbox is created.
create:
  add:
    - [".mezha/devenv.nix", "/opt/mezha/devenv.nix"]

# Commands executed in the sandbox before the requested command.
# A string uses a shell; a nested list is an exec-form command.
run: []
  # - apk add --no-cache git
  # - ["git", "config", "--global", "init.defaultBranch", "main"]
`
}

func resolveFilePatterns(baseDir string, patterns []string) ([]string, error) {
	var resolved []string
	seen := make(map[string]struct{})

	for _, pattern := range patterns {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}

		var fullPattern string
		if filepath.IsAbs(pattern) {
			fullPattern = pattern
		} else {
			fullPattern = filepath.Join(baseDir, pattern)
		}

		if strings.ContainsAny(pattern, "*?[]") {
			matches, err := filepath.Glob(fullPattern)
			if err != nil {
				return nil, fmt.Errorf("expand pattern %q: %w", pattern, err)
			}
			for _, match := range matches {
				info, err := os.Stat(match)
				if err == nil && !info.IsDir() {
					if _, ok := seen[match]; !ok {
						seen[match] = struct{}{}
						resolved = append(resolved, match)
					}
				}
			}
		} else {
			info, err := os.Stat(fullPattern)
			if err != nil {
				return nil, fmt.Errorf("read file %q: %w", pattern, err)
			}
			if info.IsDir() {
				return nil, fmt.Errorf(
					"path %q is a directory, expected a file or glob pattern",
					pattern,
				)
			}
			if _, ok := seen[fullPattern]; !ok {
				seen[fullPattern] = struct{}{}
				resolved = append(resolved, fullPattern)
			}
		}
	}

	sort.Strings(resolved)
	return resolved, nil
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
