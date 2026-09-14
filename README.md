# mezha

Declarative agent sandboxes powered by Microsandbox.

## Features

- project configuration in `mezha.yaml`
- create, run, rebuild, and destroy Microsandbox sandboxes
- synchronize committed changes through a sandbox Git remote
- upload and download dirty working-tree changes or selected files
- build and import a Dockerfile image into Microsandbox
- optionally run a single-node k3s server in the primary Microsandbox

## Requirements

- Go
- Microsandbox-supported local virtualization host (KVM on Linux or Apple Silicon on macOS)
- a local Docker-compatible daemon for `mezha build`

## Build

```bash
make build
```

`make build` enables CGO because Mezha uses the Microsandbox Go SDK.

## Usage

```bash
mezha init [options]
mezha run [options] [-- command...]
mezha build [options]
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
mezha build
mezha upload
mezha download reports/result.json ./result.json
mezha pull
mezha push
mezha destroy
```

## Configuration

Run `mezha init` in a repository to create `mezha.yaml` and `.mezha/Dockerfile`.
Run `mezha init --home` to create a default configuration at
`~/.mezha/mezha.yaml`. Relative paths in `microsandbox.dockerfile` and
`create.add` are resolved relative to the configuration file.

```yaml
version: 1

microsandbox:
  dockerfile: .mezha/Dockerfile
  memory_mib: 4096
  volumes:
    - name: nix-packages
      target: /nix
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
  # The default Dockerfile installs both the Docker client and daemon.
  # Start dockerd when the sandbox is created or reused.
  enabled: true

sandbox:
  # name: development
  # remote_dir: /sandbox/my-project
  # policy_advisor: true
  # Run a k3s server in this sandbox.
  # kubernetes: true

create:
  run:
    - nix-channel --update

run: []
```

`create.add` and `create.run` are applied only when a sandbox is first created.
Set `docker.enabled: true` to start the Docker daemon before configured or
requested commands. The selected image must include `docker` and `dockerd`; the
default Dockerfile installs both. It also installs `kubectl`: set
`sandbox.kubernetes: true` to run the bundled k3s server directly alongside
the requested command in the primary sandbox. Mezha configures `kubectl` to
use that local cluster. The default configuration mounts dedicated `docker-data`
and `k3s-data` volumes at `/var/lib/docker` and `/var/lib/rancher/k3s` to preserve
Docker images, containers, and volumes plus k3s cluster state independently of
the sandbox filesystem. k3s uses Docker as its container runtime, so Docker-built
images are immediately available to Kubernetes. Named volume names are automatically prefixed with the
sandbox name, so each sandbox receives its own volume. Entries in `run` execute
before the requested command. Strings use shell form;
YAML sequences use exec form.

## Environment variables

- `SANDBOX_NAME`
- `MICROSANDBOX_REMOTE_REPO_DIR`
- `MICROSANDBOX_POLICY_ADVISOR`

## Development

```bash
make fmt
make test
```
