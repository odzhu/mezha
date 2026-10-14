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
		Usage: "Create a project or home-level mezha.toml configuration file",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "force",
				Aliases: []string{"f"},
				Usage:   "Overwrite an existing mezha.toml",
			},
			&cli.BoolFlag{
				Name:  "home",
				Usage: "Create the home-level configuration ($MEZHA_HOME/mezha.toml)",
			},
			&cli.BoolFlag{
				Name:  "project",
				Usage: "With --home, create the current project configuration in $MEZHA_HOME",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if cmd.Bool("project") && !cmd.Bool("home") {
				return fmt.Errorf("--project requires --home")
			}
			if cmd.Bool("home") && !cmd.Bool("project") {
				return InitHome(cmd.Bool("force"))
			}
			rc, err := ResolveRepoContext(ctx)
			if err != nil {
				return err
			}
			if cmd.Bool("home") && cmd.Bool("project") {
				return InitHomeProject(rc, cmd.Bool("force"))
			}
			return Init(ctx, rc, cmd.Bool("force"))
		},
	}
}

func Init(_ context.Context, rc RepoContext, force bool) error {
	configPath := filepath.Join(rc.RepoRoot, "mezha.toml")
	extensionsDir := filepath.Join(rc.RepoRoot, ".mezha", "extensions")
	samplePath := filepath.Join(extensionsDir, "sample", "devenv.nix")
	if !force {
		for _, path := range []string{configPath, samplePath} {
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
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		return fmt.Errorf("create project configuration directory: %w", err)
	}
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write configuration file: %w", err)
	}
	if err := writeExtensionFiles(extensionsDir, force); err != nil {
		return fmt.Errorf("write managed devenv extension configuration: %w", err)
	}

	fmt.Printf("Created %s\n", configPath)
	fmt.Printf("Created %s\n", extensionsDir)
	return nil
}

func writeExtensionFiles(extensionsDir string, force bool) error {
	files, err := defaultExtensionFiles()
	if err != nil {
		return err
	}
	if !force {
		for _, file := range files {
			target := filepath.Join(extensionsDir, file.relPath)
			if _, err := os.Stat(target); err == nil {
				return fmt.Errorf(
					"configuration file already exists: %s (use --force to overwrite)",
					target,
				)
			} else if !os.IsNotExist(err) {
				return fmt.Errorf("inspect configuration file: %w", err)
			}
		}
	}
	for _, file := range files {
		target := filepath.Join(extensionsDir, file.relPath)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return fmt.Errorf("create directory %s: %w", filepath.Dir(target), err)
		}
		if err := os.WriteFile(target, []byte(file.content), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", target, err)
		}
	}
	return nil
}

// InitHome creates the home-level configuration without requiring a Git
// repository.
func InitHome(force bool) error {
	configPath, err := HomeConfigPath()
	if err != nil {
		return err
	}
	extensionsDir := filepath.Join(filepath.Dir(configPath), "extensions")
	samplePath := filepath.Join(extensionsDir, "sample", "devenv.nix")
	if !force {
		for _, path := range []string{configPath, samplePath} {
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
	if err := os.WriteFile(configPath, []byte(DefaultConfigTemplate()), 0o644); err != nil {
		return fmt.Errorf("write home configuration file: %w", err)
	}
	if err := writeExtensionFiles(extensionsDir, force); err != nil {
		return fmt.Errorf("write managed devenv extension configuration: %w", err)
	}
	fmt.Printf("Created %s\n", configPath)
	fmt.Printf("Created %s\n", extensionsDir)
	return nil
}

// InitHomeProject creates a project configuration under $MEZHA_HOME/projects/<project>.
func InitHomeProject(rc RepoContext, force bool) error {
	homeDir, err := HomeConfigDir()
	if err != nil {
		return err
	}
	configDir := projectConfigDir(homeDir, rc.RepoRoot)
	if configDir == "" {
		return fmt.Errorf("resolve project directory: invalid repository root")
	}
	configPath := filepath.Join(configDir, "mezha.toml")
	extensionsDir := filepath.Join(configDir, "extensions")
	samplePath := filepath.Join(extensionsDir, "sample", "devenv.nix")
	if !force {
		for _, path := range []string{configPath, samplePath} {
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
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		return fmt.Errorf("create project configuration directory: %w", err)
	}
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write project configuration file: %w", err)
	}
	if err := writeExtensionFiles(extensionsDir, force); err != nil {
		return fmt.Errorf("write managed devenv extension configuration: %w", err)
	}
	fmt.Printf("Created %s\n", configPath)
	fmt.Printf("Created %s\n", extensionsDir)
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

func resolveExtensionsDir(repoRoot string) string {
	if repoRoot != "" {
		p := filepath.Join(repoRoot, ".mezha", "extensions")
		if info, err := os.Stat(p); err == nil && info.IsDir() {
			return p
		}
	}
	if homeDir, err := HomeConfigDir(); err == nil {
		if projectDir := projectConfigDir(homeDir, repoRoot); projectDir != "" {
			p := filepath.Join(projectDir, "extensions")
			if info, err := os.Stat(p); err == nil && info.IsDir() {
				return p
			}
		}
		c := filepath.Join(homeDir, "extensions")
		if info, err := os.Stat(c); err == nil && info.IsDir() {
			return c
		}
	}
	if repoRoot != "" {
		return filepath.Join(repoRoot, ".mezha", "extensions")
	}
	if homeDir, err := HomeConfigDir(); err == nil {
		return filepath.Join(homeDir, "extensions")
	}
	return ""
}
