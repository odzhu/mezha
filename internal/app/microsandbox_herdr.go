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
func ensureSandboxHerdr(
	ctx context.Context,
	sandbox *msb.Sandbox,
	workdir string,
	useDevenv bool,
) error {
	versionOutput, err := execx.Output(ctx, "herdr", "--version")
	if err != nil {
		return fmt.Errorf("get Herdr version: %w", err)
	}
	matches := herdrVersion.FindStringSubmatch(strings.TrimSpace(string(versionOutput)))
	if matches == nil {
		return fmt.Errorf("unrecognized Herdr version %q", strings.TrimSpace(string(versionOutput)))
	}
	version := matches[1]
	const herdrDevenvPath = "/root/.mezha/herdr-devenv"
	setup, err := sandbox.Exec(ctx, "sh", []string{"-c", `set -eu
mkdir -p "$1"
cat > "$1/devenv.nix" <<'EOF'
{ pkgs, ... }:
{
  imports = [ /sandbox/devenv.nix ];
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

	script := fmt.Sprintf(`set -eu
mezha_dir="$HOME/.mezha"
binary="$mezha_dir/herdr-bin"
launcher="$HOME/.local/bin/herdr"
if [ "$("$binary" --version 2>/dev/null || :)" != "herdr %[1]s" ]; then
  case "$(uname -m)" in
    aarch64|arm64) arch=aarch64 ;;
    x86_64|amd64) arch=x86_64 ;;
    *) echo "unsupported Herdr architecture: $(uname -m)" >&2; exit 1 ;;
  esac
  mkdir -p "$mezha_dir"
  tmp="$binary.tmp.$$"
  url="https://github.com/herdrdev/herdr/releases/download/v%[1]s/herdr-linux-$arch"
  curl -fsSL --retry 3 --retry-all-errors "$url" -o "$tmp"
  chmod 755 "$tmp"
  mv "$tmp" "$binary"
fi
mkdir -p "$(dirname "$launcher")"
bashrc="$HOME/.bashrc"
touch "$bashrc"
if ! grep -Fqx '# mezha devenv hook' "$bashrc"; then
  cat >> "$bashrc" <<'EOF'
# mezha devenv hook
eval "$(devenv hook bash)"
EOF
fi
cat > "$launcher" <<'EOF'
#!/bin/sh
export HERDR_CONFIG_PATH="$HOME/.mezha/herdr.toml"
case "${1:-}" in
  remote-client-bridge|server)
    exec devenv shell --no-tui --quiet --from path:/sandbox -- "$HOME/.mezha/herdr-bin" "$@"
    ;;
esac
exec "$HOME/.mezha/herdr-bin" "$@"
EOF
chmod 755 "$launcher"
`, version)
	fmt.Println("Provisioning Herdr through devenv...")
	args := []string{"shell", "--from", "path:" + herdrDevenvPath, "--", "sh", "-c", script}
	var code int
	for attempt := 0; attempt < 20; attempt++ {
		code, err = sandbox.AttachWith(ctx, nativeDevenvPath, args)
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
	if code != 0 {
		return fmt.Errorf("install herdr in sandbox exited with code %d", code)
	}
	if err := syncSandboxHerdrConfig(ctx, sandbox, workdir); err != nil {
		return err
	}
	return nil
}

// syncSandboxHerdrConfig copies local keybindings so remote Herdr servers can
// invoke synchronized plugin actions, while retaining Mezha's terminal setup.
func syncSandboxHerdrConfig(ctx context.Context, sandbox *msb.Sandbox, workdir string) error {
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
		"new_cwd":       workdir,
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
	if err := nativeUpload(ctx, sandbox, temp.Name(), "/root/.mezha/herdr.toml"); err != nil {
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
