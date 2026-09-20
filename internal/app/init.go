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
				Usage: "Create the home-level configuration (~/.mezha/mezha.yaml)",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if cmd.Bool("home") {
				return InitHome(cmd.Bool("force"))
			}
			rc, err := ResolveRepoContext(ctx)
			if err != nil {
				return err
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
    pkgs.git
    pkgs.go
    pkgs.procps
  ];

  # These task-managed services run in the same Microsandbox exec namespace as
  # the devenv shell and its requested command.
  tasks = {
    "mezha:docker".exec = ''
      set -eu
      if docker --context default info >/dev/null 2>&1; then
        exit 0
      fi
      rm -f /var/run/docker.pid
      dockerd --host=unix:///var/run/docker.sock --storage-driver=vfs >/tmp/mezha-dockerd.log 2>&1 &
      for _ in $(seq 1 30); do
        docker --context default info >/dev/null 2>&1 && exit 0
        sleep 1
      done
      cat /tmp/mezha-dockerd.log >&2 || true
      exit 1
    '';

    "mezha:k3s" = {
      after = [ "mezha:docker" ];
      exec = ''
        set -eu
        k3s_dir=/var/lib/rancher/k3s
        kubeconfig="$k3s_dir/k3s.yaml"
        mkdir -p "$k3s_dir"
        k3s server --data-dir "$k3s_dir" --node-name mezha-k3s --https-listen-port=16443 --docker --write-kubeconfig "$kubeconfig" --write-kubeconfig-mode 644 >/tmp/mezha-k3s.log 2>&1 &
        k3s_pid=$!
        for _ in $(seq 1 90); do
          if kubectl --kubeconfig "$kubeconfig" get nodes --no-headers 2>/dev/null | awk '$2 ~ /^Ready/ { ready=1 } END { exit !ready }'; then
            mkdir -p "$HOME/.kube"
            cp "$kubeconfig" "$HOME/.kube/config"
            exit 0
          fi
          if ! kill -0 "$k3s_pid" 2>/dev/null; then
            cat /tmp/mezha-k3s.log >&2 || true
            exit 1
          fi
          sleep 1
        done
        cat /tmp/mezha-k3s.log >&2 || true
        exit 1
      '';
    };

    "devenv:enterShell".after = [ "mezha:k3s" ];
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
