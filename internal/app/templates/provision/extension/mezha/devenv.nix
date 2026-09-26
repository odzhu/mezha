{ pkgs, config, lib, ... }:

{
  tasks."mezha:init-sandbox" = {
    exec = ''
      set -eu

      # 1. Persistent Links
      if [ ! -f /nix/.mezha-state-v3 ]; then
        echo "shared persistent /nix volume is not mounted; recreate the sandbox" >&2
        exit 1
      fi
      persist_link() {
        target="$1"
        source="$2"
        mkdir -p "$source" "$(dirname "$target")"
        if [ -L "$target" ] && [ "$(readlink "$target")" = "$source" ]; then
          return
        fi
        if [ "$target" = "/root" ] && [ -d "/root" ] && [ ! -L "/root" ]; then
          cp -a /root/. "$source/" 2>/dev/null || true
        fi
        rm -rf "$target"
        ln -s "$source" "$target"
      }
      persist_link /var/lib/docker /nix/mezha/services/docker
      persist_link /var/lib/rancher/k3s /nix/mezha/services/k3s
      persist_link /root /nix/mezha/root

      # 2. Project Directory Links
      if [ -n "''${MSB_PROJECT:-}" ]; then
        project="$MSB_PROJECT"
        primary="$MSB_PRIMARY"
        remote="$MSB_REMOTE"
        host_project="$MSB_HOST_PROJECT"
        host_primary="$MSB_HOST_PRIMARY"
        home_project="$MSB_HOME_PROJECT"
        home_primary="$MSB_HOME_PRIMARY"

        mkdir -p /nix/mezha/projects /nix/mezha/worktrees /root/.herdr
        for path in "$primary" "$project"; do
          if [ -L "$path" ]; then rm -f "$path"; fi
        done
        mkdir -p "$primary" "$project"

        link_path() {
          target="$1" link="$2"
          [ "$target" = "$link" ] && return
          if [ -d "$link" ]; then
            target_physical=$(CDPATH= cd "$target" && pwd -P)
            link_physical=$(CDPATH= cd "$link" && pwd -P)
            [ "$target_physical" = "$link_physical" ] && return
          fi
          mkdir -p "$(dirname "$link")"
          if [ -e "$link" ] && [ ! -L "$link" ]; then
            rm -rf "$link"
          fi
          ln -sfn "$target" "$link"
        }

        link_path /nix/mezha/projects /projects
        link_path /nix/mezha/worktrees /worktrees
        if [ -L /root/projects ]; then rm -f /root/projects; fi
        if [ -L /root/worktrees ]; then rm -f /root/worktrees; fi
        link_path /nix/mezha/worktrees /root/.herdr/worktrees
        link_path "$project" "$remote"
        link_path "$project" "$host_project"
        link_path "$primary" "$host_primary"
        if [ -n "$home_project" ]; then link_path "$project" "$HOME/$home_project"; fi
        if [ -n "$home_primary" ]; then link_path "$primary" "$HOME/$home_primary"; fi
      fi

      # 3. Bash Hook
      bashrc="/root/.bashrc"
      touch "$bashrc"
      sed -i '/^# mezha devenv hook$/,/^eval "$(devenv hook bash)"$/d' "$bashrc"
      cat >> "$bashrc" <<'EOF'
# mezha devenv hook
if [ "''${MEZHA_HERDR_PANE:-}" = 1 ]; then
  unset DEVENV_ROOT MEZHA_HERDR_PANE
fi
eval "$(devenv hook bash)"
EOF

      # 4. Bash Profile
      if [ -n "''${MSB_WORKDIR:-}" ]; then
        profile="/root/.bash_profile"
        touch "$profile"
        sed -i '/^# mezha project shell$/,/^# mezha project shell end$/d' "$profile"
        cat >> "$profile" <<EOF
# mezha project shell
cd "''${MSB_WORKDIR}"
if [ -f "/root/.bashrc" ]; then
  . "/root/.bashrc"
fi
# mezha project shell end
EOF
      fi

      # 5. Herdr Installation
      if [ -n "''${MSB_HERDR_VERSION:-}" ]; then
        binary="/nix/mezha/services/herdr/bin/herdr"
        launcher="/root/.local/bin/herdr"
        if [ "$("$binary" --version 2>/dev/null || :)" != "herdr ''${MSB_HERDR_VERSION}" ]; then
          case "$(uname -m)" in
            aarch64|arm64) arch=aarch64 ;;
            x86_64|amd64) arch=x86_64 ;;
            *) echo "unsupported Herdr architecture: $(uname -m)" >&2; exit 1 ;;
          esac
          mkdir -p "$(dirname "$binary")"
          tmp="$binary.tmp.$$"
          url="https://github.com/herdrdev/herdr/releases/download/v''${MSB_HERDR_VERSION}/herdr-linux-$arch"
          ${pkgs.curl}/bin/curl -fsSL --retry 3 --retry-all-errors "$url" -o "$tmp"
          chmod 755 "$tmp"
          mv "$tmp" "$binary"
        fi
        mkdir -p "$(dirname "$launcher")"
        (cd /root/.config/mezha/services/devenv && devenv allow || true)
        cat > "$launcher" <<'EOF'
#!/bin/sh
export HERDR_CONFIG_PATH="/root/.config/mezha/services/herdr/config.toml"
case "''${1:-}" in
  remote-client-bridge|server)
    exec devenv shell --no-tui --quiet --from path:/root/.config/mezha/services/devenv -- sh -c 'unset DEVENV_ROOT; export MEZHA_HERDR_PANE=1; exec "$@"' mezha-herdr "/nix/mezha/services/herdr/bin/herdr" "$@"
    ;;
esac
exec "/nix/mezha/services/herdr/bin/herdr" "$@"
EOF
        chmod 755 "$launcher"
      fi
    '';
  };

  processes.mezha-herdr = {
    start.enable = lib.mkDefault false;
    exec = ''
      export HERDR_CONFIG_PATH="/root/.config/mezha/services/herdr/config.toml"
      if /nix/mezha/services/herdr/bin/herdr server reload-config >/dev/null 2>&1; then
        exit 0
      fi
      exec /root/.local/bin/herdr server
    '';
    ready.exec = ''
      export HERDR_CONFIG_PATH="/root/.config/mezha/services/herdr/config.toml"
      /nix/mezha/services/herdr/bin/herdr server reload-config >/dev/null 2>&1
    '';
  };
}
