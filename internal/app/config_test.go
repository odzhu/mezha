package app

import (
	"os"
	"path/filepath"
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
					"mezha-docker": {"start": {"enable": false}},
					"mezha-k3s": {"start": {"enable": false}}
				}
			}`,
			want: false,
		},
		{
			name: "processes enabled",
			input: `{
				"processes": {
					"mezha-docker": {"start": {"enable": true}},
					"mezha-k3s": {"start": {"enable": false}}
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
