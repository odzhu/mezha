# mezha

Declarative agent sandboxes powered by Microsandbox.

## Features

- project configuration in `mezha.yaml`
- create, run, rebuild, and destroy Microsandbox sandboxes
- synchronize committed changes through a sandbox Git remote
- upload and download dirty working-tree changes or selected files
- use the native `ghcr.io/cachix/devenv/devenv:latest` image for sandbox tooling
- provision Docker, k3s, and kubectl declaratively through a Mezha-managed devenv environment
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
mezha run [options] [-- command...]
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
mezha run
mezha run -- git status
mezha run --herdr true -- git status
mezha upload
mezha download reports/result.json ./result.json
mezha pull
mezha push
mezha destroy
# Remove the sandbox and its retained named volumes.
mezha destroy --volumes-flush
```

## Configuration

Run `mezha init` in a repository to create `mezha.yaml` and
`.mezha/devenv.nix`. Run `mezha init --home` to create a default configuration
at `~/.mezha/mezha.yaml`. Mezha always uses
`ghcr.io/cachix/devenv/devenv:latest` and runs it as UID 0: Docker and k3s
require it, and Microsandbox cannot resolve the native image's `1000:100` user
declaration. Mezha uploads its managed devenv configuration during sandbox
creation. Relative paths in `create.add` are resolved relative to the
configuration file.

```yaml
version: 1

microsandbox:
  # Mezha always uses ghcr.io/cachix/devenv/devenv:latest as UID 0.
  memory_mib: 4096
  volumes:
    # Mezha seeds this from the native image before mounting it at /nix/store.
    - name: nix-packages
      target: /nix/store
      mode: ensure-exists
      kind: disk
      size_mib: 20480
    # Preserve the workspace, repositories, devenv state, and GOPATH across
    # recreation with a lifecycle independent of the sandbox VM.
    - name: sandbox-state
      target: /sandbox
      mode: ensure-exists
      kind: disk
      size_mib: 20480
    # Preserve root's caches, configuration, and other state across recreation.
    - name: root-state
      target: /root
      mode: ensure-exists
      kind: disk
      size_mib: 20480
    - name: docker-data
      target: /var/lib/docker
      mode: ensure-exists
      kind: disk
      size_mib: 20480
    - name: k3s-data
      target: /var/lib/rancher/k3s
      mode: ensure-exists
      kind: disk
      size_mib: 20480
  # network:
  #   default_egress: deny
  #   rules:
  #     - action: allow
  #       direction: egress
  #       destination: public

docker:
  # Docker is supplied by the Mezha-managed devenv environment.
  # Start dockerd when the sandbox is created or reused.
  enabled: true

sandbox:
  # name: development
  # remote_dir: /sandbox/my-project
  # policy_advisor: true
  # Run a k3s server alongside each Mezha session.
  kubernetes: true

create:
  # This devenv.nix declaratively provides Docker, k3s, and kubectl.
  add:
    - [.mezha/devenv.nix, /sandbox/devenv.nix]

run: []
```

`create.add` and `create.run` are applied only when a sandbox is first created.
Mezha seeds the default 20 GiB `nix-packages` volume from the native image
before mounting it at `/nix/store`, preserving the image runtime and profiles
while keeping Nix packages and devenv downloads persistent. `/sandbox` is a
separate persistent volume, and Mezha defaults `GOPATH` to `/sandbox/go` unless
it is explicitly configured in `microsandbox.env`. The default `create.add` installs
Mezha's `.mezha/devenv.nix` at
`/sandbox/devenv.nix`. It uses devenv `packages` for Docker, k3s, and kubectl
and `devenv:enterShell` tasks to start Docker and k3s, wait for readiness, and
configure `kubectl`. Mezha enters that environment for every requested command
or interactive session. Update that file and run `mezha run --recreate` to
apply a changed managed environment.

Set `docker.enabled: true` to start the Docker daemon before configured or
requested commands. `sandbox.kubernetes` defaults to `true`, so Mezha runs k3s
alongside each command or interactive session and configures `kubectl` to use
the local cluster. Set it to `false` to disable k3s. The default configuration mounts dedicated `docker-data` and
`k3s-data` volumes at `/var/lib/docker` and `/var/lib/rancher/k3s` to preserve
Docker images, containers, and volumes plus k3s cluster state independently of
the sandbox filesystem. k3s uses Docker as its container runtime, so
Docker-built images are immediately available to Kubernetes. Named volume names
are automatically prefixed with the sandbox name, so each sandbox receives its
own volume. Volumes are retained when the sandbox is recreated or destroyed;
this includes the Nix store, the complete `/sandbox` workspace, and `/root` with
its caches and configuration, avoiding repeated downloads and evaluation after
`mezha
run --recreate`. Use `--volumes-flush` with
`mezha destroy`, or with `mezha run --recreate`, only when a clean set of
persistent volumes is required. Entries in `run` execute before the requested
command. Strings use
shell form; YAML sequences use exec form.

## Herdr integration

When the local `herdr` command is installed, register the sandbox as a saved
Herdr SSH machine with:

```bash
mezha run --herdr true
```

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

## Environment variables

- `SANDBOX_NAME`
- `MICROSANDBOX_REMOTE_REPO_DIR`
- `MICROSANDBOX_POLICY_ADVISOR`

## Development

```bash
make fmt
make test
```
