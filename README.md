# mezha

Declarative agent sandboxes powered by Microsandbox.

## Features

- project configuration in `mezha.yaml`
- create, provision, start, stop, run, rebuild, and destroy Microsandbox sandboxes
- synchronize committed changes through a sandbox Git remote
- upload and download dirty working-tree changes or selected files
- use the native `ghcr.io/cachix/devenv/devenv:latest` image for sandbox tooling
- provision Docker, k3s, kubectl, Git, Lazygit, and GitHub CLI declaratively through a Mezha-managed devenv environment
- optionally run a single-node k3s server in the primary Microsandbox

## Requirements

- Go
- Microsandbox-supported local virtualization host (KVM on Linux or Apple Silicon on macOS)
- a local Docker-compatible daemon to import the native devenv image

## Build

```bash
make build
```

`make build` enables CGO because Mezha uses the Microsandbox Go SDK.

## Usage

```bash
mezha init [options]
mezha list
mezha volumes --list|--destroy <name> [options]
mezha run [options] [-- command...]
mezha provision [options]
mezha recreate [options]
mezha start [options]
mezha stop [options]
mezha destroy [options]
mezha upload [options] [local-path] [remote-path]
mezha download [options] [remote-path] [local-path]
mezha pull
mezha push
mezha status [options]
mezha remote repair [options]
mezha logs [options]
```

Examples:

```bash
mezha init
mezha list
mezha volumes --list
mezha run
mezha run -- git status
mezha run --herdr true -- git status
# Create the sandbox and start its core services without synchronizing the repository.
mezha provision
mezha provision --herdr true
# Recreate the sandbox and its persistent state, then provision it.
mezha recreate --herdr true
# Reuse one sandbox for this and other projects.
mezha run --sandbox shared-dev -- git status
mezha upload
mezha download reports/result.json ./result.json
mezha pull
mezha push
# Stop the sandbox while retaining it, then start it again later.
mezha stop
mezha start
mezha destroy
# List all local sandboxes and volumes. Volume output includes capacity and users.
mezha list
mezha volumes --list
# Remove an unused volume after confirmation.
mezha volumes --destroy shared-dev-state
# Remove the sandbox and its retained named volumes.
mezha destroy --volumes-flush true
```

## Configuration

Run `mezha init` in a repository to create `mezha.yaml` and
`.mezha/devenv.nix`. By default, Mezha generates a sandbox name for the current
repository and Git ref. Use `--sandbox <name>` on `run`, synchronization,
transfer, logs, and destroy commands to select a reusable sandbox instead.
Each project is kept in its own `/sandbox/<project>` directory. A Git remote
named after the selected sandbox is added to the host repository, so the same
repository can synchronize with multiple sandboxes. The legacy `--name` option
remains an alias for `--sandbox`.

Run `mezha init --home` to create a default configuration at
`$MEZHA_HOME/mezha.yaml` (`~/.mezha/mezha.yaml` when `MEZHA_HOME` is unset).
Create a scoped home configuration from a Git checkout with one of:

```sh
mezha init --home --project
mezha init --home --worktree
mezha init --home --sandbox
```

The scope flags are mutually exclusive. They create the project, current
worktree, or current branch sandbox configuration, respectively.
Mezha selects one configuration rather than merging layers. The most specific
existing file is used; precedence increases in this order:

1. `$MEZHA_HOME/mezha.yaml`
2. `$MEZHA_HOME/projects/<git-project>/mezha.yaml`
3. `$MEZHA_HOME/worktrees/<git-project>-<worktree>/mezha.yaml`
4. `$MEZHA_HOME/sandboxes/<git-project>-<git-branch>/mezha.yaml`
5. `<project-root>/mezha.yaml`

The project, worktree, and sandbox directory names use Mezha's safe sandbox
name format; linked worktrees use the primary repository's name for
`<git-project>`. Mezha always uses `ghcr.io/cachix/devenv/devenv:latest` and
runs it as UID 0: Docker and k3s
require it, and Microsandbox cannot resolve the native image's `1000:100` user
declaration. Mezha uploads its managed devenv configuration during sandbox
creation. Relative paths in `provision.add` are resolved relative to the
configuration file.

```yaml
version: 1

microsandbox:
  # Mezha always uses ghcr.io/cachix/devenv/devenv:latest as UID 0.
  memory_mib: 4096
  volumes:
    # All persistent state shares this volume.
    - name: state
      target: /nix
      mode: ensure-exists
      kind: disk
      size_mib: 51200
  # network:
  #   default_egress: deny
  #   rules:
  #     - action: allow
  #       direction: egress
  #       destination: public

services:
  docker:
    # Docker is supplied by the Mezha-managed devenv environment.
    # Start dockerd when the sandbox is created or reused.
    enabled: true
  k3s:
    # Run a k3s server alongside each Mezha session.
    enabled: true

sandbox:
  # Default sandbox selection; --sandbox overrides it.
  # name: development
  # remote_dir: /sandbox/my-project
  # policy_advisor: true
  # Register the sandbox with Herdr and synchronize local plugins.
  herdr: false

provision:
  # This devenv.nix declaratively provides Docker, k3s, kubectl, Git, Lazygit, and GitHub CLI.
  add:
    - [.mezha/devenv.nix, /sandbox/devenv.nix]

run: []
```

`provision.add` and `provision.run` are applied only when a sandbox is first created.
`mezha provision` performs this initialization and verifies the configured core
devenv services, but does not publish, upload, or otherwise synchronize repository
data. Mezha seeds the complete `/nix` directory into the shared `state` volume, which
is mounted at `/nix`. The temporary state-volume provisioning sandbox receives
the same `microsandbox.env`, `microsandbox.network`, and `microsandbox.secrets`
configuration (including secret host allowlists) as the primary sandbox. It then symlinks `/home`, `/root`, `/sandbox`,
`/var/lib/docker`, and `/var/lib/rancher/k3s` into that volume before any
initialization command or devenv shell runs. Mezha defaults `GOPATH` to `/sandbox/go` unless it is
explicitly configured in `microsandbox.env`. The default `provision.add` installs
Mezha's `.mezha/devenv.nix` at
`/sandbox/devenv.nix`. It uses devenv `packages` for Docker, k3s, kubectl, Git,
Lazygit, GitHub CLI, Go, and Groff. Mezha starts Docker and k3s in the same Microsandbox
exec job as each requested command or interactive session, waits for readiness,
and configures `kubectl`. This is required because separate Microsandbox exec
jobs have isolated runtime namespaces. Update that file and run `mezha run --recreate` to
apply a changed managed environment.

Set `services.docker.enabled: true` to start the Docker daemon before configured
or requested commands. Set `services.k3s.enabled: true` to run k3s alongside
each command or interactive session and configure `kubectl` to use the local
cluster. Set it to `false` to disable k3s. Docker images, containers,
and volumes plus k3s cluster state are stored in the shared persistent volume.
k3s uses Docker as its container runtime, so Docker-built images are immediately
available to Kubernetes. Named volume names are automatically prefixed with the
sandbox name, so each sandbox receives its own volume. Volumes are retained when
the sandbox is recreated or destroyed; this includes the Nix store, the complete
`/sandbox` workspace, `/home`, and `/root` with their caches and configuration, avoiding
repeated downloads and evaluation after
`mezha
run --recreate`. Use `--volumes-flush` with
`mezha destroy`, or with `mezha run --recreate`, only when a clean set of
persistent volumes is required. Entries in `run` execute before the requested
command. Strings use
shell form; YAML sequences use exec form.

## Herdr integration

When the local `herdr` command is installed, register the sandbox as a saved
Herdr SSH machine while provisioning or running it with:

```bash
mezha provision --herdr true
# Or provision, synchronize the repository, and open a session.
mezha run --herdr true
```

To enable this by default for the project, set it in `mezha.yaml` (Mezha does
not currently use a `mezha.nix` configuration file):

```yaml
sandbox:
  herdr: true
```

The default is `false`; an explicit `--herdr true` or `--herdr false` on
`provision` or `run` overrides the configured value for that invocation.

The registration is idempotent and uses Mezha's sandbox SSH proxy. Mezha
installs the matching Linux Herdr release in the sandbox before registration,
then records its SSH host key for Herdr's strict saved-machine connection.
New Herdr panes use Mezha's configured working directory and shell environment.
Mezha also natively installs GitHub-managed local Herdr plugins in the sandbox,
preserves their enabled state, and copies each plugin's local configuration
directory. Plugin installation and build commands run through Mezha's managed
`devenv` environment. The generated `.mezha/devenv.nix` includes Go for native
plugin builds; add other plugin-specific build tools there. `mezha destroy`
removes the corresponding saved Herdr machine profile. Herdr
is optional: if its command is not on `PATH`, Mezha skips both operations.

### Herdr plugin

The plugin in `herdr` exposes a `dev.mezha.dashboard` action that opens an
interactive terminal interface in a Herdr-managed overlay pane. The dashboard
provides shortcuts for `run`, `provision`, `recreate`, `start`, `stop`, `status`,
`upload`, `download`, `pull`, `push`, and `destroy`, plus a command entry that accepts any
Mezha CLI command and arguments. It does not install keybindings.

Install it from GitHub:

```bash
herdr plugin install odzhu/mezha/herdr
```

The `mezha` executable must be available on the environment inherited by Herdr.
The dashboard follows the herdr-plus launcher-and-pane architecture. Run opens
the interactive sandbox shell in a new tab, while status and destroy use
popups. See `herdr/README.md` for local development instructions.

## Environment variables

- `SANDBOX_NAME`
- `MICROSANDBOX_REMOTE_REPO_DIR`
- `MICROSANDBOX_POLICY_ADVISOR`

## Development

```bash
make fmt
make test
```
