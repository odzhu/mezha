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
mezha upload
mezha download reports/result.json ./result.json
mezha pull
mezha push
mezha destroy
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
    - [.mezha/devenv.nix, /opt/mezha/devenv.nix]

run: []
```

`create.add` and `create.run` are applied only when a sandbox is first created.
Mezha seeds the default 20 GiB `nix-packages` volume from the native image
before mounting it at `/nix/store`, preserving the image runtime and profiles
while keeping Nix packages and devenv downloads persistent. The default `create.add` installs
Mezha's `.mezha/devenv.nix` at
`/opt/mezha/devenv.nix`. It uses devenv `packages` for Docker, k3s, and kubectl
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
own volume. Entries in `run` execute before the requested command. Strings use
shell form; YAML sequences use exec form.

## Environment variables

- `SANDBOX_NAME`
- `MICROSANDBOX_REMOTE_REPO_DIR`
- `MICROSANDBOX_POLICY_ADVISOR`

## Development

```bash
make fmt
make test
```
