package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDecodeConfigRejectsServices(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "mezha.toml")
	content := `
[services.docker]
enabled = false
`
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write test config: %v", err)
	}

	configMap, found, err := loadConfigMap(configPath)
	if err != nil {
		t.Fatalf("load config map: %v", err)
	}
	if !found {
		t.Fatal("expected config to be found")
	}

	_, _, err = decodeConfig(configMap, configPath)
	if err == nil {
		t.Fatal("expected error decoding config with services, got nil")
	}
	expected := "services configuration is no longer supported; manage provisioning parameters in devenv.nix"
	if err.Error() != expected {
		t.Fatalf("expected error %q, got %q", expected, err.Error())
	}
}

func TestDecodeConfigRejectsWorkdir(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "mezha.toml")
	content := `
[microsandbox]
workdir = "/workspace"
`
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write test config: %v", err)
	}

	configMap, found, err := loadConfigMap(configPath)
	if err != nil {
		t.Fatalf("load config map: %v", err)
	}
	if !found {
		t.Fatal("expected config to be found")
	}

	_, _, err = decodeConfig(configMap, configPath)
	if err == nil {
		t.Fatal("expected error decoding config with microsandbox.workdir, got nil")
	}
	expected := "microsandbox.workdir is no longer supported"
	if err.Error() != expected {
		t.Fatalf("expected error %q, got %q", expected, err.Error())
	}
}

func TestDefaultConfigTemplateValid(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "mezha.toml")
	template := DefaultConfigTemplate()
	if err := os.WriteFile(configPath, []byte(template), 0o644); err != nil {
		t.Fatalf("write template config: %v", err)
	}

	configMap, found, err := loadConfigMap(configPath)
	if err != nil {
		t.Fatalf("load config map: %v", err)
	}
	if !found {
		t.Fatal("expected template config to be found")
	}

	cfg, _, err := decodeConfig(configMap, configPath)
	if err != nil {
		t.Fatalf("decode template config: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
}

func TestParseDevenvHasEnabledProcesses(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    bool
		wantErr bool
	}{
		{
			name:  "empty string",
			input: "",
			want:  false,
		},
		{
			name:  "no processes",
			input: `{"processes": {}}`,
			want:  false,
		},
		{
			name: "processes disabled",
			input: `{
				"processes": {
					"docker": {"start": {"enable": false}},
					"k3s": {"start": {"enable": false}}
				}
			}`,
			want: false,
		},
		{
			name: "processes enabled",
			input: `{
				"processes": {
					"docker": {"start": {"enable": true}},
					"k3s": {"start": {"enable": false}}
				}
			}`,
			want: true,
		},
		{
			name: "wrapped in log output",
			input: `evaluating...
{
	"processes": {
		"custom-service": {"start": {"enable": true}}
	}
}
done`,
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseDevenvHasEnabledProcesses(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseDevenvHasEnabledProcesses() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("parseDevenvHasEnabledProcesses() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLoadConfigGlobalAndProject(t *testing.T) {
	globalDir := t.TempDir()
	projectDir := t.TempDir()
	t.Setenv("MEZHA_HOME", globalDir)

	globalConfig := filepath.Join(globalDir, "mezha.toml")
	if err := os.WriteFile(
		globalConfig,
		[]byte("version = 1\n[sandbox]\nname = \"global-sandbox\"\n"),
		0o644,
	); err != nil {
		t.Fatalf("write global config: %v", err)
	}

	// Falls back to global when project config is absent
	cfg, path, err := LoadConfig(projectDir)
	if err != nil {
		t.Fatalf("LoadConfig error = %v", err)
	}
	if path != globalConfig {
		t.Fatalf("LoadConfig path = %q, want %q", path, globalConfig)
	}
	if cfg.Sandbox.Name != "global-sandbox" {
		t.Fatalf("cfg.Sandbox.Name = %q, want global-sandbox", cfg.Sandbox.Name)
	}

	// Home project config takes precedence over global
	homeProjectDir := filepath.Join(globalDir, "projects", slugify(filepath.Base(projectDir)))
	if err := os.MkdirAll(homeProjectDir, 0o755); err != nil {
		t.Fatalf("mkdir home project: %v", err)
	}
	homeProjectConfig := filepath.Join(homeProjectDir, "mezha.toml")
	if err := os.WriteFile(
		homeProjectConfig,
		[]byte("version = 1\n[sandbox]\nname = \"home-project-sandbox\"\n"),
		0o644,
	); err != nil {
		t.Fatalf("write home project config: %v", err)
	}
	cfg, path, err = LoadConfig(projectDir)
	if err != nil {
		t.Fatalf("LoadConfig error = %v", err)
	}
	if path != homeProjectConfig {
		t.Fatalf("LoadConfig path = %q, want %q", path, homeProjectConfig)
	}
	if cfg.Sandbox.Name != "home-project-sandbox" {
		t.Fatalf("cfg.Sandbox.Name = %q, want home-project-sandbox", cfg.Sandbox.Name)
	}

	// Project repo config takes highest precedence
	projectConfig := filepath.Join(projectDir, "mezha.toml")
	if err := os.WriteFile(
		projectConfig,
		[]byte("version = 1\n[sandbox]\nname = \"project-sandbox\"\n"),
		0o644,
	); err != nil {
		t.Fatalf("write project config: %v", err)
	}
	cfg, path, err = LoadConfig(projectDir)
	if err != nil {
		t.Fatalf("LoadConfig error = %v", err)
	}
	if path != projectConfig {
		t.Fatalf("LoadConfig path = %q, want %q", path, projectConfig)
	}
	if cfg.Sandbox.Name != "project-sandbox" {
		t.Fatalf("cfg.Sandbox.Name = %q, want project-sandbox", cfg.Sandbox.Name)
	}
}

func TestCandidateConfigPaths(t *testing.T) {
	globalDir := t.TempDir()
	projectDir := t.TempDir()
	t.Setenv("MEZHA_HOME", globalDir)

	candidates := CandidateConfigPaths(projectDir)
	expectedGlobal := filepath.Join(globalDir, "mezha.toml")
	expectedProjectUser := filepath.Join(
		globalDir,
		"projects",
		slugify(filepath.Base(projectDir)),
		"mezha.toml",
	)
	expectedLocalRepo := filepath.Join(projectDir, "mezha.toml")

	if len(candidates) != 3 {
		t.Fatalf("CandidateConfigPaths length = %d, want 3: %v", len(candidates), candidates)
	}
	if candidates[0] != expectedGlobal {
		t.Errorf("candidate[0] = %q, want %q", candidates[0], expectedGlobal)
	}
	if candidates[1] != expectedProjectUser {
		t.Errorf("candidate[1] = %q, want %q", candidates[1], expectedProjectUser)
	}
	if candidates[2] != expectedLocalRepo {
		t.Errorf("candidate[2] = %q, want %q", candidates[2], expectedLocalRepo)
	}
}

func TestConfigurationLayering(t *testing.T) {
	globalDir := t.TempDir()
	projectDir := t.TempDir()
	t.Setenv("MEZHA_HOME", globalDir)

	// 1. Global config
	globalConfig := filepath.Join(globalDir, "mezha.toml")
	globalContent := `
version = 1

[sandbox]
remote_dir = "/workspace-global"

[microsandbox]
cpus = 2
memory_mib = 2048

[[microsandbox.mounts]]
source = "./data"
target = "/mnt/data"

[[microsandbox.volumes]]
name = "custom"
target = "/var/custom"
mode = "ensure-exists"
kind = "disk"

[[microsandbox.secrets]]
env = "API_KEY"
value = "global_key"

[[files.add]]
source = "./config.json"
target = "config.json"

[microsandbox.network]
profiles = ["base", "global"]
deny_domains = ["bad.global"]

[[microsandbox.network.port_bindings]]
host_port = 8080
guest_port = 80
protocol = "tcp"

[microsandbox.network.ports]
"3000" = 3000

[[microsandbox.network.rules]]
action = "allow"
direction = "egress"
protocol = "tcp"

[secretspec]
enabled = true
provider = "global-provider"
profile = "prod"
`
	if err := os.WriteFile(globalConfig, []byte(globalContent), 0o644); err != nil {
		t.Fatalf("write global config: %v", err)
	}

	// 2. Project user config
	projectUserDir := filepath.Join(globalDir, "projects", slugify(filepath.Base(projectDir)))
	if err := os.MkdirAll(projectUserDir, 0o755); err != nil {
		t.Fatalf("mkdir project user dir: %v", err)
	}
	projectUserConfig := filepath.Join(projectUserDir, "mezha.toml")
	projectUserContent := `
[microsandbox]
memory_mib = 8192

[microsandbox.network]
deny_domains = ["bad.project"]

[secretspec]
profile = "staging"
`
	if err := os.WriteFile(projectUserConfig, []byte(projectUserContent), 0o644); err != nil {
		t.Fatalf("write project user config: %v", err)
	}

	// 3. Local repo config
	localRepoConfig := filepath.Join(projectDir, "mezha.toml")
	localRepoContent := `
[sandbox]
name = "repo-sandbox"
remote_dir = "/workspace-repo"

[[microsandbox.mounts]]
source = "./repo-data"
target = "/mnt/data"

[[microsandbox.mounts]]
source = "./extra"
target = "/mnt/extra"

[[microsandbox.secrets]]
env = "API_KEY"
value = "repo_key"

[[microsandbox.secrets]]
env = "OTHER_KEY"
value = "other_val"

[[files.add]]
source = "./repo-config.json"
target = "config.json"

[microsandbox.network]
profiles = ["repo"]

[[microsandbox.network.port_bindings]]
host_port = 8080
guest_port = 8080
protocol = "tcp"

[[microsandbox.network.port_bindings]]
host_port = 8080
guest_port = 80
protocol = "udp"

[microsandbox.network.ports]
"4000" = 4000

[[microsandbox.network.rules]]
action = "deny"
direction = "ingress"
protocol = "udp"
`
	if err := os.WriteFile(localRepoConfig, []byte(localRepoContent), 0o644); err != nil {
		t.Fatalf("write local repo config: %v", err)
	}

	effective, activePath, loadedPaths, err := LoadConfigLayers(projectDir)
	if err != nil {
		t.Fatalf("LoadConfigLayers error = %v", err)
	}

	if activePath != localRepoConfig {
		t.Errorf("activePath = %q, want %q", activePath, localRepoConfig)
	}
	if len(loadedPaths) != 3 {
		t.Fatalf("loadedPaths length = %d, want 3: %v", len(loadedPaths), loadedPaths)
	}

	// Scalars
	if effective.Sandbox.Name != "repo-sandbox" {
		t.Errorf("Sandbox.Name = %q, want repo-sandbox", effective.Sandbox.Name)
	}
	if effective.Sandbox.RemoteDir != "/workspace-repo" {
		t.Errorf("Sandbox.RemoteDir = %q, want /workspace-repo", effective.Sandbox.RemoteDir)
	}
	if effective.Microsandbox.CPUs != 2 {
		t.Errorf("Microsandbox.CPUs = %d, want 2", effective.Microsandbox.CPUs)
	}
	if effective.Microsandbox.MemoryMiB != 8192 {
		t.Errorf("Microsandbox.MemoryMiB = %d, want 8192", effective.Microsandbox.MemoryMiB)
	}

	// Mounts: merged by Target and relative paths resolved relative to defining file
	if len(effective.Microsandbox.Mounts) != 2 {
		t.Fatalf("Mounts length = %d, want 2", len(effective.Microsandbox.Mounts))
	}
	var dataMount, extraMount *MicrosandboxMount
	for i := range effective.Microsandbox.Mounts {
		m := &effective.Microsandbox.Mounts[i]
		switch m.Target {
		case "/mnt/data":
			dataMount = m
		case "/mnt/extra":
			extraMount = m
		}
	}
	if dataMount == nil || dataMount.Source != filepath.Join(projectDir, "repo-data") {
		t.Errorf(
			"dataMount Source = %v, want %q",
			dataMount,
			filepath.Join(projectDir, "repo-data"),
		)
	}
	if extraMount == nil || extraMount.Source != filepath.Join(projectDir, "extra") {
		t.Errorf("extraMount Source = %v, want %q", extraMount, filepath.Join(projectDir, "extra"))
	}

	// Volumes: default /nix preserved + custom volume preserved
	hasNix := false
	hasCustom := false
	for _, v := range effective.Microsandbox.Volumes {
		if v.Target == "/nix" {
			hasNix = true
		}
		if v.Target == "/var/custom" {
			hasCustom = true
		}
	}
	if !hasNix {
		t.Error("expected default /nix volume to be preserved")
	}
	if !hasCustom {
		t.Error("expected /var/custom volume from global config to be preserved")
	}

	// Secrets: merged by Env
	if len(effective.Microsandbox.Secrets) != 2 {
		t.Fatalf("Secrets length = %d, want 2", len(effective.Microsandbox.Secrets))
	}
	for _, s := range effective.Microsandbox.Secrets {
		if s.EnvVar == "API_KEY" && s.Value != "repo_key" {
			t.Errorf("API_KEY secret value = %q, want repo_key", s.Value)
		}
		if s.EnvVar == "OTHER_KEY" && s.Value != "other_val" {
			t.Errorf("OTHER_KEY secret value = %q, want other_val", s.Value)
		}
	}

	// Files.Add: merged by Target
	if len(effective.Files.Add) != 1 {
		t.Fatalf("Files.Add length = %d, want 1", len(effective.Files.Add))
	}
	if effective.Files.Add[0].Source != filepath.Join(projectDir, "repo-config.json") {
		t.Errorf(
			"Files.Add Source = %q, want %q",
			effective.Files.Add[0].Source,
			filepath.Join(projectDir, "repo-config.json"),
		)
	}

	// Network: profiles union, deny_domains union, port_bindings merged, ports merged, rules appended
	profiles := strings.Join(effective.Microsandbox.Network.Profiles, ",")
	if !strings.Contains(profiles, "base") || !strings.Contains(profiles, "global") ||
		!strings.Contains(profiles, "repo") {
		t.Errorf(
			"Network.Profiles = %v, want union containing base, global, repo",
			effective.Microsandbox.Network.Profiles,
		)
	}
	domains := strings.Join(effective.Microsandbox.Network.DenyDomains, ",")
	if !strings.Contains(domains, "bad.global") || !strings.Contains(domains, "bad.project") {
		t.Errorf(
			"Network.DenyDomains = %v, want union containing bad.global, bad.project",
			effective.Microsandbox.Network.DenyDomains,
		)
	}
	if len(effective.Microsandbox.Network.PortBindings) != 2 {
		t.Fatalf(
			"PortBindings length = %d, want 2",
			len(effective.Microsandbox.Network.PortBindings),
		)
	}
	for _, pb := range effective.Microsandbox.Network.PortBindings {
		if pb.Protocol == "tcp" && pb.GuestPort != 8080 {
			t.Errorf("tcp 8080 GuestPort = %d, want 8080", pb.GuestPort)
		}
		if pb.Protocol == "udp" && pb.GuestPort != 80 {
			t.Errorf("udp 8080 GuestPort = %d, want 80", pb.GuestPort)
		}
	}
	if effective.Microsandbox.Network.Ports["3000"] != 3000 ||
		effective.Microsandbox.Network.Ports["4000"] != 4000 {
		t.Errorf("Ports = %v, want 3000 and 4000", effective.Microsandbox.Network.Ports)
	}
	if len(effective.Microsandbox.Network.Rules) != 2 {
		t.Errorf("Rules length = %d, want 2", len(effective.Microsandbox.Network.Rules))
	}

	// SecretSpec: overlay non-empty fields, Enabled true overrides
	if !effective.SecretSpec.Enabled {
		t.Error("SecretSpec.Enabled = false, want true")
	}
	if effective.SecretSpec.Provider != "global-provider" {
		t.Errorf("SecretSpec.Provider = %q, want global-provider", effective.SecretSpec.Provider)
	}
	if effective.SecretSpec.Profile != "staging" {
		t.Errorf("SecretSpec.Profile = %q, want staging", effective.SecretSpec.Profile)
	}
}

func TestSaneDefaultsAndZeroConfig(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("MEZHA_HOME", t.TempDir()) // empty global dir

	// 1. DefaultConfig
	cfg := DefaultConfig(tempDir)
	if cfg == nil {
		t.Fatal("expected non-nil DefaultConfig")
	}
	expectedName := slugify(filepath.Base(tempDir))
	if cfg.Sandbox.Name != expectedName {
		t.Errorf("Sandbox.Name = %q, want %q", cfg.Sandbox.Name, expectedName)
	}
	if cfg.Sandbox.RemoteDir != "" {
		t.Errorf("Sandbox.RemoteDir = %q, want empty", cfg.Sandbox.RemoteDir)
	}
	if cfg.Microsandbox == nil {
		t.Fatal("expected non-nil Microsandbox in DefaultConfig")
	}
	if cfg.Microsandbox.MemoryMiB != 4096 {
		t.Errorf("MemoryMiB = %d, want 4096", cfg.Microsandbox.MemoryMiB)
	}
	if len(cfg.Microsandbox.Volumes) != 1 {
		t.Fatalf("Volumes length = %d, want 1", len(cfg.Microsandbox.Volumes))
	}
	v := cfg.Microsandbox.Volumes[0]
	if v.Name != "state" || v.Target != "/nix" || v.Mode != "ensure-exists" || v.Kind != "disk" ||
		v.SizeMiB != 51200 {
		t.Errorf("default volume = %+v, unexpected fields", v)
	}

	// 2. Zero-config LoadConfig returns DefaultConfig when no file found
	loaded, activePath, err := LoadConfig(tempDir)
	if err != nil {
		t.Fatalf("LoadConfig zero-config error = %v", err)
	}
	if activePath != "" {
		t.Errorf("activePath = %q, want empty", activePath)
	}
	if loaded.Sandbox.Name != expectedName {
		t.Errorf("loaded.Sandbox.Name = %q, want %q", loaded.Sandbox.Name, expectedName)
	}
	if loaded.Microsandbox.MemoryMiB != 4096 {
		t.Errorf("loaded MemoryMiB = %d, want 4096", loaded.Microsandbox.MemoryMiB)
	}

	// 3. Overlay that omits microsandbox gets sane defaults applied
	configPath := filepath.Join(tempDir, "mezha.toml")
	if err := os.WriteFile(
		configPath,
		[]byte("version = 1\n[sandbox]\nname = \"zero-msb\"\n"),
		0o644,
	); err != nil {
		t.Fatalf("write test config: %v", err)
	}
	loaded, activePath, err = LoadConfig(tempDir)
	if err != nil {
		t.Fatalf("LoadConfig error = %v", err)
	}
	if activePath != configPath {
		t.Errorf("activePath = %q, want %q", activePath, configPath)
	}
	if loaded.Sandbox.Name != "zero-msb" {
		t.Errorf("loaded Sandbox.Name = %q, want zero-msb", loaded.Sandbox.Name)
	}
	if loaded.Microsandbox == nil || loaded.Microsandbox.MemoryMiB != 4096 {
		t.Errorf("loaded Microsandbox.MemoryMiB = %v, want 4096", loaded.Microsandbox)
	}
	if len(loaded.Microsandbox.Volumes) != 1 || loaded.Microsandbox.Volumes[0].Target != "/nix" {
		t.Errorf("loaded Volumes = %v, want default /nix volume", loaded.Microsandbox.Volumes)
	}

	// 4. Overlay specifying custom /nix volume does not duplicate it
	customNix := `
[microsandbox]
memory_mib = 2048

[[microsandbox.volumes]]
name = "custom-nix"
target = "/nix"
mode = "ensure-exists"
kind = "disk"
size_mib = 102400
`
	if err := os.WriteFile(configPath, []byte(customNix), 0o644); err != nil {
		t.Fatalf("write custom nix config: %v", err)
	}
	loaded, _, err = LoadConfig(tempDir)
	if err != nil {
		t.Fatalf("LoadConfig error = %v", err)
	}
	if len(loaded.Microsandbox.Volumes) != 1 {
		t.Fatalf("Volumes length = %d, want 1 (no duplicate)", len(loaded.Microsandbox.Volumes))
	}
	if loaded.Microsandbox.Volumes[0].SizeMiB != 102400 {
		t.Errorf("Volumes[0].SizeMiB = %d, want 102400", loaded.Microsandbox.Volumes[0].SizeMiB)
	}
	if loaded.Microsandbox.MemoryMiB != 2048 {
		t.Errorf("MemoryMiB = %d, want 2048", loaded.Microsandbox.MemoryMiB)
	}
}

func TestDirectDecodingAndObsoleteKeys(t *testing.T) {
	tempDir := t.TempDir()

	t.Setenv("TEST_APP_NAME", "expanded-sandbox")
	t.Setenv("TEST_ENV_VAR", "expanded-secret-val")
	t.Setenv("TEST_TARGET", "/workspace-expanded")

	validContent := `
version = 1

[sandbox]
name = "$TEST_APP_NAME"
remote_dir = "${UNSET_VAR:-/workspace-default}"

[[microsandbox.secrets]]
env = "SECRET_KEY"
value = "env://TEST_ENV_VAR"

[microsandbox.scripts]
init = "echo $TEST_APP_NAME"
`
	configPath := filepath.Join(tempDir, "mezha.toml")
	if err := os.WriteFile(configPath, []byte(validContent), 0o644); err != nil {
		t.Fatalf("write valid config: %v", err)
	}

	cfg, found, err := loadConfigFile(configPath)
	if err != nil {
		t.Fatalf("loadConfigFile error = %v", err)
	}
	if !found || cfg == nil {
		t.Fatal("expected found config")
	}

	if cfg.Sandbox.Name != "expanded-sandbox" {
		t.Errorf("Sandbox.Name = %q, want expanded-sandbox", cfg.Sandbox.Name)
	}
	if cfg.Sandbox.RemoteDir != "/workspace-default" {
		t.Errorf("Sandbox.RemoteDir = %q, want /workspace-default", cfg.Sandbox.RemoteDir)
	}
	if len(cfg.Microsandbox.Secrets) != 1 ||
		cfg.Microsandbox.Secrets[0].Value != "expanded-secret-val" {
		t.Errorf("Secrets[0].Value = %v, want expanded-secret-val", cfg.Microsandbox.Secrets)
	}
	if cfg.Microsandbox.Scripts["init"] != "echo expanded-sandbox" {
		t.Errorf("Scripts[init] = %q, want echo expanded-sandbox", cfg.Microsandbox.Scripts["init"])
	}

	// Reject services key via loadConfigFile
	servicesPath := filepath.Join(tempDir, "services.toml")
	if err := os.WriteFile(
		servicesPath,
		[]byte("[services.redis]\nenabled = true\n"),
		0o644,
	); err != nil {
		t.Fatalf("write services config: %v", err)
	}
	_, _, err = loadConfigFile(servicesPath)
	if err == nil {
		t.Fatal("expected error on obsolete services key, got nil")
	}
	expectedServicesErr := "services configuration is no longer supported; manage provisioning parameters in devenv.nix"
	if err.Error() != expectedServicesErr {
		t.Errorf("error = %q, want %q", err.Error(), expectedServicesErr)
	}

	// Reject microsandbox.workdir via loadConfigFile
	workdirPath := filepath.Join(tempDir, "workdir.toml")
	if err := os.WriteFile(
		workdirPath,
		[]byte("[microsandbox]\nworkdir = \"/app\"\n"),
		0o644,
	); err != nil {
		t.Fatalf("write workdir config: %v", err)
	}
	_, _, err = loadConfigFile(workdirPath)
	if err == nil {
		t.Fatal("expected error on obsolete microsandbox.workdir key, got nil")
	}
	expectedWorkdirErr := "microsandbox.workdir is no longer supported"
	if err.Error() != expectedWorkdirErr {
		t.Errorf("error = %q, want %q", err.Error(), expectedWorkdirErr)
	}
}

func TestValidate(t *testing.T) {
	tempDir := t.TempDir()
	mountSrc := filepath.Join(tempDir, "host-data")
	if err := os.Mkdir(mountSrc, 0o755); err != nil {
		t.Fatalf("mkdir mount source: %v", err)
	}
	fileSrc := filepath.Join(tempDir, "add-file.txt")
	if err := os.WriteFile(fileSrc, []byte("data"), 0o644); err != nil {
		t.Fatalf("write file source: %v", err)
	}
	specPath := filepath.Join(tempDir, "spec.json")
	if err := os.WriteFile(specPath, []byte("{}"), 0o644); err != nil {
		t.Fatalf("write spec path: %v", err)
	}

	// Valid config
	validCfg := &MezhaConfig{
		Sandbox: SandboxConfig{
			Name:      "test-box",
			RemoteDir: "/workspace",
		},
		SecretSpec: SecretSpecConfig{
			Enabled: true,
			Path:    specPath,
		},
		Files: FilesConfig{
			Add: []FileAdd{
				{Source: fileSrc, Target: "data.txt"},
			},
		},
		Microsandbox: &MicrosandboxSpec{
			MemoryMiB: 4096,
			Mounts: []MicrosandboxMount{
				{Source: mountSrc, Target: "/mnt/data"},
			},
			Volumes: []MicrosandboxVolume{
				{Name: "state", Target: "/nix", Mode: "ensure-exists", Kind: "disk"},
			},
			Ports:    map[string]uint16{"8080": 80},
			PortsUDP: map[string]uint16{"53": 53},
			PortBindings: []MicrosandboxPortBinding{
				{HostPort: 8080, GuestPort: 80, Protocol: "tcp"},
			},
			Network: MicrosandboxNetwork{
				Rules: []MicrosandboxNetworkRule{
					{Action: "allow", Direction: "egress", Protocol: "tcp"},
					{Action: "deny", Direction: "ingress", Protocol: "udp"},
				},
			},
		},
	}

	if err := validCfg.Validate(); err != nil {
		t.Fatalf("valid config Validate() error = %v", err)
	}

	// Invalid test cases
	tests := []struct {
		name    string
		modify  func(cfg *MezhaConfig)
		wantErr string
	}{
		{
			name: "mount empty source",
			modify: func(c *MezhaConfig) {
				c.Microsandbox.Mounts[0].Source = ""
			},
			wantErr: "mount source cannot be empty",
		},
		{
			name: "mount empty target",
			modify: func(c *MezhaConfig) {
				c.Microsandbox.Mounts[0].Target = ""
			},
			wantErr: "mount target cannot be empty",
		},
		{
			name: "mount relative target",
			modify: func(c *MezhaConfig) {
				c.Microsandbox.Mounts[0].Target = "mnt/data"
			},
			wantErr: "must be absolute",
		},
		{
			name: "mount non-existent source",
			modify: func(c *MezhaConfig) {
				c.Microsandbox.Mounts[0].Source = filepath.Join(tempDir, "nonexistent")
			},
			wantErr: "mount source",
		},
		{
			name: "mount duplicate targets",
			modify: func(c *MezhaConfig) {
				c.Microsandbox.Mounts = append(c.Microsandbox.Mounts, MicrosandboxMount{
					Source: mountSrc,
					Target: "/mnt/data",
				})
			},
			wantErr: "duplicate mount target",
		},
		{
			name: "volume empty name",
			modify: func(c *MezhaConfig) {
				c.Microsandbox.Volumes[0].Name = ""
			},
			wantErr: "volume name cannot be empty",
		},
		{
			name: "volume empty target",
			modify: func(c *MezhaConfig) {
				c.Microsandbox.Volumes[0].Target = ""
			},
			wantErr: "volume target cannot be empty",
		},
		{
			name: "volume relative target",
			modify: func(c *MezhaConfig) {
				c.Microsandbox.Volumes[0].Target = "var/data"
			},
			wantErr: "must be absolute",
		},
		{
			name: "volume invalid kind",
			modify: func(c *MezhaConfig) {
				c.Microsandbox.Volumes[0].Kind = "nfs"
			},
			wantErr: "invalid volume kind",
		},
		{
			name: "volume invalid mode",
			modify: func(c *MezhaConfig) {
				c.Microsandbox.Volumes[0].Mode = "sync"
			},
			wantErr: "invalid volume mode",
		},
		{
			name: "volume duplicate targets",
			modify: func(c *MezhaConfig) {
				c.Microsandbox.Volumes = append(c.Microsandbox.Volumes, MicrosandboxVolume{
					Name:   "state2",
					Target: "/nix",
				})
			},
			wantErr: "duplicate volume target",
		},
		{
			name: "volume target conflicts with mount target",
			modify: func(c *MezhaConfig) {
				c.Microsandbox.Volumes[0].Target = "/mnt/data"
			},
			wantErr: "conflicts with mount target",
		},
		{
			name: "invalid host port string",
			modify: func(c *MezhaConfig) {
				c.Microsandbox.Ports["abc"] = 80
			},
			wantErr: "invalid host port",
		},
		{
			name: "zero host port",
			modify: func(c *MezhaConfig) {
				c.Microsandbox.Ports["0"] = 80
			},
			wantErr: "invalid host port",
		},
		{
			name: "zero guest port",
			modify: func(c *MezhaConfig) {
				c.Microsandbox.Ports["8080"] = 0
			},
			wantErr: "invalid guest port",
		},
		{
			name: "invalid host port udp",
			modify: func(c *MezhaConfig) {
				c.Microsandbox.PortsUDP["foo"] = 53
			},
			wantErr: "invalid host port",
		},
		{
			name: "zero guest port udp",
			modify: func(c *MezhaConfig) {
				c.Microsandbox.PortsUDP["53"] = 0
			},
			wantErr: "invalid guest port",
		},
		{
			name: "port binding host port zero",
			modify: func(c *MezhaConfig) {
				c.Microsandbox.PortBindings[0].HostPort = 0
			},
			wantErr: "invalid host port",
		},
		{
			name: "port binding guest port zero",
			modify: func(c *MezhaConfig) {
				c.Microsandbox.PortBindings[0].GuestPort = 0
			},
			wantErr: "invalid guest port",
		},
		{
			name: "port binding invalid protocol",
			modify: func(c *MezhaConfig) {
				c.Microsandbox.PortBindings[0].Protocol = "http"
			},
			wantErr: "invalid protocol",
		},
		{
			name: "port binding duplicate host port",
			modify: func(c *MezhaConfig) {
				c.Microsandbox.PortBindings = append(
					c.Microsandbox.PortBindings,
					MicrosandboxPortBinding{
						HostPort:  8080,
						GuestPort: 9090,
						Protocol:  "tcp",
					},
				)
			},
			wantErr: "duplicate port binding",
		},
		{
			name: "port binding cross-slice duplicate with network.port_bindings",
			modify: func(c *MezhaConfig) {
				c.Microsandbox.Network.PortBindings = append(
					c.Microsandbox.Network.PortBindings,
					MicrosandboxPortBinding{
						HostPort:  8080,
						GuestPort: 9090,
						Protocol:  "tcp",
					},
				)
			},
			wantErr: "duplicate port binding",
		},
		{
			name: "secretspec non-existent path",
			modify: func(c *MezhaConfig) {
				c.SecretSpec.Path = filepath.Join(tempDir, "missing-spec.json")
			},
			wantErr: "secretspec path",
		},
		{
			name: "network rule invalid action",
			modify: func(c *MezhaConfig) {
				c.Microsandbox.Network.Rules[0].Action = "drop"
			},
			wantErr: "invalid network rule action",
		},
		{
			name: "network rule invalid direction",
			modify: func(c *MezhaConfig) {
				c.Microsandbox.Network.Rules[0].Direction = "both"
			},
			wantErr: "invalid network rule direction",
		},
		{
			name: "network rule invalid protocol",
			modify: func(c *MezhaConfig) {
				c.Microsandbox.Network.Rules[0].Protocol = "sctp"
			},
			wantErr: "invalid network rule protocol",
		},
		{
			name: "files add empty source",
			modify: func(c *MezhaConfig) {
				c.Files.Add[0].Source = ""
			},
			wantErr: "file add source cannot be empty",
		},
		{
			name: "files add empty target",
			modify: func(c *MezhaConfig) {
				c.Files.Add[0].Target = ""
			},
			wantErr: "file add target cannot be empty",
		},
		{
			name: "files add non-existent source",
			modify: func(c *MezhaConfig) {
				c.Files.Add[0].Source = filepath.Join(tempDir, "nonexistent-file")
			},
			wantErr: "file add source",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := cloneConfig(validCfg)
			tt.modify(c)
			err := c.Validate()
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error %q does not contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestRedacted(t *testing.T) {
	cfg := &MezhaConfig{
		SecretSpec: SecretSpecConfig{
			Enabled:  true,
			Profile:  "my-profile",
			Scope:    "read:all",
			Reason:   "production-token",
			Provider: "vault",
		},
		Microsandbox: &MicrosandboxSpec{
			Secrets: []MicrosandboxSecret{
				{EnvVar: "API_KEY", Value: "supersecret123"},
			},
			Network: MicrosandboxNetwork{
				TLS: &MicrosandboxTLS{
					CAKey:  "private-key-data",
					CACert: "public-cert-data",
				},
			},
		},
	}

	redacted := cfg.Redacted()

	// Redacted copy checks
	if redacted.SecretSpec.Profile != "[REDACTED]" {
		t.Errorf("redacted SecretSpec.Profile = %q, want [REDACTED]", redacted.SecretSpec.Profile)
	}
	if redacted.SecretSpec.Scope != "[REDACTED]" {
		t.Errorf("redacted SecretSpec.Scope = %q, want [REDACTED]", redacted.SecretSpec.Scope)
	}
	if redacted.SecretSpec.Reason != "[REDACTED]" {
		t.Errorf("redacted SecretSpec.Reason = %q, want [REDACTED]", redacted.SecretSpec.Reason)
	}
	if redacted.Microsandbox.Secrets[0].Value != "[REDACTED]" {
		t.Errorf(
			"redacted Secret Value = %q, want [REDACTED]",
			redacted.Microsandbox.Secrets[0].Value,
		)
	}
	if redacted.Microsandbox.Network.TLS.CAKey != "[REDACTED]" {
		t.Errorf(
			"redacted TLS.CAKey = %q, want [REDACTED]",
			redacted.Microsandbox.Network.TLS.CAKey,
		)
	}

	// Non-sensitive fields preserved
	if redacted.SecretSpec.Provider != "vault" {
		t.Errorf("redacted SecretSpec.Provider = %q, want vault", redacted.SecretSpec.Provider)
	}
	if redacted.Microsandbox.Network.TLS.CACert != "public-cert-data" {
		t.Errorf(
			"redacted TLS.CACert = %q, want public-cert-data",
			redacted.Microsandbox.Network.TLS.CACert,
		)
	}

	// Original struct unmodified
	if cfg.SecretSpec.Profile != "my-profile" {
		t.Errorf("original SecretSpec.Profile mutated: %q", cfg.SecretSpec.Profile)
	}
	if cfg.SecretSpec.Scope != "read:all" {
		t.Errorf("original SecretSpec.Scope mutated: %q", cfg.SecretSpec.Scope)
	}
	if cfg.SecretSpec.Reason != "production-token" {
		t.Errorf("original SecretSpec.Reason mutated: %q", cfg.SecretSpec.Reason)
	}
	if cfg.Microsandbox.Secrets[0].Value != "supersecret123" {
		t.Errorf("original Secret Value mutated: %q", cfg.Microsandbox.Secrets[0].Value)
	}
	if cfg.Microsandbox.Network.TLS.CAKey != "private-key-data" {
		t.Errorf("original TLS.CAKey mutated: %q", cfg.Microsandbox.Network.TLS.CAKey)
	}
}
