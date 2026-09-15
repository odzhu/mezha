package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	cli "github.com/urfave/cli/v3"
)

const rootUsageText = `Usage:
  mezha init [options]
  mezha run [options] [-- command...]
  mezha destroy [options]
  mezha upload [options] [local-path] [remote-path]
  mezha download [options] [remote-path] [local-path]
  mezha pull
  mezha push
  mezha status [options]
  mezha remote repair [options]
  mezha logs [options]
  mezha help

Creates or reuses a sandbox for the current git repo and opens a shell.
Repository contents are synchronized through the sandbox Git remote.
Use "mezha upload" and "mezha download" without paths to synchronize uncommitted tracked changes and untracked files.
Pass paths to transfer an individual file or folder. Use "mezha pull" and "mezha push" to synchronize committed changes.

Use the run command to pass options and, optionally, a command after --
to run it in the repo directory instead of opening an interactive shell.

Mezha uses ghcr.io/cachix/devenv/devenv:latest and provisions Docker and k3s
through its managed devenv environment.

Examples:
  mezha init
  mezha init --home
  mezha run
  mezha run --recreate
  mezha run --tty
  mezha run -- ls -la
  mezha run -- bash -lc 'git status && pwd'
  mezha destroy
  mezha destroy --force
  mezha upload # synchronize local dirty changes and untracked files
  mezha download # synchronize sandbox dirty changes and untracked files
  mezha upload ./notes.txt notes.txt
  mezha upload ./assets /tmp/assets
  mezha download results/report.json ./report.json
  mezha download /tmp/assets ./assets
  mezha pull
  mezha push
  mezha status
  mezha remote repair
  mezha logs
  mezha logs -f
  mezha logs --tail 200 --source sandbox

Environment variables:
  SANDBOX_NAME                Override the generated sandbox name
  MICROSANDBOX_REMOTE_REPO_DIR   Destination directory in the sandbox
                              (default: /sandbox/<repo-name>)
  MICROSANDBOX_POLICY_ADVISOR    Enable or disable the Microsandbox policy advisor (default: true)`

func New() *cli.Command {
	return &cli.Command{
		Name:      "mezha",
		Usage:     "Microsandbox sandbox helper for the current Git repository",
		UsageText: rootUsageText,
		Commands: []*cli.Command{
			newInitCommand(),
			newRunCommand(),
			newDestroyCommand(),
			newUploadCommand(),
			newDownloadCommand(),
			newPullCommand(),
			newPushCommand(),
			newStatusCommand(),
			newRemoteCommand(),
			newLogsCommand(),
			newSSHProxyCommand(),
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			return cli.ShowRootCommandHelp(cmd)
		},
	}
}

func newSSHProxyCommand() *cli.Command {
	return &cli.Command{
		Name:   "ssh-proxy",
		Usage:  "Relay an SSH client connection to a Microsandbox",
		Hidden: true,
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "name", Usage: "Sandbox name", Required: true},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			return SSHProxy(ctx, cmd.String("name"))
		},
	}
}

func newRunCommand() *cli.Command {
	flags := []cli.Flag{
		&cli.BoolFlag{
			Name:  "recreate",
			Usage: "Delete and recreate the sandbox if it already exists",
		},
		&cli.BoolFlag{
			Name:    "no-advisor",
			Aliases: []string{"no-policy-advisor"},
			Usage:   "Disable the Microsandbox policy advisor",
		},
		&cli.BoolFlag{
			Name:    "policy-advisor",
			Aliases: []string{"advisor"},
			Usage:   "Enable the Microsandbox policy advisor (default: true)",
		},
		&cli.BoolFlag{Name: "tty", Usage: "Force an interactive terminal session"},
		&cli.BoolFlag{Name: "no-tty", Usage: "Disable interactive terminal mode"},
		&cli.StringFlag{
			Name:  "editor",
			Usage: "Open a remote editor instead of an interactive shell",
		},
		&cli.StringFlag{Name: "name", Usage: "Override the generated sandbox name"},
		&cli.StringFlag{Name: "remote-dir", Usage: "Destination directory in the sandbox"},
		&cli.BoolFlag{
			Name:  "replace-sandbox-remote",
			Usage: "Replace an existing non-Mezha sandbox Git remote",
		},
		&cli.BoolFlag{
			Name:  "no-login-shell",
			Usage: "Skip shell login/profile startup files when running the command or session",
		},
	}
	return &cli.Command{
		Name:      "run",
		Usage:     "Create or reuse a sandbox and open a session",
		UsageText: rootUsageText,
		Flags:     flags,
		Action: func(ctx context.Context, cmd *cli.Command) error {
			rc, err := ResolveRepoContext(ctx)
			if err != nil {
				return err
			}
			cfg, _, err := LoadConfig(rc.RepoRoot)
			if err != nil {
				return fmt.Errorf("load mezha configuration: %w", err)
			}
			if cfg == nil {
				cfg = &MezhaConfig{}
			}

			remoteArgs := commandArgs(cmd)
			if cmd.Bool("tty") && cmd.Bool("no-tty") {
				return errors.New("--tty cannot be used together with --no-tty")
			}
			var tty *bool
			if cmd.Bool("tty") {
				value := true
				tty = &value
			} else if cmd.Bool("no-tty") {
				value := false
				tty = &value
			}

			recreate := resolveBoolParam(cmd, "recreate", cfg.Sandbox.Recreate)
			advisor := resolveAdvisorParam(cmd, cfg)

			kubernetes := cfg.Sandbox.Kubernetes
			if cfg.Kubernetes != nil {
				kubernetes = *cfg.Kubernetes
			}

			params := RunParams{
				SandboxName: resolveParam(
					cmd,
					"name",
					os.Getenv("SANDBOX_NAME"),
					cfg.Sandbox.Name,
					rc.DefaultSandboxName,
				),
				RemoteRepoDir: resolveParam(
					cmd,
					"remote-dir",
					os.Getenv("MICROSANDBOX_REMOTE_REPO_DIR"),
					cfg.Sandbox.RemoteDir,
					filepath.ToSlash(filepath.Join("/sandbox", rc.RepoName)),
				),
				Recreate:             recreate,
				Kubernetes:           kubernetes,
				ReplaceSandboxRemote: cmd.Bool("replace-sandbox-remote"),
				Editor:               resolveParam(cmd, "editor", "", cfg.Sandbox.Editor, ""),
				RemoteCommand:        remoteArgs,
				TTY:                  tty,
				PolicyAdvisor:        advisor,
				NoLoginShell: resolveBoolParam(
					cmd,
					"no-login-shell",
					cfg.Sandbox.NoLoginShell,
				),
			}

			if params.Editor != "" && len(params.RemoteCommand) > 0 {
				return errors.New("--editor cannot be used together with a command after --")
			}

			return Run(ctx, rc, params)
		},
	}
}

func newDestroyCommand() *cli.Command {
	flags := []cli.Flag{
		&cli.StringFlag{Name: "name", Usage: "Override the generated sandbox name"},
		&cli.BoolFlag{
			Name:    "force",
			Aliases: []string{"f"},
			Usage:   "Delete without prompting for confirmation",
		},
	}
	return &cli.Command{
		Name:  "destroy",
		Usage: "Delete the Microsandbox for the current repository",
		Flags: flags,
		Action: func(ctx context.Context, cmd *cli.Command) error {
			rc, err := ResolveRepoContext(ctx)
			if err != nil {
				return err
			}
			cfg, _, err := LoadConfig(rc.RepoRoot)
			if err != nil {
				return fmt.Errorf("load mezha configuration: %w", err)
			}
			if cfg == nil {
				cfg = &MezhaConfig{}
			}
			params := DestroyParams{
				SandboxName: resolveParam(
					cmd,
					"name",
					os.Getenv("SANDBOX_NAME"),
					cfg.Sandbox.Name,
					rc.DefaultSandboxName,
				),
				Force: cmd.Bool("force"),
			}
			return Destroy(ctx, rc, params)
		},
	}
}

func transferCommandFlags(includeRecreate bool) []cli.Flag {
	flags := []cli.Flag{
		&cli.StringFlag{Name: "name", Usage: "Override the generated sandbox name"},
		&cli.StringFlag{Name: "remote-dir", Usage: "Sandbox repository directory"},
	}
	if includeRecreate {
		flags = append(
			flags,
			&cli.BoolFlag{
				Name:  "recreate",
				Usage: "Delete and recreate the sandbox before uploading",
			},
		)
	}
	return flags
}

func loadTransferParams(cmd *cli.Command, rc RepoContext) (TransferParams, error) {
	cfg, _, err := LoadConfig(rc.RepoRoot)
	if err != nil {
		return TransferParams{}, fmt.Errorf("load mezha configuration: %w", err)
	}
	if cfg == nil {
		cfg = &MezhaConfig{}
	}
	return TransferParams{
		SandboxName: resolveParam(
			cmd,
			"name",
			os.Getenv("SANDBOX_NAME"),
			cfg.Sandbox.Name,
			rc.DefaultSandboxName,
		),
		RemoteRepoDir: resolveParam(
			cmd,
			"remote-dir",
			os.Getenv("MICROSANDBOX_REMOTE_REPO_DIR"),
			cfg.Sandbox.RemoteDir,
			filepath.ToSlash(filepath.Join("/sandbox", rc.RepoName)),
		),
		Recreate: cmd.Bool("recreate"),
	}, nil
}

func newUploadCommand() *cli.Command {
	return &cli.Command{
		Name:      "upload",
		Usage:     "Upload repository dirty state or one file or folder to the sandbox",
		ArgsUsage: "[local-path] [remote-path]",
		Flags:     transferCommandFlags(true),
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if cmd.NArg() > 2 {
				return errors.New("upload accepts an optional local path and remote path")
			}
			rc, err := ResolveRepoContext(ctx)
			if err != nil {
				return err
			}
			params, err := loadTransferParams(cmd, rc)
			if err != nil {
				return err
			}
			if cmd.NArg() == 0 {
				return Upload(ctx, rc, UploadParams{
					SandboxName:   params.SandboxName,
					RemoteRepoDir: params.RemoteRepoDir,
					Recreate:      params.Recreate,
				})
			}
			remotePath := ""
			if cmd.NArg() == 2 {
				remotePath = cmd.Args().Get(1)
			}
			return UploadPath(ctx, rc, params, cmd.Args().First(), remotePath)
		},
	}
}

func newDownloadCommand() *cli.Command {
	return &cli.Command{
		Name:      "download",
		Usage:     "Download repository dirty state or one file or folder from the sandbox",
		ArgsUsage: "[remote-path] [local-path]",
		Flags:     transferCommandFlags(false),
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if cmd.NArg() > 2 {
				return errors.New("download accepts an optional remote path and local path")
			}
			rc, err := ResolveRepoContext(ctx)
			if err != nil {
				return err
			}
			params, err := loadTransferParams(cmd, rc)
			if err != nil {
				return err
			}
			if cmd.NArg() == 0 {
				return Download(ctx, rc, DownloadParams{
					SandboxName: params.SandboxName, RemoteRepoDir: params.RemoteRepoDir,
				})
			}
			localPath := ""
			if cmd.NArg() == 2 {
				localPath = cmd.Args().Get(1)
			}
			return DownloadPath(ctx, rc, params, cmd.Args().First(), localPath)
		},
	}
}

func gitCommandFlags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{Name: "name", Usage: "Override the generated sandbox name"},
		&cli.StringFlag{Name: "remote-dir", Usage: "Sandbox repository directory"},
	}
}

func loadGitParams(cmd *cli.Command, rc RepoContext) (GitParams, error) {
	cfg, _, err := LoadConfig(rc.RepoRoot)
	if err != nil {
		return GitParams{}, fmt.Errorf("load mezha configuration: %w", err)
	}
	if cfg == nil {
		cfg = &MezhaConfig{}
	}
	return GitParams{
		SandboxName: resolveParam(
			cmd,
			"name",
			os.Getenv("SANDBOX_NAME"),
			cfg.Sandbox.Name,
			rc.DefaultSandboxName,
		),
		RemoteRepoDir: resolveParam(
			cmd,
			"remote-dir",
			os.Getenv("MICROSANDBOX_REMOTE_REPO_DIR"),
			cfg.Sandbox.RemoteDir,
			filepath.ToSlash(filepath.Join("/sandbox", rc.RepoName)),
		),
	}, nil
}

func newPullCommand() *cli.Command {
	return &cli.Command{
		Name:  "pull",
		Usage: "Fast-forward the current branch from the sandbox Git remote",
		Flags: append(
			gitCommandFlags(),
			&cli.BoolFlag{Name: "rebase", Usage: "Rebase instead of fast-forward only"},
			&cli.BoolFlag{Name: "merge", Usage: "Allow a merge commit"},
		),
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if cmd.Bool("rebase") && cmd.Bool("merge") {
				return errors.New("--rebase cannot be used together with --merge")
			}
			rc, err := ResolveRepoContext(ctx)
			if err != nil {
				return err
			}
			params, err := loadGitParams(cmd, rc)
			if err != nil {
				return err
			}
			return PullSandboxBranch(ctx, rc, params, cmd.Bool("rebase"), cmd.Bool("merge"))
		},
	}
}

func newPushCommand() *cli.Command {
	return &cli.Command{
		Name:  "push",
		Usage: "Push the current branch to the sandbox Git remote",
		Flags: append(
			gitCommandFlags(),
			&cli.BoolFlag{
				Name:  "force-with-lease",
				Usage: "Force push only when the sandbox branch has not changed",
			},
		),
		Action: func(ctx context.Context, cmd *cli.Command) error {
			rc, err := ResolveRepoContext(ctx)
			if err != nil {
				return err
			}
			params, err := loadGitParams(cmd, rc)
			if err != nil {
				return err
			}
			return PushSandboxBranch(ctx, rc, params, cmd.Bool("force-with-lease"))
		},
	}
}

func newStatusCommand() *cli.Command {
	return &cli.Command{
		Name:  "status",
		Usage: "Show local and sandbox Git synchronization status",
		Flags: gitCommandFlags(),
		Action: func(ctx context.Context, cmd *cli.Command) error {
			rc, err := ResolveRepoContext(ctx)
			if err != nil {
				return err
			}
			params, err := loadGitParams(cmd, rc)
			if err != nil {
				return err
			}
			return SandboxGitStatus(ctx, rc, params)
		},
	}
}

func newRemoteCommand() *cli.Command {
	return &cli.Command{
		Name:  "remote",
		Usage: "Manage the sandbox Git remote",
		Commands: []*cli.Command{
			{
				Name:  "repair",
				Usage: "Refresh the sandbox remote SSH configuration",
				Flags: append(
					gitCommandFlags(),
					&cli.BoolFlag{
						Name:  "replace-sandbox-remote",
						Usage: "Replace an existing non-Mezha sandbox Git remote",
					},
				),
				Action: func(ctx context.Context, cmd *cli.Command) error {
					rc, err := ResolveRepoContext(ctx)
					if err != nil {
						return err
					}
					params, err := loadGitParams(cmd, rc)
					if err != nil {
						return err
					}
					return RepairSandboxGitRemote(
						ctx,
						rc,
						params,
						cmd.Bool("replace-sandbox-remote"),
					)
				},
			},
		},
	}
}

func newLogsCommand() *cli.Command {
	flags := []cli.Flag{
		&cli.StringFlag{Name: "name", Usage: "Override the generated sandbox name"},
		&cli.UintFlag{Name: "tail", Usage: "Maximum number of log lines to return", Value: 100},
		&cli.BoolFlag{
			Name:    "follow",
			Aliases: []string{"f"},
			Usage:   "Follow logs until interrupted",
		},
		&cli.DurationFlag{
			Name:  "since",
			Usage: "Only show logs from the last duration (for example, 15m or 1h)",
		},
		&cli.StringSliceFlag{
			Name:  "source",
			Usage: "Filter by Microsandbox log source; repeat for multiple sources",
		},
		&cli.StringFlag{Name: "level", Usage: "Minimum log level to include (for example, WARN)"},
	}
	return &cli.Command{
		Name:  "logs",
		Usage: "Show logs for the current repository's sandbox",
		Flags: flags,
		Action: func(ctx context.Context, cmd *cli.Command) error {
			rc, err := ResolveRepoContext(ctx)
			if err != nil {
				return err
			}
			cfg, _, _ := LoadConfig(rc.RepoRoot)
			if cfg == nil {
				cfg = &MezhaConfig{}
			}
			return Logs(ctx, rc, LogsParams{
				SandboxName: resolveParam(
					cmd,
					"name",
					os.Getenv("SANDBOX_NAME"),
					cfg.Sandbox.Name,
					rc.DefaultSandboxName,
				),
				Tail:     cmd.Uint("tail"),
				Follow:   cmd.Bool("follow"),
				Since:    cmd.Duration("since"),
				Sources:  cmd.StringSlice("source"),
				MinLevel: cmd.String("level"),
			})
		},
	}
}

func commandArgs(cmd *cli.Command) []string {
	args := make([]string, 0, cmd.NArg())
	for i := 0; i < cmd.NArg(); i++ {
		args = append(args, cmd.Args().Get(i))
	}
	return args
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func resolveParam(
	cmd *cli.Command,
	flagName string,
	envVal string,
	cfgVal string,
	defaultVal string,
) string {
	var value string
	switch {
	case cmd != nil && cmd.IsSet(flagName):
		value = cmd.String(flagName)
	case strings.TrimSpace(envVal) != "":
		value = envVal
	case strings.TrimSpace(cfgVal) != "":
		value = cfgVal
	default:
		value = defaultVal
	}

	// Every current --name option identifies an Microsandbox sandbox. Normalize
	// overrides too, rather than only generated names, because Microsandbox limits
	// sandbox names to 19 characters.
	if flagName == "name" {
		return slugify(value)
	}
	return value
}

func resolveBoolParam(cmd *cli.Command, flagName string, cfgVal bool) bool {
	if cmd != nil && cmd.IsSet(flagName) {
		return cmd.Bool(flagName)
	}
	return cfgVal
}

func resolveAdvisorParam(cmd *cli.Command, cfg *MezhaConfig) *bool {
	if cmd != nil {
		if cmd.Bool("no-advisor") || cmd.Bool("no-policy-advisor") {
			val := false
			return &val
		}
		if cmd.IsSet("policy-advisor") || cmd.IsSet("advisor") {
			val := cmd.Bool("policy-advisor") || cmd.Bool("advisor")
			return &val
		}
	}

	if envAdvisor := firstNonEmpty(
		os.Getenv("MICROSANDBOX_POLICY_ADVISOR"),
		os.Getenv("MEZHA_POLICY_ADVISOR"),
		os.Getenv("MICROSANDBOX_ADVISOR"),
	); envAdvisor != "" {
		val := parseBool(envAdvisor, true)
		return &val
	}

	if cfg != nil {
		if cfg.Sandbox.PolicyAdvisor != nil {
			return cfg.Sandbox.PolicyAdvisor
		}
		if cfg.Sandbox.Advisor != nil {
			return cfg.Sandbox.Advisor
		}
	}

	val := true
	return &val
}
