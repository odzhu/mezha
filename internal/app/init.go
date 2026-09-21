package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	cli "github.com/urfave/cli/v3"
)

func newInitCommand() *cli.Command {
	return &cli.Command{
		Name:  "init",
		Usage: "Create a project or home-level mezha.yaml configuration file",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "force",
				Aliases: []string{"f"},
				Usage:   "Overwrite an existing mezha.yaml",
			},
			&cli.BoolFlag{
				Name:  "home",
				Usage: "Create the home-level configuration ($MEZHA_HOME/mezha.yaml)",
			},
			&cli.BoolFlag{
				Name:  "project",
				Usage: "With --home, create the current project configuration",
			},
			&cli.BoolFlag{
				Name:  "worktree",
				Usage: "With --home, create the current worktree configuration",
			},
			&cli.BoolFlag{
				Name:  "sandbox",
				Usage: "With --home, create the current branch sandbox configuration",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			scopes := []string{"project", "worktree", "sandbox"}
			selectedScope := ""
			for _, scope := range scopes {
				if !cmd.Bool(scope) {
					continue
				}
				if !cmd.Bool("home") {
					return fmt.Errorf("--%s requires --home", scope)
				}
				if selectedScope != "" {
					return fmt.Errorf("--project, --worktree, and --sandbox are mutually exclusive")
				}
				selectedScope = scope
			}
			if cmd.Bool("home") && selectedScope == "" {
				return InitHome(cmd.Bool("force"))
			}
			rc, err := ResolveRepoContext(ctx)
			if err != nil {
				return err
			}
			if selectedScope != "" {
				return InitHomeScope(rc, selectedScope, cmd.Bool("force"))
			}
			return Init(ctx, rc, cmd.Bool("force"))
		},
	}
}

func Init(_ context.Context, rc RepoContext, force bool) error {
	configPath := filepath.Join(rc.RepoRoot, "mezha.yaml")
	devenvPath := filepath.Join(rc.RepoRoot, ".mezha", "devenv.nix")
	if !force {
		for _, path := range []string{configPath, filepath.Join(rc.RepoRoot, "mezha.yml"), devenvPath} {
			if _, err := os.Stat(path); err == nil {
				return fmt.Errorf(
					"configuration file already exists: %s (use --force to overwrite)",
					path,
				)
			} else if !os.IsNotExist(err) {
				return fmt.Errorf("inspect configuration file: %w", err)
			}
		}
	}

	content, err := projectConfigTemplate()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(devenvPath), 0o755); err != nil {
		return fmt.Errorf("create project configuration directory: %w", err)
	}
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write configuration file: %w", err)
	}
	if err := os.WriteFile(devenvPath, []byte(defaultManagedDevenv), 0o644); err != nil {
		return fmt.Errorf("write managed devenv configuration: %w", err)
	}

	fmt.Printf("Created %s\n", configPath)
	fmt.Printf("Created %s\n", devenvPath)
	return nil
}

const defaultManagedDevenv = `{ pkgs, ... }:

{
  packages = [
    pkgs.docker
    pkgs.k3s
    pkgs.kubectl
    pkgs.less
    pkgs.unixtools.col
    pkgs.git
    pkgs.lazygit
    pkgs.gh
    pkgs.go
    pkgs.groff
    pkgs.procps
  ];

  # Render manpages safely when command output is captured instead of attached
  # to a terminal. grotty then emits overstrikes and col removes them.
  env.GROFF_NO_SGR = "1";
  env.MANPAGER = "col -b";
  env.KUBECONFIG = "/var/lib/rancher/k3s/k3s.yaml";

  # Mezha starts these once with devenv up -d and each session is a client.
  processes.mezha-docker = {
    start.enable = false;
    exec = ''
      rm -f /var/run/docker.pid
      exec dockerd --host=unix:///var/run/docker.sock --storage-driver=vfs
    '';
    ready.exec = "docker info >/dev/null";
    restart.on = "always";
    shutdown.grace = 30;
  };

  processes.mezha-k3s = {
    start.enable = false;
    after = [ "devenv:processes:mezha-docker" ];
    exec = ''
      exec k3s server \
        --data-dir /var/lib/rancher/k3s \
        --node-name mezha-k3s \
        --https-listen-port=16443 \
        --docker \
        --write-kubeconfig /var/lib/rancher/k3s/k3s.yaml \
        --write-kubeconfig-mode 644
    '';
    ready.exec = ''
      kubectl --kubeconfig /var/lib/rancher/k3s/k3s.yaml get nodes --no-headers |
        awk '$2 ~ /^Ready/ { ready=1 } END { exit !ready }'
    '';
    restart.on = "always";
    shutdown.grace = 30;
  };
}
`

// InitHome creates the home-level configuration without requiring a Git
// repository.
func InitHome(force bool) error {
	configPath, err := HomeConfigPath()
	if err != nil {
		return err
	}
	devenvPath := filepath.Join(filepath.Dir(configPath), ".mezha", "devenv.nix")
	if !force {
		for _, path := range []string{configPath, devenvPath} {
			if _, err := os.Stat(path); err == nil {
				return fmt.Errorf(
					"configuration file already exists: %s (use --force to overwrite)",
					path,
				)
			} else if !os.IsNotExist(err) {
				return fmt.Errorf("inspect configuration file: %w", err)
			}
		}
	}
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		return fmt.Errorf("create home configuration directory: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(devenvPath), 0o755); err != nil {
		return fmt.Errorf("create home configuration directory: %w", err)
	}
	if err := os.WriteFile(configPath, []byte(DefaultConfigTemplate()), 0o644); err != nil {
		return fmt.Errorf("write home configuration file: %w", err)
	}
	if err := os.WriteFile(devenvPath, []byte(defaultManagedDevenv), 0o644); err != nil {
		return fmt.Errorf("write managed devenv configuration: %w", err)
	}
	fmt.Printf("Created %s\n", configPath)
	fmt.Printf("Created %s\n", devenvPath)
	return nil
}

// InitHomeScope creates a configuration for one current Git configuration scope.
func InitHomeScope(rc RepoContext, scope string, force bool) error {
	homeDir, err := HomeConfigDir()
	if err != nil {
		return err
	}
	projectName, worktreeName, gitRef, err := configScopeNames(rc.RepoRoot)
	if err != nil {
		return err
	}
	var configDir string
	switch scope {
	case "project":
		configDir = filepath.Join(homeDir, "projects", slugify(projectName))
	case "worktree":
		configDir = filepath.Join(homeDir, "worktrees", slugify(projectName+"-"+worktreeName))
	case "sandbox":
		configDir = filepath.Join(homeDir, "sandboxes", slugify(projectName+"-"+gitRef))
	default:
		return fmt.Errorf("unknown home configuration scope: %s", scope)
	}
	configPath := filepath.Join(configDir, "mezha.yaml")
	devenvPath := filepath.Join(configDir, ".mezha", "devenv.nix")
	if !force {
		for _, path := range []string{configPath, devenvPath} {
			if _, err := os.Stat(path); err == nil {
				return fmt.Errorf(
					"configuration file already exists: %s (use --force to overwrite)",
					path,
				)
			} else if !os.IsNotExist(err) {
				return fmt.Errorf("inspect configuration file: %w", err)
			}
		}
	}
	content, err := projectConfigTemplate()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(devenvPath), 0o755); err != nil {
		return fmt.Errorf("create %s configuration directory: %w", scope, err)
	}
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write %s configuration file: %w", scope, err)
	}
	if err := os.WriteFile(devenvPath, []byte(defaultManagedDevenv), 0o644); err != nil {
		return fmt.Errorf("write managed devenv configuration: %w", err)
	}
	fmt.Printf("Created %s\n", configPath)
	fmt.Printf("Created %s\n", devenvPath)
	return nil
}

// projectConfigTemplate uses the home-level configuration verbatim when it
// exists, so a new project starts from the user's defaults.
func projectConfigTemplate() (string, error) {
	path, err := HomeConfigPath()
	if err != nil {
		return "", err
	}
	contents, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return ProjectDefaultConfigTemplate(), nil
	}
	if err != nil {
		return "", fmt.Errorf("read home configuration template: %w", err)
	}
	return string(contents), nil
}
