# mezha

Declarative agent sandboxes powered by Microsandbox.

## Features

- project configuration in `mezha.toml`
- create, provision, start, stop, run, rebuild, and destroy Microsandbox sandboxes
- synchronize committed changes through a sandbox Git remote
- upload and download dirty working-tree changes or selected files
- use the native `ghcr.io/cachix/devenv/devenv:latest` image for sandbox tooling
- provision Docker, k3s, kubectl, Git, Lazygit, and GitHub CLI declaratively through a Mezha-managed devenv environment
- optionally run a single-node k3s server in the primary Microsandbox

## Requirements

- Go
- Microsandbox-supported local virtualization host (KVM on Linux or Apple Silicon on macOS)
- network access to pull the native devenv image from GHCR on first use
- when SecretSpec integration is enabled, a C compiler, Cargo, and Rust (the build stages SecretSpec's static library)

## Build

```bash
make build
```

`make build` enables CGO because Mezha uses the Microsandbox Go SDK.

## Usage

```bash
mezha [run-options] [-- command...]
mezha run [run-options] [-- command...]
mezha init [options]
mezha sandbox list
mezha image pull
mezha sandbox create [options]
mezha sandbox recreate [options]
mezha sandbox start [options]
mezha sandbox stop [options]
mezha sandbox destroy [options]
mezha sandbox status [options]
mezha sandbox logs [options]
mezha sync status [options]
mezha sync upload [options] [local-path] [remote-path]
mezha sync download [options] [remote-path] [local-path]
mezha sync pull [options]
mezha sync push [options]
mezha sync remote repair [options]
mezha volume list
mezha volume rm <name> [options]
```

Examples:

```bash
mezha init
mezha sandbox list
# Refresh the cached ghcr.io/cachix/devenv/devenv:latest image.
mezha image pull
mezha volume list
mezha
mezha -- git status
# `mezha run` is an explicit alias for the default session command.
mezha run --recreate --volumes-flush
mezha --herdr -- git status
# Create the sandbox and start its core services without synchronizing the repository.
mezha sandbox create
mezha sandbox create --herdr
# Recreate the sandbox and its persistent state, then provision it.
mezha sandbox recreate --herdr
# Reuse one sandbox for this and other projects.
mezha --sandbox shared-dev -- git status
mezha sync upload
mezha sync download reports/result.json ./result.json
mezha sync pull
mezha sync push
# Stop the sandbox while retaining it, then start it again later.
mezha sandbox stop
mezha sandbox start
mezha sandbox destroy
# List all local sandboxes and volumes. Volume output includes capacity and users.
mezha sandbox list
mezha volume list
# Remove an unused volume after confirmation.
mezha volume rm shared-dev-state
# Remove the sandbox and its retained named volumes.
mezha sandbox destroy --volumes-flush
```

## Configuration

Run `mezha init` in a repository to create `mezha.toml` and
`.mezha/devenv.nix`. By default, Mezha generates a sandbox name for the current
repository. Use `--sandbox <name>` (or `-s <name>`) with the default session, `sandbox`,
and `sync` commands to select a reusable sandbox instead. Each project is kept
in its own `/root/<project>` directory, where `<project>` exactly matches the
host repository folder name. `sandbox.remote_dir`, `MICROSANDBOX_REMOTE_REPO_DIR`,
and `--remote-dir` may choose the sandbox location but must end in that same
folder name; Mezha rejects a mismatched path. Mezha also creates a sandbox
symlink at the repository's absolute host path that points to this project
directory, allowing sandbox tools to resolve host-style project paths. A Git remote named after the selected
sandbox is added to the host repository, so the same repository can synchronize
with multiple sandboxes.

Run `mezha init --home` to create a default configuration at
`$MEZHA_HOME/mezha.toml` (`~/.mezha/mezha.toml` when `MEZHA_HOME` is unset).
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

1. `$MEZHA_HOME/mezha.toml`
2. `$MEZHA_HOME/projects/<git-project>/mezha.toml`
3. `$MEZHA_HOME/worktrees/<git-project>-<worktree>/mezha.toml`
4. `$MEZHA_HOME/sandboxes/<git-project>-<git-branch>/mezha.toml`
5. `<project-root>/mezha.toml`

The project, worktree, and sandbox directory names use Mezha's safe sandbox
name format; linked worktrees use the primary repository's name for
`<git-project>`. Mezha always uses `ghcr.io/cachix/devenv/devenv:latest` and
runs it as UID 0: Docker and k3s
require it, and Microsandbox cannot resolve the native image's `1000:100` user
declaration. Mezha uploads its managed devenv configuration during sandbox
creation. Relative paths in `provision.add` are resolved relative to the
configuration file.

```toml
version = 1

[microsandbox]
# Mezha always uses ghcr.io/cachix/devenv/devenv:latest as UID 0.
memory_mib = 4096

# All persistent state shares this volume.
[[microsandbox.volumes]]
name = "state"
target = "/nix"
mode = "ensure-exists"
kind = "disk"
size_mib = 51200

# [microsandbox.network]
# default_egress = "deny"
# [[microsandbox.network.rules]]
# action = "allow"
# direction = "egress"
# destination = "public"

[services.docker]
# Docker is supplied by the Mezha-managed devenv environment.
# Start dockerd when the sandbox is created or reused.
enabled = true

[services.k3s]
# Run a k3s server alongside each Mezha session.
enabled = true

[sandbox]
# Default sandbox selection; --sandbox overrides it.
# name = "development"
# remote_dir = "/root/my-project"
# policy_advisor = true
# Register the sandbox with Herdr and synchronize local plugins.
herdr = false

# This devenv.nix declaratively provides Docker, k3s, kubectl, Git, Lazygit, and GitHub CLI.
[[provision.add]]
source = ".mezha/devenv.nix"
target = "/root/.config/mezha/services/devenv/user-devenv.nix"
```

`provision.add` and `provision.run` are applied only when a sandbox is first created.
`mezha sandbox create` performs this initialization and verifies the configured core
devenv services, but does not publish, upload, or otherwise synchronize repository
data. Mezha seeds the complete `/nix` directory into the shared `state` volume, which
is mounted at `/nix`. The temporary state-volume provisioning sandbox receives
the same `microsandbox.network` and `microsandbox.secrets`
configuration (including secret host allowlists) as the primary sandbox. It then symlinks `/root`,
`/var/lib/docker`, and `/var/lib/rancher/k3s` into that volume before any initialization command
or devenv shell runs; `/home` remains empty. The default `provision.add` installs
Mezha's `.mezha/devenv.nix` at
`/root/.config/mezha/services/devenv/user-devenv.nix`. It uses devenv `packages` for Docker, k3s, kubectl, Git,
Lazygit, GitHub CLI, Go, Groff, Less, and `col`. It configures Groff and the
manpage pager so captured help output is plain text rather than raw formatting
control sequences. Mezha declares Docker and k3s as supervised `processes` in its managed devenv
configuration. It starts the requested processes once with `devenv up -d` and
waits for their readiness probes before opening commands or interactive
sessions. Subsequent sessions attach to the same process manager, so concurrent
sessions share one Docker daemon and one k3s cluster. Update that file and run
`mezha --recreate` to apply a changed managed environment.

Set `services.docker.enabled: true` to start the shared Docker process before
configured or requested commands. Set `services.k3s.enabled: true` to start the
shared k3s process and configure `kubectl` to use the local cluster. Set it to
`false` to disable k3s. Docker images, containers, and volumes plus k3s cluster
state are stored in the shared persistent volume.
k3s uses Docker as its container runtime, so Docker-built images are immediately
available to Kubernetes. Named volume names are automatically prefixed with the
sandbox name, so each sandbox receives its own volume. Volumes are retained when
the sandbox is recreated or destroyed; this includes the Nix store and `/root` with its project
workspaces, caches, and configuration, avoiding repeated downloads and evaluation after
`mezha --recreate`. Use `--volumes-flush` with
`mezha sandbox destroy`, or with `mezha --recreate`, only when a clean set of
persistent volumes is required.

## SecretSpec integration

Mezha can resolve a project’s `secretspec.toml` with the SecretSpec Go SDK before
starting or provisioning a sandbox. This replaces a host-side wrapper such as:

```sh
secretspec run --provider keyring --profile devtools -- mezha --sandbox sandbox1 -- nvim
```

with:

```sh
mezha --sandbox sandbox1 -- nvim
```

Enable it in `mezha.toml` and configure the equivalent SecretSpec options:

```toml
[secretspec]
enabled = true
provider = "keyring"
profile = "devtools"
# path = "secretspec.toml" # optional; the default is SecretSpec's manifest search
# scope = "sandbox"
# reason = "start development sandbox"
```

Resolved values are exported only to the Mezha process. To pass a value to the
Microsandbox, explicitly declare it in `[[microsandbox.secrets]]`; this keeps
Microsandbox’s host allowlist and TLS policy in effect:

```toml
[[microsandbox.secrets]]
env = "GITHUB_TOKEN"
value_from_env = "GITHUB_TOKEN"
allow_hosts = ["api.github.com"]
require_tls = true
```

Mezha statically links `libsecretspec` into its binary. `make build`, `make run`,
and `make test` download the SecretSpec source release matching the pinned Go
SDK, verify its checksum, and build its static archive on the first run. This
requires Cargo, Rust, and a C compiler at build time, but neither
`libsecretspec` nor `SECRETSPEC_FFI_LIB` is needed at runtime.

## Herdr integration

When the local `herdr` command is installed, register the sandbox as a saved
Herdr SSH machine while creating a sandbox or opening a session with:

```bash
mezha sandbox create --herdr
# Or create, synchronize the repository, and open a session.
mezha --herdr
```

To enable this by default for the project, set it in `mezha.toml` (Mezha does
not currently use a `mezha.nix` configuration file):

```toml
[sandbox]
herdr = true
```

The default is `false`; `--herdr` or `--no-herdr` on `sandbox create` or the
default session overrides the configured value for that invocation.

The registration is idempotent and uses Mezha's sandbox SSH proxy. Mezha
installs the matching Linux Herdr release in the sandbox before registration,
then records its SSH host key for Herdr's strict saved-machine connection.
New Herdr panes start in `/root` and use the environment inherited from the remote Herdr server. Mezha launches that server through its
managed `devenv` environment, trusts its managed service configuration with `devenv allow`, and
adds `eval "$(devenv hook bash)"` to root's `.bashrc`. The server retains the devenv environment
but marks each Herdr parent pane as not already activated, allowing that hook to
activate the managed environment when a user enters it. The marker is consumed
before the hook starts its child shell, so that child retains its active-project
marker and does not recursively re-enter devenv. Herdr's pane shell remains a
direct `bash` process so plugins can reliably send startup commands as soon as a
pane is ready.
Mezha also natively installs GitHub-managed local Herdr plugins in the sandbox,
preserves their enabled state, and copies each plugin's local configuration
directory. It also copies the local Herdr `[keys]` configuration, so
`plugin_action` hotkeys (such as herdr-plus) work on the remote Herdr server.
Plugin installation and build commands run through Mezha's managed
`devenv` environment. The generated `.mezha/devenv.nix` includes Go for native
plugin builds; add other plugin-specific build tools there. `mezha sandbox destroy`
removes the corresponding saved Herdr machine profile. Herdr
is optional: if its command is not on `PATH`, Mezha skips both operations.

### Herdr plugin

The plugin in `herdr` exposes a `dev.mezha.dashboard` action that opens an
interactive terminal interface in a Herdr-managed overlay pane. The dashboard
provides shortcuts for the default shell, sandbox lifecycle, and repository
synchronization commands, plus a command entry that accepts any Mezha CLI command and arguments. It does not install keybindings.

Install it from GitHub:

```bash
herdr plugin install odzhu/mezha/herdr
```

The `mezha` executable must be available on the environment inherited by Herdr.
The dashboard follows the herdr-plus launcher-and-pane architecture. The default
session opens the interactive sandbox shell in a new tab, while destruction uses
a popup. See `herdr/README.md` for local development instructions.

## Environment variables

- `SANDBOX_NAME`
- `MICROSANDBOX_REMOTE_REPO_DIR`
- `MICROSANDBOX_POLICY_ADVISOR`

## Development

```bash
make fmt
make test
```
