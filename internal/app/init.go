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
	configPath := filepath.Join(rc.RepoRoot, "mezha.toml")
	provisionDir := filepath.Join(rc.RepoRoot, ".mezha", "provision")
	devenvPath := filepath.Join(provisionDir, "devenv.nix")
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
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		return fmt.Errorf("create project configuration directory: %w", err)
	}
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write configuration file: %w", err)
	}
	if err := writeProvisionFiles(provisionDir, force); err != nil {
		return fmt.Errorf("write managed devenv provision configuration: %w", err)
	}

	fmt.Printf("Created %s\n", configPath)
	fmt.Printf("Created %s\n", provisionDir)
	return nil
}

func writeProvisionFiles(provisionDir string, force bool) error {
	files, err := defaultProvisionFiles()
	if err != nil {
		return err
	}
	if !force {
		for _, file := range files {
			target := filepath.Join(provisionDir, file.relPath)
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
		target := filepath.Join(provisionDir, file.relPath)
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
	provisionDir := filepath.Join(filepath.Dir(configPath), ".mezha", "provision")
	devenvPath := filepath.Join(provisionDir, "devenv.nix")
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
	if err := os.WriteFile(configPath, []byte(DefaultConfigTemplate()), 0o644); err != nil {
		return fmt.Errorf("write home configuration file: %w", err)
	}
	if err := writeProvisionFiles(provisionDir, force); err != nil {
		return fmt.Errorf("write managed devenv provision configuration: %w", err)
	}
	fmt.Printf("Created %s\n", configPath)
	fmt.Printf("Created %s\n", provisionDir)
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
	configPath := filepath.Join(configDir, "mezha.toml")
	provisionDir := filepath.Join(configDir, ".mezha", "provision")
	devenvPath := filepath.Join(provisionDir, "devenv.nix")
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
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		return fmt.Errorf("create %s configuration directory: %w", scope, err)
	}
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write %s configuration file: %w", scope, err)
	}
	if err := writeProvisionFiles(provisionDir, force); err != nil {
		return fmt.Errorf("write managed devenv provision configuration: %w", err)
	}
	fmt.Printf("Created %s\n", configPath)
	fmt.Printf("Created %s\n", provisionDir)
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

func resolveProvisionDir(repoRoot string) string {
	if repoRoot != "" {
		p := filepath.Join(repoRoot, ".mezha", "provision")
		if info, err := os.Stat(p); err == nil && info.IsDir() {
			return p
		}
	}
	if homeDir, err := HomeConfigDir(); err == nil {
		if repoRoot != "" {
			if projectName, worktreeName, gitRef, err := configScopeNames(repoRoot); err == nil {
				candidates := []string{
					filepath.Join(
						homeDir,
						"sandboxes",
						slugify(projectName+"-"+gitRef),
						".mezha",
						"provision",
					),
					filepath.Join(
						homeDir,
						"worktrees",
						slugify(projectName+"-"+worktreeName),
						".mezha",
						"provision",
					),
					filepath.Join(homeDir, "projects", slugify(projectName), ".mezha", "provision"),
				}
				for _, c := range candidates {
					if info, err := os.Stat(c); err == nil && info.IsDir() {
						return c
					}
				}
			}
		}
		c := filepath.Join(homeDir, ".mezha", "provision")
		if info, err := os.Stat(c); err == nil && info.IsDir() {
			return c
		}
	}
	if repoRoot != "" {
		return filepath.Join(repoRoot, ".mezha", "provision")
	}
	return ""
}
