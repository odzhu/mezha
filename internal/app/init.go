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
			&cli.BoolFlag{Name: "force", Aliases: []string{"f"}, Usage: "Overwrite an existing mezha.yaml"},
			&cli.BoolFlag{Name: "home", Usage: "Create the home-level configuration (~/.mezha/mezha.yaml)"},
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
	dockerfilePath := filepath.Join(rc.RepoRoot, ".mezha", "Dockerfile")
	if !force {
		for _, path := range []string{configPath, filepath.Join(rc.RepoRoot, "mezha.yml"), dockerfilePath} {
			if _, err := os.Stat(path); err == nil {
				return fmt.Errorf("configuration file already exists: %s (use --force to overwrite)", path)
			} else if !os.IsNotExist(err) {
				return fmt.Errorf("inspect configuration file: %w", err)
			}
		}
	}

	content, err := projectConfigTemplate()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dockerfilePath), 0o755); err != nil {
		return fmt.Errorf("create project configuration directory: %w", err)
	}
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write configuration file: %w", err)
	}
	if err := os.WriteFile(dockerfilePath, []byte(defaultNixDockerfile), 0o644); err != nil {
		return fmt.Errorf("write project Dockerfile: %w", err)
	}

	fmt.Printf("Created %s\n", configPath)
	fmt.Printf("Created %s\n", dockerfilePath)
	return nil
}

const defaultNixDockerfile = `FROM debian:trixie-slim

# nix-bin is packaged under /usr, so it stays executable when the persistent
# named volume is mounted at /nix during sandbox creation.
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates git nix-bin procps \
    && rm -rf /var/lib/apt/lists/* \
    && mkdir -p /nix /etc/nix \
    && printf 'sandbox = false\nbuild-users-group =\nexperimental-features = nix-command flakes\n' > /etc/nix/nix.conf

ENV PATH=/usr/bin:/bin
`

// InitHome creates the home-level configuration without requiring a Git
// repository.
func InitHome(force bool) error {
	configPath, err := HomeConfigPath()
	if err != nil {
		return err
	}
	dockerfilePath := filepath.Join(filepath.Dir(configPath), ".mezha", "Dockerfile")
	if !force {
		for _, path := range []string{configPath, dockerfilePath} {
			if _, err := os.Stat(path); err == nil {
				return fmt.Errorf("configuration file already exists: %s (use --force to overwrite)", path)
			} else if !os.IsNotExist(err) {
				return fmt.Errorf("inspect configuration file: %w", err)
			}
		}
	}
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		return fmt.Errorf("create home configuration directory: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(dockerfilePath), 0o755); err != nil {
		return fmt.Errorf("create home Dockerfile directory: %w", err)
	}
	if err := os.WriteFile(configPath, []byte(DefaultConfigTemplate()), 0o644); err != nil {
		return fmt.Errorf("write home configuration file: %w", err)
	}
	if err := os.WriteFile(dockerfilePath, []byte(defaultNixDockerfile), 0o644); err != nil {
		return fmt.Errorf("write default Dockerfile: %w", err)
	}
	fmt.Printf("Created %s\n", configPath)
	fmt.Printf("Created %s\n", dockerfilePath)
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
