# mezha

Isolated, declarative development environments for the agentic era.

Mezha gives developers and coding agents a local Linux sandbox for each Git
project—keeping commands, dependencies, and services away from the host.

- **[Microsandbox](https://docs.microsandbox.dev/getting-started/introduction):** lightweight microVMs with their own kernel and configurable network, secret, and resource policies.
- **[devenv](https://devenv.sh/basics/):** Nix-based tooling and background services, with persistent caches and state.
- **[SecretSpec](https://secretspec.dev/llms.txt):** minimal native integration to resolve secrets on the host before starting the sandbox.
- **Git integration:** publish branches, synchronize working-tree changes, and support linked worktrees or a shared sandbox.
- **[Herdr](https://herdr.dev):** optional sandbox access and a terminal dashboard for managing your development environment.

## Quickstart

### 1. Install

Download a binary for your platform from [GitHub Releases](https://github.com/odzhu/mezha/releases):
`mezha-linux-amd64`, `mezha-linux-arm64`, or `mezha-darwin-arm64`.
Verify its SHA-256 against the release's `checksums.txt`, then install it
(for example, on Linux amd64):

```sh
mkdir -p "$HOME/.local/bin"
install -m 0755 ./mezha-linux-amd64 "$HOME/.local/bin/mezha"
export PATH="$HOME/.local/bin:$PATH"
```

Supported hosts are Linux with KVM or Apple Silicon macOS. Linux release binaries
are built on Ubuntu 24.04 and require a compatible glibc (2.39 or newer).
SecretSpec is linked into the binary; no separate SecretSpec installation is needed.

#### Build from source

Requirements: Linux with KVM or Apple Silicon macOS; Go (the version specified in
[`go.mod`](go.mod)), a C compiler, Rust/Cargo, Python 3, Git, and Make.
Builds also use `curl` and `tar` to download dependencies.

```sh
git clone https://github.com/odzhu/mezha.git
cd mezha
make install
export PATH="$HOME/.local/bin:$PATH"
```

The build enables CGO and statically links SecretSpec, even when secret resolution
is disabled. Alternatively, `make build` produces `./bin/mezha`.

### 2. Open a project sandbox

Run from a Git checkout on a branch with at least one commit:

```sh
cd /path/to/your-project
mezha init                # Create mezha.toml and .mezha/extensions/sample/devenv.nix
mezha config check
mezha -- git status      # Create/provision the sandbox and publish the branch
mezha sync upload        # Copy local uncommitted changes and untracked files
mezha                    # Open an interactive shell
```

Mezha installs the local Microsandbox runtime automatically and bootstraps Debian,
Nix, and devenv on first use. Internet access is required; Nix and devenv do not
need to be installed on the host.

Commands after `--` run in the sandbox's project directory. Interactive shells
start in `/root`; projects are available at `/root/<project-folder>`.
Subsequent runs reuse the sandbox rather than automatically synchronizing changes.

## Customize

Use `mezha.toml` for sandbox resources, volumes, network rules, and explicit secret
allowlists. See the [configuration template](internal/app/templates/mezha.toml)
for examples. `mezha init --home` creates user defaults in `~/.mezha` (or
`$MEZHA_HOME`); project configuration overrides those defaults.

To use SecretSpec, enable `[secretspec]` in `mezha.toml` and declare the secrets
in `secretspec.toml`. Mezha resolves them through the native SDK into its host
process; pass selected values into the sandbox with `[[microsandbox.secrets]]`
entries (see the [configuration template](internal/app/templates/mezha.toml)).

Add tools, environment variables, and services through
`.mezha/extensions/<name>/devenv.nix`. For example:

```nix
{ pkgs, ... }: {
  packages = [ pkgs.nodejs pkgs.ripgrep ];
  env.EDITOR = "nvim";
}
```

Extensions are automatically imported into Mezha's managed devenv environment.
Use `mezha --recreate` to rebuild the sandbox while retaining persistent state.

## Everyday commands

```sh
mezha sync status            # Compare local and sandbox Git state
mezha sync push              # Send local commits to the sandbox
mezha sync pull              # Bring sandbox commits back locally
mezha sync upload            # Send uncommitted files to the sandbox
mezha sync download          # Bring uncommitted files back locally
mezha processes list         # Inspect background devenv services
mezha sandbox logs --follow
mezha sandbox stop           # Stop without deleting the sandbox
mezha sandbox start
mezha sandbox destroy        # Delete the sandbox, retain persistent volumes
```

Synchronization can overwrite or delete files; review changes before transferring.
To remove persistent state too, use `mezha sandbox destroy --volumes-flush`.
Use `--sandbox <name>` to select a shared sandbox, and `mezha --help` for the full CLI.

## Herdr integration

With Herdr installed and `mezha` on its inherited `PATH`:

```sh
herdr plugin install odzhu/mezha/herdr
mezha --herdr
```

`--herdr` registers the sandbox as a saved SSH machine and synchronizes local
Herdr plugins. Set `sandbox.herdr = true` in `mezha.toml` to enable it by default.
The plugin's `dev.mezha.dashboard` action provides shell, lifecycle, configuration,
and sync controls. See [`herdr/README.md`](herdr/README.md) for details.

## Development

```sh
make fmt
make test
```

### CI and releases

[GitHub Actions](.github/workflows/ci.yml) checks formatting and module consistency,
runs tests, builds the CGO-enabled CLI, and checks `--help` on Linux amd64,
Linux arm64, and Apple Silicon macOS. It runs for pull requests to `main`, pushes
to `main`, `v*` tags, and manual dispatches. Each build uploads a standalone binary
as a workflow artifact; no archives or installer packages are produced.

Push a version tag to publish all three binaries and `checksums.txt` to a GitHub
Release after the checks pass:

```sh
git tag v0.1.0
git push origin v0.1.0
```

Tags containing a hyphen (such as `v0.1.0-rc.1`) create prereleases. Releases use
the workflow's `GITHUB_TOKEN`; no additional publishing secret is required.

## Configuration template

See the [`mezha.toml` template](internal/app/templates/mezha.toml) for the full
configuration reference and examples.

Licensed under [Apache 2.0](LICENSE).
