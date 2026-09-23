//go:build cgo

package app

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/odzhu/mezha/internal/execx"
	msb "github.com/superradcompany/microsandbox/sdk/go"
)

var herdrVersion = regexp.MustCompile(`^herdr (\d+\.\d+\.\d+)$`)

// ensureSandboxHerdr installs the matching Linux release without relying on
// SSH stdin streaming, which Microsandbox's SSH proxy does not support here.
func ensureSandboxHerdr(ctx context.Context, sandbox *msb.Sandbox, useDevenv bool) error {
	versionOutput, err := execx.Output(ctx, "herdr", "--version")
	if err != nil {
		return fmt.Errorf("get Herdr version: %w", err)
	}
	matches := herdrVersion.FindStringSubmatch(strings.TrimSpace(string(versionOutput)))
	if matches == nil {
		return fmt.Errorf("unrecognized Herdr version %q", strings.TrimSpace(string(versionOutput)))
	}
	version := matches[1]
	const herdrDevenvPath = "/tmp/mezha-herdr-install"
	setup, err := sandbox.Exec(ctx, "sh", []string{"-c", `set -eu
mkdir -p "$1"
cat > "$1/devenv.nix" <<'EOF'
{ pkgs, ... }:
{
  imports = [ /root/.config/mezha/services/devenv/devenv.nix ];
  packages = [ pkgs.curl ];
}
EOF
`, "mezha-herdr-env", herdrDevenvPath})
	if err != nil {
		return fmt.Errorf("prepare Herdr installation environment: %w", err)
	}
	if !setup.Success() {
		return fmt.Errorf(
			"prepare Herdr installation environment: %s",
			strings.TrimSpace(setup.Stderr()),
		)
	}
	// This devenv project is only needed to install Herdr; retain no evaluation state.
	defer func() { _ = removeSandboxPath(context.Background(), sandbox, herdrDevenvPath) }()

	script := fmt.Sprintf(`set -eu
binary="/nix/mezha/services/herdr/bin/herdr"
launcher="$HOME/.local/bin/herdr"
if [ "$("$binary" --version 2>/dev/null || :)" != "herdr %[1]s" ]; then
  case "$(uname -m)" in
    aarch64|arm64) arch=aarch64 ;;
    x86_64|amd64) arch=x86_64 ;;
    *) echo "unsupported Herdr architecture: $(uname -m)" >&2; exit 1 ;;
  esac
  mkdir -p "$(dirname "$binary")"
  tmp="$binary.tmp.$$"
  url="https://github.com/herdrdev/herdr/releases/download/v%[1]s/herdr-linux-$arch"
  curl -fsSL --retry 3 --retry-all-errors "$url" -o "$tmp"
  chmod 755 "$tmp"
  mv "$tmp" "$binary"
fi
mkdir -p "$(dirname "$launcher")"
bashrc="$HOME/.bashrc"
touch "$bashrc"
# Replace the managed hook so existing sandboxes receive hook fixes on provision.
sed -i '/^# mezha devenv hook$/,/^eval "$(devenv hook bash)"$/d' "$bashrc"
cat >> "$bashrc" <<'EOF'
# mezha devenv hook
# A Herdr pane inherits the server's devenv environment. Consume this marker
# before activating so the hook-spawned shell keeps DEVENV_ROOT and cannot recurse.
if [ "${MEZHA_HERDR_PANE:-}" = 1 ]; then
  unset DEVENV_ROOT MEZHA_HERDR_PANE
fi
eval "$(devenv hook bash)"
EOF
(cd /root/.config/mezha/services/devenv && devenv allow)
cat > "$launcher" <<'EOF'
#!/bin/sh
export HERDR_CONFIG_PATH="/root/.config/mezha/services/herdr/config.toml"
case "${1:-}" in
  remote-client-bridge|server)
    # Keep devenv's PATH and environment, but let each pane's Bash hook detect
    # and activate the project rather than inheriting an already-active shell.
    exec devenv shell --no-tui --quiet --from path:/root/.config/mezha/services/devenv -- sh -c 'unset DEVENV_ROOT; export MEZHA_HERDR_PANE=1; exec "$@"' mezha-herdr "/nix/mezha/services/herdr/bin/herdr" "$@"
    ;;
esac
exec "/nix/mezha/services/herdr/bin/herdr" "$@"
EOF
chmod 755 "$launcher"
`, version)
	fmt.Println("Provisioning Herdr through devenv...")
	args := []string{"shell", "--from", "path:" + herdrDevenvPath, "--", "sh", "-c", script}
	var output *msb.ExecOutput
	for attempt := 0; attempt < 20; attempt++ {
		output, err = sandbox.Exec(ctx, nativeDevenvPath, args)
		if err == nil || !strings.Contains(err.Error(), "No such file or directory") {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
	}
	if err != nil {
		return fmt.Errorf("install Herdr in sandbox: %w", err)
	}
	if !output.Success() {
		return fmt.Errorf("install Herdr in sandbox: %s", strings.TrimSpace(output.Stderr()))
	}
	if err := syncSandboxHerdrConfig(ctx, sandbox); err != nil {
		return err
	}
	if err := ensureSandboxHerdrServer(ctx, sandbox); err != nil {
		return err
	}
	return nil
}

// ensureSandboxHerdrServer starts one persistent server for the sandbox.
func ensureSandboxHerdrServer(ctx context.Context, sandbox *msb.Sandbox) error {
	output, err := sandbox.Exec(ctx, "sh", []string{"-eu", "-c", `
launcher="$HOME/.local/bin/herdr"
binary="/nix/mezha/services/herdr/bin/herdr"
socket="$HOME/.config/herdr/herdr.sock"
export HERDR_CONFIG_PATH="/root/.config/mezha/services/herdr/config.toml"

# reload-config succeeds only when a server owns the socket.
if "$binary" server reload-config >/dev/null 2>&1; then
  exit 0
fi

# A server that exited uncleanly can leave its socket behind.
rm -f "$socket"
mkdir -p "$(dirname "$socket")"
nohup "$launcher" server >>"$HOME/.config/herdr/herdr-server.log" 2>&1 </dev/null &
pid=$!
for _ in $(seq 1 40); do
  if "$binary" server reload-config >/dev/null 2>&1; then
    exit 0
  fi
  if ! kill -0 "$pid" 2>/dev/null; then
    wait "$pid" || true
    echo "Herdr server exited during startup" >&2
    exit 1
  fi
  sleep 0.25
done
kill "$pid" 2>/dev/null || true
wait "$pid" 2>/dev/null || true
echo "Herdr server did not become ready" >&2
exit 1
`})
	if err != nil {
		return fmt.Errorf("start Herdr server in sandbox: %w", err)
	}
	if !output.Success() {
		return fmt.Errorf("start Herdr server in sandbox: %s", strings.TrimSpace(output.Stderr()))
	}
	return nil
}

// syncSandboxHerdrConfig copies local keybindings so remote Herdr servers can
// invoke synchronized plugin actions, while retaining Mezha's terminal setup.
func syncSandboxHerdrConfig(ctx context.Context, sandbox *msb.Sandbox) error {
	config := map[string]any{}
	configPath, err := localHerdrConfigPath()
	if err != nil {
		return err
	}
	contents, err := os.ReadFile(configPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read local Herdr configuration: %w", err)
	}
	if err == nil {
		var local map[string]any
		if _, err := toml.Decode(string(contents), &local); err != nil {
			return fmt.Errorf("parse local Herdr configuration: %w", err)
		}
		if keys, ok := local["keys"]; ok {
			config["keys"] = keys
		}
	}
	config["terminal"] = map[string]string{
		"default_shell": "bash",
		"shell_mode":    "non_login",
		"new_cwd":       "follow",
	}

	var encoded bytes.Buffer
	if err := toml.NewEncoder(&encoded).Encode(config); err != nil {
		return fmt.Errorf("encode sandbox Herdr configuration: %w", err)
	}
	temp, err := os.CreateTemp("", "mezha-herdr-*.toml")
	if err != nil {
		return fmt.Errorf("create sandbox Herdr configuration: %w", err)
	}
	defer func() { _ = os.Remove(temp.Name()) }()
	if _, err := temp.Write(encoded.Bytes()); err != nil {
		_ = temp.Close()
		return fmt.Errorf("write sandbox Herdr configuration: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close sandbox Herdr configuration: %w", err)
	}
	if err := nativeUpload(
		ctx,
		sandbox,
		temp.Name(),
		"/root/.config/mezha/services/herdr/config.toml",
	); err != nil {
		return fmt.Errorf("copy Herdr keybindings to sandbox: %w", err)
	}
	// Reload an already-running remote server; a first connection loads it normally.
	_, _ = sandbox.Exec(ctx, "/root/.local/bin/herdr", []string{"server", "reload-config"})
	return nil
}

func localHerdrConfigPath() (string, error) {
	if path := strings.TrimSpace(os.Getenv("HERDR_CONFIG_PATH")); path != "" {
		return path, nil
	}
	if configHome := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); configHome != "" {
		return filepath.Join(configHome, "herdr", "config.toml"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve local Herdr configuration: %w", err)
	}
	return filepath.Join(home, ".config", "herdr", "config.toml"), nil
}
