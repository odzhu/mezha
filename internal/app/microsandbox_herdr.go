//go:build cgo

package app

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

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
	shell := "exec bash"
	if useDevenv {
		shell = "exec devenv shell --from path:/sandbox -- bash"
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
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$url" -o "$tmp"
  else
    wget -q -O "$tmp" "$url"
  fi
  chmod 755 "$tmp"
  mv "$tmp" "$binary"
fi
mkdir -p "$(dirname "$launcher")"
cat > "$launcher" <<'EOF'
#!/bin/sh
export HERDR_CONFIG_PATH="$HOME/.mezha/herdr.toml"
for arg in "$@"; do
  if [ "$arg" = "remote-client-bridge" ]; then
    exec devenv shell --no-tui --quiet --from path:/sandbox -- "$HOME/.mezha/herdr-bin" "$@"
  fi
done
exec "$HOME/.mezha/herdr-bin" "$@"
EOF
chmod 755 "$launcher"
cat > "$mezha_dir/herdr-shell" <<'EOF'
#!/bin/sh
%[2]s
EOF
chmod 755 "$mezha_dir/herdr-shell"
cat > "$mezha_dir/herdr.toml" <<'EOF'
[terminal]
default_shell = %[3]s
shell_mode = "non_login"
new_cwd = %[4]s
EOF
`, version, shell, strconv.Quote("/root/.mezha/herdr-shell"), strconv.Quote(workdir))
	fmt.Println("Provisioning Herdr through devenv...")
	args := []string{"shell", "--from", "path:" + managedDevenvPath, "--", "sh", "-c", script}
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
	return nil
}
